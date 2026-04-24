//go:build windows

package runner

import "errors"

// Fence only supports Linux and macOS; on Windows the replay sandbox is a
// no-op. Callers treat the error the same as "sandbox not available on this
// platform" on an unsupported Unix.
var errSandboxUnsupportedOnWindows = errors.New("replay sandbox not supported on Windows")

func isSandboxSupported() bool {
	return false
}

func newReplaySandboxManager(_ replaySandboxOptions) (sandboxManager, error) {
	return nil, errSandboxUnsupportedOnWindows
}
