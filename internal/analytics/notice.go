package analytics

import (
	"fmt"
	"os"
	"time"

	"github.com/Use-Tusk/tusk-drift-cli/internal/cliconfig"
	"github.com/Use-Tusk/tusk-drift-cli/internal/log"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const NoticeText = `Tusk CLI collects usage analytics to help improve the product.
Before login, data is anonymous. After login, it's associated with your Tusk Cloud account.
To disable: export TUSK_ANALYTICS_DISABLED=1 or run: tusk config set analytics false`

// ShowFirstRunNotice displays the analytics notice on first run
// Returns true if the notice was shown (and we should track first_run)
func ShowFirstRunNotice(cmd *cobra.Command) bool {
	// Skip if user is running "analytics disable" - don't show notice when they're trying to disable
	if getCommandPath(cmd) == "analytics disable" {
		return false
	}

	// Skip if analytics is disabled (includes CI check)
	if !cliconfig.IsAnalyticsEnabled() {
		return false
	}

	// Skip if not a TTY (piped output)
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return false
	}

	cfg := cliconfig.CLIConfig

	// Skip if already shown
	if cfg.NoticeShown {
		return false
	}

	// Display notice
	log.Println("")
	log.Println(NoticeText)
	log.Println("")

	// Countdown
	for i := 4; i > 0; i-- {
		unit := "seconds"
		if i == 1 {
			unit = "second"
		}
		log.Print(fmt.Sprintf("\rContinuing in %d %s... (Ctrl+C to cancel)", i, unit))
		time.Sleep(1 * time.Second)
	}
	log.Print(fmt.Sprintf("\r%-50s\n", "")) // Clear the countdown line

	// Mark as shown and save
	cfg.NoticeShown = true
	if err := cfg.Save(); err != nil {
		log.Debug("Failed to save config after showing notice", "error", err)
	}

	return true
}
