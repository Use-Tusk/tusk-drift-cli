package review

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Hard and soft caps. Hard caps abort with an error listing top contributors.
// Soft caps emit a stderr warning and continue.
const (
	SoftLineCap  = 2_000
	SoftFileCap  = 50
	HardLineCap  = 10_000
	HardFileCap  = 200
	HardBytesCap = 1 << 20 // 1 MiB
)

// FileSummary describes a single file's contribution to the patch, used to
// build "top contributors" lists for size-cap error messages.
type FileSummary struct {
	Path       string
	AddedLines int
	DelLines   int
}

// PatchResult is what BuildPatch returns on success.
type PatchResult struct {
	Patch        []byte
	BaseSha      string
	BaseRef      string // The ref the base resolved from (e.g. "origin/main"), informational.
	ChangedFiles []FileSummary
	ChangedLines int
	FileCount    int
}

// ErrEmptyPatch is returned when the working tree diff against the base is
// empty (or empty after filtering).
var ErrEmptyPatch = errors.New("empty patch")

// SubmoduleError is returned when the generated patch contains submodule
// updates — these are not supported in v1.
type SubmoduleError struct {
	Paths []string
}

func (e *SubmoduleError) Error() string {
	return fmt.Sprintf("submodule changes are not supported (%d path(s))", len(e.Paths))
}

// PatchTooLargeError is returned when the patch exceeds a hard cap.
type PatchTooLargeError struct {
	Reason       string // "lines", "files", or "bytes"
	Bytes        int
	Lines        int
	Files        int
	TopFiles     []FileSummary
	LimitMessage string // human-readable summary, e.g. "1.4MB (limit: 1MB)"
}

func (e *PatchTooLargeError) Error() string {
	return "patch too large: " + e.LimitMessage
}

// BaseResolutionError wraps merge-base or ref-resolution failures with the
// structured remediation UX from the plan.
type BaseResolutionError struct {
	Message string
}

func (e *BaseResolutionError) Error() string { return e.Message }

// IsBaseResolutionError reports whether err (or anything it wraps) is a
// *BaseResolutionError.
func IsBaseResolutionError(err error) bool {
	var b *BaseResolutionError
	return errors.As(err, &b)
}

// PatchOptions drives BuildPatch.
type PatchOptions struct {
	RepoRoot        string
	Base            string // user-provided --base ref/sha; empty → auto-detect via origin/HEAD
	ExtraExcludes   []string
	Includes        []string
	RegisterCleanup func(fn func()) // typically cmd.RegisterCleanup; may be nil for tests
	// Stderr is where soft-warnings are written. Defaults to os.Stderr when nil.
	Stderr *os.File
}

