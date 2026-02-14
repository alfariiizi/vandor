package cmd

import (
	"fmt"
	"os/exec"

	"github.com/spf13/cobra"
)

func newTaskRunnerCmd(use, taskName, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			command := exec.Command("task", taskName)
			command.Stdout = cmd.OutOrStdout()
			command.Stderr = cmd.ErrOrStderr()

			if err := command.Run(); err != nil {
				return fmt.Errorf("failed running task %s: %w", taskName, err)
			}
			return nil
		},
	}
}

func init() {
	RootCmd.AddCommand(newTaskRunnerCmd("dev:app", "dev:app", "Run application in development mode (Taskfile)"))
	RootCmd.AddCommand(newTaskRunnerCmd("dev:worker", "dev:worker", "Run worker in development mode (Taskfile)"))
	RootCmd.AddCommand(newTaskRunnerCmd("run:app", "run:app", "Run application in production mode (Taskfile)"))
	RootCmd.AddCommand(newTaskRunnerCmd("run:worker", "run:worker", "Run worker in production mode (Taskfile)"))
}
