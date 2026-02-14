package cmd

import (
	"fmt"

	"github.com/alfariiizi/vandor/internal/coregen"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync generated core registries",
}

var syncPath string

var syncCoreCmd = &cobra.Command{
	Use:   "core",
	Short: "Sync core generated files",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(syncPath)
		if err != nil {
			return err
		}
		contexts, err := coregen.SyncCore(root)
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Synced core (%d context(s))", len(contexts)), map[string]any{
			"contexts": contexts,
		})
	},
}

var syncAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Sync all generated registries",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(syncPath)
		if err != nil {
			return err
		}
		contexts, err := coregen.SyncAll(root)
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Synced all (%d context(s))", len(contexts)), map[string]any{
			"contexts": contexts,
		})
	},
}

var syncContextCmd = &cobra.Command{
	Use:   "context <name>",
	Short: "Sync core generated files for a specific context",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(syncPath)
		if err != nil {
			return err
		}
		contexts, err := coregen.SyncContext(root, args[0])
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Synced context %s", args[0]), map[string]any{
			"contexts": contexts,
		})
	},
}

func init() {
	syncCoreCmd.Flags().StringVar(&syncPath, "path", ".", "Project root path")
	syncAllCmd.Flags().StringVar(&syncPath, "path", ".", "Project root path")
	syncContextCmd.Flags().StringVar(&syncPath, "path", ".", "Project root path")

	syncCmd.AddCommand(syncCoreCmd)
	syncCmd.AddCommand(syncContextCmd)
	syncCmd.AddCommand(syncAllCmd)

	RootCmd.AddCommand(syncCmd)
}
