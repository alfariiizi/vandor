package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""

	binaryName = "vandor"

	interactiveMode  bool
	interactiveAlias bool
	noInputMode      bool
	assumeYes        bool
	outputMode       string
)

var RootCmd = &cobra.Command{
	Use:   binaryName,
	Short: "Vandor CLI for core-first Go backend scaffolding",
	Long: `vandor is the core-first CLI for building Go backends with
hexagonal architecture + DDD boundaries and vpkg-based transport/infrastructure wiring.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if interactiveAlias {
			interactiveMode = true
		}
		if interactiveMode && noInputMode {
			return fmt.Errorf("flags --interactive and --no-input cannot be used together")
		}
		if outputMode != "text" && outputMode != "json" {
			return fmt.Errorf("invalid --output %q, allowed values: text, json", outputMode)
		}
		return nil
	},
}

func Execute() error {
	RootCmd.Version = fmt.Sprintf("%s, commit %s, built at %s", Version, Commit, Date)
	return RootCmd.Execute()
}

func init() {
	RootCmd.PersistentFlags().BoolVarP(&interactiveMode, "interactive", "i", false, "Enable interactive prompt mode")
	RootCmd.PersistentFlags().BoolVarP(&interactiveAlias, "interactive-alias", "t", false, "Alias flag used so '-it' works for interactive mode")
	if err := RootCmd.PersistentFlags().MarkHidden("interactive-alias"); err != nil {
		panic(err)
	}
	RootCmd.PersistentFlags().BoolVar(&noInputMode, "no-input", false, "Disable prompts and fail fast on missing args")
	RootCmd.PersistentFlags().BoolVar(&assumeYes, "yes", false, "Skip confirmation prompts")
	RootCmd.PersistentFlags().StringVar(&outputMode, "output", "text", "Output mode: text|json")
}