// BuildPatch generates a binary-clean git patch of the current working tree
// against a resolved base SHA. Untracked files are added with `git add -N`
// (and the intent-to-add is undone both via a normal return path and via a
// signal-safe cleanup registered through opts.RegisterCleanup).
//
// Filtering is always applied (EXTENSIONS/FILES/DIRECTORIES skip lists +
// .tuskignore + --exclude/--include). Returns ErrEmptyPatch if nothing
// survives filtering.
func BuildPatch(ctx context.Context, opts PatchOptions) (*PatchResult, error) {
	stderr := opts.Stderr
	if stderr == nil {
		stderr = os.Stderr
	}

	untracked, err := listUntracked(ctx, opts.RepoRoot)
	if err != nil {
		return nil, err
	}

	if len(untracked) > 0 {
		if err := gitRun(ctx, opts.RepoRoot, append([]string{"add", "-N", "--"}, untracked...)...); err != nil {
			return nil, fmt.Errorf("git add -N: %w", err)
		}
		restore := func() {
			// Cleanup must use a fresh context — ctx may already be cancelled
			// (Ctrl+C) at the time this runs, and we still need the restore
			// to succeed. Idempotent — if a path is no longer staged, this is
			// a no-op.
			_ = gitRun(context.Background(), opts.RepoRoot, append([]string{"restore", "--staged", "--"}, untracked...)...)
		}
		if opts.RegisterCleanup != nil {
			opts.RegisterCleanup(restore)
		}
		// Best-effort restore on the normal return path as well.
		defer restore()
	}

	baseSha, baseRef, err := resolveBase(ctx, opts.RepoRoot, opts.Base)
	if err != nil {
		return nil, err
	}

	// Merge .tuskignore entries with --exclude flags.
	tuskignoreExtras, err := ReadTuskignore(opts.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("read .tuskignore: %w", err)
	}
	extraExcludes := append([]string{}, opts.ExtraExcludes...)
	extraExcludes = append(extraExcludes, tuskignoreExtras...)

	pathspecs := BuildPathspecExclusions(extraExcludes, opts.Includes)

	// Generate binary patch.
	diffArgs := append([]string{"diff", "--binary", baseSha, "--"}, pathspecs...)
	patchBytes, err := gitOutput(ctx, opts.RepoRoot, diffArgs...)
	if err != nil {
		return nil, fmt.Errorf("git diff --binary: %w", err)
	}

	// Generate numstat for per-file line counts. `-z` writes NUL-terminated
	// records so paths containing spaces or other whitespace survive intact.
	numstatArgs := append([]string{"diff", "--numstat", "-z", baseSha, "--"}, pathspecs...)
	numstatBytes, err := gitOutput(ctx, opts.RepoRoot, numstatArgs...)
	if err != nil {
		return nil, fmt.Errorf("git diff --numstat: %w", err)
	}
	files := parseNumstat(numstatBytes)

	if len(files) == 0 {
		return nil, ErrEmptyPatch
	}

	// Submodule check on raw patch bytes. Two markers: `160000` mode lines or
	// `Subproject commit ` lines.
	if paths := detectSubmodulePaths(patchBytes); len(paths) > 0 {
		return nil, &SubmoduleError{Paths: paths}
	}

	totalLines := 0
	for _, f := range files {
		totalLines += f.AddedLines + f.DelLines
	}
	fileCount := len(files)
	byteLen := len(patchBytes)

	// Hard caps — most specific wins for the error reason, but all checks are
	// equivalent: patch is too big to upload.
	if byteLen > HardBytesCap {
		return nil, &PatchTooLargeError{
			Reason:       "bytes",
			Bytes:        byteLen,
			Lines:        totalLines,
			Files:        fileCount,
			TopFiles:     topContributors(files, 5),
			LimitMessage: fmt.Sprintf("%s (limit: 1MB)", humanBytes(byteLen)),
		}
	}
	if totalLines > HardLineCap {
		return nil, &PatchTooLargeError{
			Reason:       "lines",
			Bytes:        byteLen,
			Lines:        totalLines,
			Files:        fileCount,
			TopFiles:     topContributors(files, 5),
			LimitMessage: fmt.Sprintf("%d lines changed (limit: %d)", totalLines, HardLineCap),
		}
	}
	if fileCount > HardFileCap {
		return nil, &PatchTooLargeError{
			Reason:       "files",
			Bytes:        byteLen,
			Lines:        totalLines,
			Files:        fileCount,
			TopFiles:     topContributors(files, 5),
			LimitMessage: fmt.Sprintf("%d files changed (limit: %d)", fileCount, HardFileCap),
		}
	}

	// Soft caps — warn but continue.
	if totalLines > SoftLineCap {
		_, _ = fmt.Fprintf(stderr, "warning: %d lines changed (soft limit: %d). Review quality may degrade on large diffs.\n",
			totalLines, SoftLineCap)
	}
	if fileCount > SoftFileCap {
		_, _ = fmt.Fprintf(stderr, "warning: %d files changed (soft limit: %d). Review quality may degrade on large diffs.\n",
			fileCount, SoftFileCap)
	}

	return &PatchResult{
		Patch:        patchBytes,
		BaseSha:      baseSha,
		BaseRef:      baseRef,
		ChangedFiles: files,
		ChangedLines: totalLines,
		FileCount:    fileCount,
	}, nil
}

