package cmd

// ckeletin:allow-custom-command

import (
	"fmt"
	"os"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open Vandor terminal UI (Phase-1 read-only placeholder)",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !isatty.IsTerminal(os.Stdin.Fd()) || !isatty.IsTerminal(os.Stdout.Fd()) {
			return fmt.Errorf("vandor tui requires a TTY; use non-interactive commands like `vandor add ...` or `vandor sync ...`")
		}

		return emitResult(cmd, "TUI Phase-1 placeholder. Use CLI source-of-truth commands for write actions.", map[string]any{
			"hint": "Use `vandor add`, `vandor sync`, `vandor vpkg` commands directly.",
		})
	},
}

func init() {
	RootCmd.AddCommand(tuiCmd)
}
