package review

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Use-Tusk/tusk-cli/internal/log"
)

// PreflightError is returned for user-actionable pre-flight failures (e.g.
// mid-rebase, no origin/HEAD, shallow). The CLI renders the Message
// verbatim and exits with code 2.
type PreflightError struct {
	Message string
}

func (e *PreflightError) Error() string { return e.Message }

// IsPreflightError reports whether err (or anything it wraps) is a
// *PreflightError.
func IsPreflightError(err error) bool {
	var p *PreflightError
	return errors.As(err, &p)
}

// GitDir returns the path to the .git directory (or worktree gitdir file) for
// the repository at repoRoot.
func GitDir(repoRoot string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-dir") //nolint:gosec
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --git-dir: %w", err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repoRoot, gitDir)
	}
	return gitDir, nil
}

// RepoRoot returns the top-level directory of the git repository that the
// current working directory is inside, or an error if cwd isn't a repo.
func RepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output() //nolint:gosec
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Preflight runs the quick local checks that must pass before we try to
// generate a patch. It returns a *PreflightError for user-actionable failures
// and a plain error for unexpected ones (e.g. git not on PATH).
//
// Emits a stderr warning (not an error) for detached HEAD. Detached HEAD
// doesn't prevent patch generation; the user is just told their results may
// look off.
func Preflight(repoRoot string) error {
	gitDir, err := GitDir(repoRoot)
	if err != nil {
		return err
	}

	// Mid-operation detection. Each of these produces a specific, user-actionable error.
	midOps := []struct {
		path string
		name string
		next string
	}{
		{filepath.Join(gitDir, "rebase-merge"), "rebase", "git rebase --continue` or `git rebase --abort"},
		{filepath.Join(gitDir, "rebase-apply"), "rebase", "git rebase --continue` or `git rebase --abort"},
		{filepath.Join(gitDir, "MERGE_HEAD"), "merge", "git merge --continue` or `git merge --abort"},
		{filepath.Join(gitDir, "CHERRY_PICK_HEAD"), "cherry-pick", "git cherry-pick --continue` or `git cherry-pick --abort"},
		{filepath.Join(gitDir, "REVERT_HEAD"), "revert", "git revert --continue` or `git revert --abort"},
	}
	for _, op := range midOps {
		if _, err := os.Stat(op.path); err == nil {
			return &PreflightError{
				Message: fmt.Sprintf(
					"working tree is mid-%s. Finish (`%s`) before running `tusk review run`.",
					op.name, op.next),
			}
		}
	}

	// `origin` presence is NOT checked here — either:
	//   - the user passed --repo + --base (no origin needed at all), OR
	//   - the repo-identity step (resolveReviewRepo) will fail with a
	//     specific "run inside a git repo with an origin remote" message, OR
	//   - the base-resolution step will call CheckOriginHead for a targeted
	//     "origin/HEAD not set" error.
	// Firing a generic origin-missing error in preflight would block the
	// --repo + --base bypass that this command is designed to allow.

	// Detached HEAD is a warning, not a refusal.
	//
	// `git symbolic-ref -q HEAD` exits 1 *only* when HEAD is not a symbolic
	// ref (i.e. detached); other failure modes (corrupt repo, git missing)
	// exit 128. We only want to surface the detached-head warning in the
	// exit-1 case — treating any non-zero exit as "detached" would
	// mis-diagnose those other failures.
	if detached, err := isDetachedHEAD(repoRoot); err != nil {
		// Don't block the command on an unexpected symbolic-ref failure;
		// the real patch-generation step will surface a better error if
		// the repo is broken. Do log for debuggability.
		log.Debug("symbolic-ref HEAD check failed", "error", err)
	} else if detached {
		_, _ = fmt.Fprintln(os.Stderr, "warning: HEAD is detached; review results may look odd.")
	}

	return nil
}

// isDetachedHEAD reports whether HEAD is currently detached. Returns
// (false, err) for unexpected git failures so callers can distinguish
// "definitely not detached" from "can't tell".
func isDetachedHEAD(repoRoot string) (bool, error) {
	cmd := exec.Command("git", "symbolic-ref", "-q", "HEAD") //nolint:gosec
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Exit 1 with `-q` is the documented "not a symbolic ref" signal.
			if exitErr.ExitCode() == 1 {
				return true, nil
			}
		}
		return false, fmt.Errorf("git symbolic-ref -q HEAD: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return false, nil
}

// CheckOriginHead confirms that `origin/HEAD` is set on this clone, which
// `git merge-base` relies on for auto-detecting the base branch. Returns a
// *PreflightError with remediation text when missing.
//
// Only called when the user did NOT pass `--base` — explicit base bypasses
// the origin/HEAD requirement.
func CheckOriginHead(repoRoot string) error {
	if err := runGitSilent(repoRoot, "symbolic-ref", "refs/remotes/origin/HEAD"); err != nil {
		shallow := isShallow(repoRoot)
		msg := "couldn't determine base commit for this branch.\n\n"
		msg += "Cause: this clone has no `origin/HEAD` ref."
		if shallow {
			msg += " Also: this is a shallow clone."
		}
		msg += "\n\nFix: git remote set-head origin --auto"
		if shallow {
			msg += "\n     git fetch --unshallow"
		}
		msg += "\n\nOr pass --base explicitly:\n  tusk review run --base main"
		return &PreflightError{Message: msg}
	}
	return nil
}

func isShallow(repoRoot string) bool {
	gitDir, err := GitDir(repoRoot)
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(gitDir, "shallow"))
	return err == nil
}

// runGitSilent runs git with the given args in repoRoot and returns an error
// if the process exits non-zero (or fails to start). stderr is captured but
// not printed.
func runGitSilent(repoRoot string, args ...string) error {
	cmd := exec.Command("git", args...) //nolint:gosec // args are controlled
	cmd.Dir = repoRoot
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
