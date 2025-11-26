package cmd

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/Use-Tusk/tusk-drift-cli/internal/analytics"
	"github.com/Use-Tusk/tusk-drift-cli/internal/tui/styles"
	"github.com/Use-Tusk/tusk-drift-cli/internal/utils"
	"github.com/Use-Tusk/tusk-drift-cli/internal/version"
	"github.com/spf13/cobra"
)

var (
	cfgFile     string
	debug       bool
	showVersion bool
	logger      *slog.Logger

	// Cleanup infrastructure
	cleanupFuncs []func()
	cleanupMutex sync.Mutex
	signalSetup  sync.Once

	// Analytics tracker for the current command
	tracker *analytics.Tracker
)

//go:embed short_docs/overview.md
var overviewContent string

var rootCmd = &cobra.Command{
	Use:   "tusk",
	Short: "Tusk CLI - API test record/replay system",
	Long:  utils.RenderMarkdown(overviewContent),
	Run: func(cmd *cobra.Command, args []string) {
		showASCIIArt()
	},
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if showVersion {
			version.PrintVersion()
			os.Exit(0)
		}
		setupLogger()

		// Initialize analytics tracker
		tracker = analytics.NewTracker(cmd)

		return nil
	},
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

func Execute() error {
	return rootCmd.Execute()
}

// GetTracker returns the analytics tracker for the current command
// Used by main.go to track the command result
func GetTracker() *analytics.Tracker {
	return tracker
}

func showASCIIArt() {
	purple := ""
	reset := ""

	if os.Getenv("NO_COLOR") == "" {
		purple = "\x1b[38;5;053m"
		if styles.HasDarkBackground {
			purple = "\x1b[38;5;213m"
		}
		reset = "\033[0m"
	}

	banner := []string{
		"  dBBBBBBP dBP dBP.dBBBBP   dBP dBP    dBBBBb dBBBBBb    dBP dBBBBP dBBBBBBP",
		"                  BP       d8P.dBP        dB'     dBP                       ",
		"   dBP   dBP dBP  `BBBBb  dBBBBP     dBP dB'  dBBBBK   dBP dBBBP     dBP    ",
		"  dBP   dBP_dBP      dBP dBP BB     dBP dB'  dBP  BB  dBP dBP       dBP     ",
		" dBP   dBBBBBP  dBBBBP' dBP dBP    dBBBBB'  dBP  dB' dBP dBP       dBP      ",
		"                                                                            ",
	}

	maxWidth := 0

	// Check banner width
	for _, line := range banner {
		if len(line) > maxWidth {
			maxWidth = len(line)
		}
	}

	fmt.Print(purple + "\n")

	// Center and print banner
	for _, line := range banner {
		bannerPadding := max((maxWidth-len(line))/2, 0)
		for range bannerPadding {
			fmt.Print(" ")
		}
		fmt.Println(line)
	}

	fmt.Print(reset)

	// Center subtitle
	subtitle := "API test record/replay system"
	subtitlePadding := max((maxWidth-len(subtitle))/2, 0)
	for range subtitlePadding {
		fmt.Print(" ")
	}
	fmt.Print(subtitle)
	fmt.Print("\n")

	// Center version
	versionText := fmt.Sprintf("Version: %s", version.Version)
	versionPadding := max((maxWidth-len(versionText))/2, 0)
	for range versionPadding {
		fmt.Print(" ")
	}
	fmt.Print(versionText)
	fmt.Print("\n\n")

	fmt.Println("Use \"tusk --help\" for more information about available commands.")
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is .tusk/config.yaml)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "debug output")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "show version and exit")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "ver", "V", false, "show version and exit")

	_ = rootCmd.PersistentFlags().MarkHidden("ver")

	// Note: Analytics cleanup happens in main.go after TrackResult is called
}

func setupLogger() {
	level := slog.LevelInfo
	if debug {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewTextHandler(os.Stderr, opts)

	logger = slog.New(handler)
	slog.SetDefault(logger)
}

// RegisterCleanup adds a cleanup function to be called on program termination
func RegisterCleanup(fn func()) {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()
	cleanupFuncs = append(cleanupFuncs, fn)
}

// runCleanup executes all registered cleanup functions
func runCleanup() {
	cleanupMutex.Lock()
	defer cleanupMutex.Unlock()

	slog.Debug("Running cleanup functions", "count", len(cleanupFuncs))
	for i, fn := range cleanupFuncs {
		slog.Debug("Running cleanup function", "index", i)
		fn()
	}
	cleanupFuncs = nil // Clear the slice
}

// setupSignalHandling sets up signal handlers for graceful shutdown
func setupSignalHandling() {
	signalSetup.Do(func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		go func() {
			sig := <-c
			fmt.Fprintf(os.Stderr, "Received %s signal, cleaning up\n", sig)
			runCleanup()
			os.Exit(1)
		}()

		slog.Debug("Signal handling setup complete")
	})
}
