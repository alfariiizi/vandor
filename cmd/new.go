package cmd

// ckeletin:allow-custom-command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alfariiizi/vandor/internal/coregen"
	"github.com/spf13/cobra"
)

var (
	newBasePath string
	newModule   string
	newForce    bool
	newTidyMode string
)

var newCmd = &cobra.Command{
	Use:   "new <project-name>",
	Short: "Create a new core-only Vandor project",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		basePath, err := resolvePath(newBasePath)
		if err != nil {
			return err
		}

		targetDir, err := coregen.InitProject(basePath, args[0], newModule, newForce)
		if err != nil {
			return err
		}
		effectiveModule, err := readModuleFromGoMod(targetDir)
		if err != nil {
			return err
		}

		tidyStatus, err := runTidy(targetDir, newTidyMode)
		if err != nil {
			return err
		}

		return emitResult(cmd, fmt.Sprintf("Created project at %s", targetDir), map[string]any{
			"path":        targetDir,
			"module":      effectiveModule,
			"tidy_status": tidyStatus,
		})
	},
}

func runTidy(projectDir, mode string) (string, error) {
	switch mode {
	case "never":
		return "skipped (mode=never)", nil
	case "auto", "always":
		command := exec.Command("go", "mod", "tidy")
		command.Dir = projectDir
		output, err := command.CombinedOutput()
		if err == nil {
			return "success", nil
		}

		errMsg := strings.TrimSpace(string(output))
		if errMsg == "" {
			errMsg = err.Error()
		}

		if mode == "always" {
			return "", fmt.Errorf("go mod tidy failed: %s", errMsg)
		}
		return fmt.Sprintf("failed (non-fatal): %s", errMsg), nil
	default:
		return "", fmt.Errorf("invalid --tidy value %q, allowed values: auto, always, never", mode)
	}
}

func readModuleFromGoMod(projectDir string) (string, error) {
	// nosemgrep: go-path-traversal -- projectDir is resolved and created by vandor init flow.
	data, err := os.ReadFile(filepath.Join(projectDir, "go.mod"))
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	return "", fmt.Errorf("go.mod module path not found in %s", projectDir)
}

func init() {
	newCmd.Flags().StringVar(&newBasePath, "path", ".", "Base directory to create the project in")
	newCmd.Flags().StringVar(&newModule, "module", "", "Go module path (default: github.com/yourorg/<project>)")
	newCmd.Flags().BoolVar(&newForce, "force", false, "Allow writing into a non-empty target directory")
	newCmd.Flags().StringVar(&newTidyMode, "tidy", "auto", "Run go mod tidy mode: auto|always|never")

	RootCmd.AddCommand(newCmd)
}
