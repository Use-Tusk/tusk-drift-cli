//go:build darwin || linux || freebsd

package runner

import (
	"fmt"
	"strings"

	"github.com/Use-Tusk/fence/pkg/fence"
	"github.com/Use-Tusk/tusk-cli/internal/utils"
)

// isSandboxSupported reports whether the current platform can actually
// isolate replay service startup (i.e. fence is available).
func isSandboxSupported() bool {
	return fence.IsSupported()
}

// fenceSandbox is the Unix-platform implementation of sandboxManager,
// backed by github.com/Use-Tusk/fence.
type fenceSandbox struct {
	mgr *fence.Manager
}

// WrapCommand delegates to the underlying fence.Manager.
func (s *fenceSandbox) WrapCommand(command string) (string, error) {
	return s.mgr.WrapCommand(command)
}

// Cleanup releases fence's socat bridges, proxies, and temp sockets.
func (s *fenceSandbox) Cleanup() {
	if s.mgr != nil {
		s.mgr.Cleanup()
	}
}

// newReplaySandboxManager builds the effective fence config for replay
// mode, creates the fence.Manager, applies the requested service
// execution model + exposed host paths, and initializes the manager.
// On any error after fence.NewManager succeeds, the manager's Cleanup is
// invoked before returning so no fence-allocated resources leak.
func newReplaySandboxManager(opts replaySandboxOptions) (sandboxManager, error) {
	fenceCfg, err := createReplayFenceConfig(opts.UserConfigPath)
	if err != nil {
		// No prefix here: service.go adds the user-facing
		// "failed to prepare replay sandbox config:" framing.
		return nil, &sandboxConfigError{err: err}
	}

	mgr := fence.NewManager(fenceCfg, opts.Debug, false)
	// Defensive: Cleanup is idempotent and fence's Initialize already
	// unwinds its own partial state on failure, but this guards against
	// future fence changes that add allocating steps between NewManager
	// and Initialize (or between error returns inside Initialize).
	success := false
	defer func() {
		if !success {
			mgr.Cleanup()
		}
	}()

	executionModel := fence.ServiceBindsInSandbox
	if opts.BindsOnHost {
		executionModel = fence.ServiceBindsOnHost
	}
	mgr.SetService(fence.ServiceOptions{
		ExposedPorts:   []int{opts.ExposedPort},
		ExecutionModel: executionModel,
	})

	for _, ehp := range opts.ExposedHostPaths {
		if err := mgr.ExposeHostPath(ehp.Path, ehp.Writable); err != nil {
			return nil, fmt.Errorf("expose host path %q to sandbox: %w", ehp.Path, err)
		}
	}

	if err := mgr.Initialize(); err != nil {
		return nil, fmt.Errorf("initialize replay sandbox: %w", err)
	}

	success = true
	return &fenceSandbox{mgr: mgr}, nil
}

// createReplayFenceConfig creates the effective fence config for replay mode.
// This blocks localhost outbound connections to force the service to use SDK
// mocks.
//
// Exposed (lowercase) for the Unix-only service_test.go tests that verify
// user-config merging behavior. Not part of the package's cross-platform
// surface.
func createReplayFenceConfig(userConfigPath string) (*fence.Config, error) {
	cfg := baseReplayFenceConfig()
	if userConfigPath == "" {
		return cfg, nil
	}

	resolvedPath := utils.ResolveTuskPath(userConfigPath)
	userCfg, err := fence.LoadConfigResolved(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("load custom fence config %q: %w", resolvedPath, err)
	}
	if userCfg == nil {
		return nil, fmt.Errorf("custom fence config not found: %s", resolvedPath)
	}
	if err := validateReplayFenceConfig(userCfg); err != nil {
		return nil, err
	}

	merged := fence.MergeConfigs(cfg, userCfg)
	applyReplayFenceInvariants(merged)
	return merged, nil
}

func baseReplayFenceConfig() *fence.Config {
	f := false
	return &fence.Config{
		Network: fence.NetworkConfig{
			AllowedDomains: []string{
				// Allow localhost for the service's own health checks
				"localhost",
				"127.0.0.1",
			},
			AllowLocalBinding:   true, // Allow service to bind to its port
			AllowLocalOutbound:  &f,   // Block outbound to localhost (Postgres, Redis, etc.)
			AllowAllUnixSockets: true, // Allow SDK to connect to mock server via Unix socket
		},
		Filesystem: fence.FilesystemConfig{
			AllowWrite: getAllowedWriteDirs(),
		},
	}
}

func validateReplayFenceConfig(cfg *fence.Config) error {
	if cfg == nil {
		return nil
	}

	requiredDomains := []string{"localhost", "127.0.0.1"}
	for _, deniedDomain := range cfg.Network.DeniedDomains {
		for _, requiredDomain := range requiredDomains {
			if strings.EqualFold(deniedDomain, requiredDomain) {
				return fmt.Errorf("custom replay fence config cannot deny %q because replay health checks require it", requiredDomain)
			}
		}
	}

	return nil
}

func applyReplayFenceInvariants(cfg *fence.Config) {
	if cfg == nil {
		return
	}

	f := false
	cfg.Network.AllowedDomains = mergeUniqueStrings(
		cfg.Network.AllowedDomains,
		[]string{"localhost", "127.0.0.1"},
	)
	cfg.Network.AllowLocalBinding = true
	cfg.Network.AllowLocalOutbound = &f
	cfg.Network.AllowAllUnixSockets = true
	cfg.Filesystem.AllowWrite = mergeUniqueStrings(cfg.Filesystem.AllowWrite, getAllowedWriteDirs())
}

func mergeUniqueStrings(existing, required []string) []string {
	if len(required) == 0 {
		return existing
	}

	seen := make(map[string]struct{}, len(existing)+len(required))
	merged := make([]string, 0, len(existing)+len(required))
	for _, value := range existing {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	for _, value := range required {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged
}

// getAllowedWriteDirs returns the default writable paths for replay mode.
// We allow broad local writes by default. Note that Fence still enforces
// mandatory dangerous-path protections (see
// https://github.com/Use-Tusk/fence/blob/main/internal/sandbox/dangerous.go).
func getAllowedWriteDirs() []string {
	return []string{
		"/",
	}
}
