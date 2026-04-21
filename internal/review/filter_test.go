package review

import (
	"strings"
	"testing"
)

// TestFilterListSizes pins list lengths so a CI-side check flags drift
// relative to backend/src/utils/repoUtils.ts. Update these numbers (and
// the lists themselves) when the backend list changes.
func TestFilterListSizes(t *testing.T) {
	if got, want := len(ExtensionsToSkip), 84; got != want {
		t.Errorf("ExtensionsToSkip size = %d; want %d (update list or test if backend changed)", got, want)
	}
	if got, want := len(FilesToSkip), 9; got != want {
		t.Errorf("FilesToSkip size = %d; want %d", got, want)
	}
	if got, want := len(DirectoriesToSkip), 37; got != want {
		t.Errorf("DirectoriesToSkip size = %d; want %d", got, want)
	}
}

func TestBuildPathspecExclusions_IncludesDefaults(t *testing.T) {
	specs := BuildPathspecExclusions(nil, nil)
	if len(specs) == 0 {
		t.Fatal("expected at least some default pathspec exclusions")
	}
	// Every entry should be prefixed with the pathspec magic.
	for _, s := range specs {
		if !strings.HasPrefix(s, ":(exclude,glob)") {
			t.Errorf("pathspec not properly prefixed: %q", s)
		}
	}
	// Sanity-check a handful of well-known defaults made it in.
	joined := strings.Join(specs, "\n")
	for _, want := range []string{
		"**/package-lock.json",
		"**/*.png",
		"**/node_modules/**",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected pathspec list to contain %q", want)
		}
	}
}

func TestBuildPathspecExclusions_ExtraExcludes(t *testing.T) {
	specs := BuildPathspecExclusions([]string{"docs/**", "*.generated.go"}, nil)
	joined := strings.Join(specs, "\n")
	if !strings.Contains(joined, "docs/**") {
		t.Errorf("missing extra exclude docs/**")
	}
	if !strings.Contains(joined, "*.generated.go") {
		t.Errorf("missing extra exclude *.generated.go")
	}
}

func TestBuildPathspecExclusions_IncludeCancelsDefault(t *testing.T) {
	// Ask to keep package-lock.json — it should be removed from the default
	// exclusion list rather than forcing a separate include pathspec.
	specs := BuildPathspecExclusions(nil, []string{"package-lock.json"})
	joined := strings.Join(specs, "\n")
	if strings.Contains(joined, "**/package-lock.json") {
		t.Errorf("expected --include 'package-lock.json' to drop the default exclusion; got %q", joined)
	}
	// Other defaults should remain.
	if !strings.Contains(joined, "**/*.png") {
		t.Errorf("unrelated default was unexpectedly dropped")
	}
}
