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