// listUntracked returns the set of untracked (and not-ignored) paths under
// repoRoot, suitable for `git add -N`.
func listUntracked(ctx context.Context, repoRoot string) ([]string, error) {
	out, err := gitOutput(ctx, repoRoot, "status", "--porcelain=v1", "--untracked-files=normal")
	if err != nil {
		return nil, fmt.Errorf("git status: %w", err)
	}
	var untracked []string
	for _, line := range strings.Split(string(out), "\n") {
		// Porcelain v1: "?? <path>" for untracked.
		if strings.HasPrefix(line, "?? ") {
			p := strings.TrimPrefix(line, "?? ")
			// Unquote if git quoted a path containing special chars.
			if strings.HasPrefix(p, "\"") && strings.HasSuffix(p, "\"") {
				if unq, err := strconv.Unquote(p); err == nil {
					p = unq
				}
			}
			if p != "" {
				untracked = append(untracked, p)
			}
		}
	}
	return untracked, nil
}

// resolveBase turns the user-provided base (or empty → origin/HEAD auto-detect)
// into a resolved commit SHA. Returns (sha, ref) where ref is the original
// input (informational) or "origin/HEAD" for auto-detection.
func resolveBase(ctx context.Context, repoRoot string, userBase string) (string, string, error) {
	if userBase != "" {
		out, err := gitOutput(ctx, repoRoot, "rev-parse", "--verify", userBase+"^{commit}")
		if err != nil {
			// `git rev-parse --verify` writes its failure reason to stderr
			// (e.g. "fatal: Needed a single revision"), which `gitOutput`
			// has already captured into err. `out` (stdout) is empty on
			// failure — using it here would produce an empty reason.
			return "", "", &BaseResolutionError{
				Message: fmt.Sprintf("couldn't resolve --base %q to a commit: %s\n\nTry:\n  tusk review --base origin/main",
					userBase, strings.TrimSpace(err.Error())),
			}
		}
		return strings.TrimSpace(string(out)), userBase, nil
	}

	// Auto-detect via origin/HEAD.
	if err := CheckOriginHead(repoRoot); err != nil {
		return "", "", err
	}
	baseOut, err := gitOutput(ctx, repoRoot, "merge-base", "origin/HEAD", "HEAD")
	if err != nil {
		shallow := isShallow(repoRoot)
		msg := "couldn't determine base commit for this branch.\n\n"
		msg += "Cause: `git merge-base origin/HEAD HEAD` failed — this branch may not share history with origin's default branch."
		if shallow {
			msg += " Also: this is a shallow clone."
		}
		msg += "\n\nThings to try:"
		if shallow {
			msg += "\n  • git fetch --unshallow"
		}
		msg += "\n  • Pass the base explicitly: tusk review --base <branch-or-sha>"
		return "", "", &BaseResolutionError{Message: msg}
	}
	return strings.TrimSpace(string(baseOut)), "origin/HEAD", nil
}

// parseNumstat parses the output of `git diff --numstat -z` into per-file
// summaries. Binary files show "- -" for their counts; we record them as
// zero changed lines but still include them in the file count.
//
// With `-z`, records are NUL-terminated. The added/deleted counts are
// tab-delimited, followed by either:
//   - a single path (non-renamed), OR
//   - three NUL-terminated tokens: "<added>\t<deleted>\t" + "<old>\0<new>\0"
//     for renames/copies (the `-M`/`-C` case — safe to handle even if we
//     don't explicitly enable those flags, since users can via git config).
//
// NUL-terminated parsing is mandatory for correctness: `git diff --numstat`
// without `-z` double-quotes paths containing special characters, and paths
// with embedded whitespace would otherwise be split by any field-based
// parser.
func parseNumstat(out []byte) []FileSummary {
	var summaries []FileSummary
	records := strings.Split(string(out), "\x00")
	i := 0
	for i < len(records) {
		rec := records[i]
		if rec == "" {
			i++
			continue
		}
		// The first record contains "<added>\t<deleted>\t<path-or-empty>".
		// If the trailing path is empty, the next two records are the
		// rename's old and new paths.
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) < 3 {
			i++
			continue
		}
		added := 0
		del := 0
		if n, err := strconv.Atoi(parts[0]); err == nil {
			added = n
		}
		if n, err := strconv.Atoi(parts[1]); err == nil {
			del = n
		}
		path := parts[2]
		if path == "" {
			// Rename/copy: next two records are "old\0new\0". Use the new path.
			if i+2 < len(records) {
				path = records[i+2]
				i += 3
			} else {
				i++
				continue
			}
		} else {
			i++
		}
		if path == "" {
			continue
		}
		summaries = append(summaries, FileSummary{
			Path:       path,
			AddedLines: added,
			DelLines:   del,
		})
	}
	return summaries
}

