package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var vpkgCmd = &cobra.Command{
	Use:   "vpkg",
	Short: "Manage transport/infrastructure packages",
	Long:  "Vpkg commands are the extension surface for transport/infrastructure setup.",
}

func newPhase2Placeholder(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("%s (Phase-2)", name),
		RunE: func(cmd *cobra.Command, args []string) error {
			return emitResult(cmd, fmt.Sprintf("vpkg %s is planned for Phase-2", name), map[string]any{
				"phase": 2,
			})
		},
	}
}

func init() {
	vpkgCmd.AddCommand(newPhase2Placeholder("add"))
	vpkgCmd.AddCommand(newPhase2Placeholder("remove"))
	vpkgCmd.AddCommand(newPhase2Placeholder("list"))
	vpkgCmd.AddCommand(newPhase2Placeholder("sync"))
	vpkgCmd.AddCommand(newPhase2Placeholder("doctor"))
	RootCmd.AddCommand(vpkgCmd)
}
