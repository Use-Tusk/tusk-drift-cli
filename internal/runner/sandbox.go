package runner

// Platform-split sandbox adapter. Real implementation lives in sandbox_unix.go
// (fence-backed); Windows gets a no-op stub in sandbox_windows.go because
// fence doesn't cross-compile there.

// sandboxManager wraps whatever sandbox backs replay isolation on the current
// platform. Nil means no sandbox configured.
type sandboxManager interface {
	WrapCommand(command string) (string, error)
	Cleanup()
}

// sandboxConfigError marks errors that stem from invalid user sandbox config
// (bad JSON, denied localhost, missing file). These are always fatal
// regardless of sandbox mode — a user who supplied a broken config asked for
// sandboxing and shouldn't silently get unisolated execution. Distinct from
// runtime-availability errors (missing bwrap/socat, Initialize failure), which
// auto mode treats as "fall back to no sandbox".
type sandboxConfigError struct{ err error }

func (e *sandboxConfigError) Error() string { return e.err.Error() }
func (e *sandboxConfigError) Unwrap() error { return e.err }

type replaySandboxOptions struct {
	UserConfigPath string // optional fence config override (e.g. .tusk/replay.fence.json)
	Debug          bool
	ExposedPort    int
	// BindsOnHost signals that an external daemon (docker, podman) binds
	// ExposedPort outside the sandbox netns; skips the reverse bridge.
	BindsOnHost      bool
	ExposedHostPaths []exposedHostPath
}

type exposedHostPath struct {
	Path     string
	Writable bool
}