// topContributors returns up to n file summaries sorted by (added+deleted)
// lines descending, for inclusion in size-cap error messages.
func topContributors(files []FileSummary, n int) []FileSummary {
	sorted := append([]FileSummary{}, files...)
	sort.Slice(sorted, func(i, j int) bool {
		return (sorted[i].AddedLines + sorted[i].DelLines) > (sorted[j].AddedLines + sorted[j].DelLines)
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	return sorted
}

// submoduleModeRe matches `:160000 160000 ...` or `new mode 160000` etc. in
// raw git-diff output. Conservative: we just look for a 160000 octal mode
// token on its own, which only appears for submodule entries.
var (
	submoduleModeRe       = regexp.MustCompile(`(?m)^(?:new file mode|deleted file mode|old mode|new mode|index [^\s]+|similarity index)\b[^\n]*\b160000\b`)
	submoduleHeaderRe     = regexp.MustCompile(`(?m)^diff --git a/(.+?) b/(.+)$`)
	submoduleIndexLineRe  = regexp.MustCompile(`(?m)^index [^\s]+\s+160000\b`)
	submoduleSubprojectRe = regexp.MustCompile(`(?m)^Subproject commit [0-9a-f]+`)
)

// detectSubmodulePaths scans the raw git patch for submodule markers.
// Returns up to ~10 offending paths (deduped) so the error message stays
// readable; the user only needs one example to know what to exclude.
func detectSubmodulePaths(patch []byte) []string {
	// Fast path: if no markers at all, skip the per-file walk.
	if !submoduleModeRe.Match(patch) &&
		!submoduleIndexLineRe.Match(patch) &&
		!submoduleSubprojectRe.Match(patch) {
		return nil
	}

	// Walk file headers; for each file section, scan for a submodule marker.
	var paths []string
	seen := map[string]struct{}{}
	headerIdx := submoduleHeaderRe.FindAllSubmatchIndex(patch, -1)
	for i, idx := range headerIdx {
		start := idx[1]
		end := len(patch)
		if i+1 < len(headerIdx) {
			end = headerIdx[i+1][0]
		}
		section := patch[idx[0]:end]
		if submoduleModeRe.Match(section) ||
			submoduleIndexLineRe.Match(section) ||
			submoduleSubprojectRe.Match(section) {
			path := string(patch[idx[2]:idx[3]])
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				paths = append(paths, path)
				if len(paths) >= 10 {
					break
				}
			}
		}
		_ = start
	}
	return paths
}

// humanBytes formats a byte count as a short human-readable string
// ("1.4MB", "912KB"). Rounded to one decimal for KB/MB.
func humanBytes(n int) string {
	const (
		kb = 1 << 10
		mb = 1 << 20
	)
	switch {
	case n >= mb:
		return fmt.Sprintf("%.1fMB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.1fKB", float64(n)/kb)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

// gitOutput runs git in repoRoot and returns combined stdout (stderr is
// captured and included in the error on failure). The process is launched
// with ctx so callers can cancel or time out in-flight git operations.
func gitOutput(ctx context.Context, repoRoot string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // args are controlled
	cmd.Dir = repoRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// gitRun runs git and returns any error (with stderr captured in the message).
func gitRun(ctx context.Context, repoRoot string, args ...string) error {
	_, err := gitOutput(ctx, repoRoot, args...)
	return err
}
