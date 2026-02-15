package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/alfariiizi/vandor/internal/vpkg"
	"github.com/spf13/cobra"
)

var vpkgCmd = &cobra.Command{
	Use:   "vpkg",
	Short: "Manage transport/infrastructure packages",
	Long:  "Vpkg commands are the extension surface for transport/infrastructure setup.",
}

var vpkgPath string
var vpkgAddPlanOnly bool

var vpkgAddCmd = &cobra.Command{
	Use:   "add <source>",
	Short: "Install a vpkg package from local path or registry alias",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		previewMode := vpkgAddPlanOnly || (interactiveMode && !assumeYes)
		if previewMode {
			preview, previewErr := manager.Add(args[0], vpkg.AddOptions{PlanOnly: true})
			if previewErr != nil {
				return previewErr
			}
			if outputMode == "text" && len(preview.Plan) > 0 {
				printPlan(cmd, preview.Plan)
			}
			if vpkgAddPlanOnly {
				return emitResult(cmd, fmt.Sprintf("Planned package %s@%s (dry-run)", preview.Name, preview.Version), preview)
			}
			if err := confirmApplyPlan(cmd); err != nil {
				return err
			}
		}
		result, err := manager.Add(args[0], vpkg.AddOptions{})
		if err != nil {
			return err
		}
		if outputMode == "text" && !previewMode && !assumeYes && len(result.Plan) > 0 {
			printPlan(cmd, result.Plan)
		}
		return emitResult(cmd, fmt.Sprintf("Installed package %s@%s", result.Name, result.Version), result)
	},
}

var (
	vpkgRemoveForce bool
)

var vpkgRemoveCmd = &cobra.Command{
	Use:   "remove <package>",
	Short: "Remove an installed vpkg package by name",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		result, err := manager.Remove(args[0], vpkg.RemoveOptions{Force: vpkgRemoveForce})
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Removed package %s", result.Name), result)
	},
}

var vpkgListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed vpkg packages from lockfile",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		items, err := manager.List()
		if err != nil {
			return err
		}
		if outputMode == "json" {
			return emitResult(cmd, "Listed installed packages", map[string]any{
				"packages": items,
				"count":    len(items),
			})
		}
		if len(items) == 0 {
			return emitResult(cmd, "No installed vpkg packages", map[string]any{"count": 0})
		}
		lines := make([]string, 0, len(items)+1)
		lines = append(lines, "Installed vpkg packages:")
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("- %s@%s (%s/%s) source=%s", item.Name, item.Version, item.Tier, item.Kind, item.Source))
		}
		return emitResult(cmd, strings.Join(lines, "\n"), map[string]any{
			"packages": items,
			"count":    len(items),
		})
	},
}

var vpkgSearchTier string
var vpkgSearchLimit int
var vpkgSearchOffset int
var vpkgDoctorFix bool

var vpkgSearchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search available vpkg packages from configured registries",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		query := ""
		if len(args) > 0 {
			query = args[0]
		}
		items, err := manager.Search(query, vpkgSearchTier)
		if err != nil {
			return err
		}
		pagedItems, total, err := paginateSearchItems(items, vpkgSearchOffset, vpkgSearchLimit)
		if err != nil {
			return err
		}
		if outputMode == "json" {
			return emitResult(cmd, "Searched vpkg packages", map[string]any{
				"packages": pagedItems,
				"count":    len(pagedItems),
				"total":    total,
				"query":    query,
				"tier":     vpkgSearchTier,
				"offset":   vpkgSearchOffset,
				"limit":    vpkgSearchLimit,
			})
		}
		if len(pagedItems) == 0 {
			return emitResult(cmd, "No vpkg packages found", map[string]any{
				"count":  0,
				"total":  total,
				"query":  query,
				"tier":   vpkgSearchTier,
				"offset": vpkgSearchOffset,
				"limit":  vpkgSearchLimit,
			})
		}
		lines := make([]string, 0, len(pagedItems)+2)
		lines = append(lines, fmt.Sprintf("Available vpkg packages (showing %d of %d):", len(pagedItems), total))
		for _, item := range pagedItems {
			lines = append(lines, fmt.Sprintf("- %s@%s [%s] registry=%s", item.Name, item.Latest, item.Tier, item.Registry))
		}
		return emitResult(cmd, strings.Join(lines, "\n"), map[string]any{
			"packages": pagedItems,
			"count":    len(pagedItems),
			"total":    total,
			"query":    query,
			"tier":     vpkgSearchTier,
			"offset":   vpkgSearchOffset,
			"limit":    vpkgSearchLimit,
		})
	},
}

var vpkgSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Re-apply installed vpkg packages from lockfile cache",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		result, err := manager.Sync()
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Synced %d package(s)", result.Packages), result)
	},
}

var vpkgDoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check vpkg integrity, lock drift, and ownership status",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		report, err := manager.Doctor()
		if err != nil {
			return err
		}
		if report.Healthy {
			return emitResult(cmd, "vpkg doctor: healthy", report)
		}
		if vpkgDoctorFix {
			syncResult, syncErr := manager.Sync()
			if syncErr != nil {
				return syncErr
			}
			fixedReport, fixedErr := manager.Doctor()
			if fixedErr != nil {
				return fixedErr
			}
			payload := map[string]any{
				"before": report,
				"after":  fixedReport,
				"sync":   syncResult,
			}
			if fixedReport.Healthy {
				return emitResult(cmd, fmt.Sprintf("vpkg doctor fixed %d issue(s)", len(report.Issues)), payload)
			}
			if outputMode == "text" {
				lines := []string{
					fmt.Sprintf("vpkg doctor --fix unresolved: before=%d issue(s), after=%d issue(s)", len(report.Issues), len(fixedReport.Issues)),
				}
				lines = append(lines, renderDoctorIssueLines(fixedReport)...)
				if err := emitResult(cmd, strings.Join(lines, "\n"), payload); err != nil {
					return err
				}
			} else {
				if err := emitResult(cmd, fmt.Sprintf("vpkg doctor --fix unresolved: %d issue(s)", len(fixedReport.Issues)), payload); err != nil {
					return err
				}
			}
			return fmt.Errorf("vpkg doctor --fix failed")
		}
		if outputMode == "text" {
			lines := []string{fmt.Sprintf("vpkg doctor found %d issue(s):", len(report.Issues))}
			lines = append(lines, renderDoctorIssueLines(report)...)
			if err := emitResult(cmd, strings.Join(lines, "\n"), report); err != nil {
				return err
			}
		} else {
			if err := emitResult(cmd, fmt.Sprintf("vpkg doctor found %d issue(s)", len(report.Issues)), report); err != nil {
				return err
			}
		}
		return fmt.Errorf("vpkg doctor failed")
	},
}

var vpkgExecCmd = &cobra.Command{
	Use:   "exec <package> <action> [args...]",
	Short: "Execute package action from installed vpkg cache",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		result, err := manager.Exec(args[0], args[1], args[2:], cmd.OutOrStdout(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Executed %s:%s", result.Package, result.Action), result)
	},
}

var vpkgExecAliasCmd = &cobra.Command{
	Use:   "exec-alias <alias> [args...]",
	Short: "Execute package alias from installed vpkg lock",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		result, err := manager.ExecAlias(args[0], args[1:], cmd.OutOrStdout(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Executed alias %s (%s:%s)", args[0], result.Package, result.Action), result)
	},
}

var vpkgRegistryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage vpkg registries in vpkg.yaml",
}

var vpkgRegistryAddCmd = &cobra.Command{
	Use:   "add <name> <url>",
	Short: "Add or update registry entry",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		entry, err := manager.RegistryAdd(args[0], args[1])
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Upserted registry %s", entry.Name), entry)
	},
}

var vpkgRegistryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured registries",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		items, err := manager.RegistryList()
		if err != nil {
			return err
		}
		if outputMode == "json" {
			return emitResult(cmd, "Listed registries", map[string]any{
				"registries": items,
				"count":      len(items),
			})
		}
		if len(items) == 0 {
			return emitResult(cmd, "No registries configured", map[string]any{"count": 0})
		}
		lines := make([]string, 0, len(items)+1)
		lines = append(lines, "Configured registries:")
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("- %s => %s", item.Name, item.URL))
		}
		return emitResult(cmd, strings.Join(lines, "\n"), map[string]any{
			"registries": items,
			"count":      len(items),
		})
	},
}

var vpkgRegistryRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove registry entry",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(vpkgPath)
		if err != nil {
			return err
		}
		manager := vpkg.NewManager(root)
		entry, err := manager.RegistryRemove(args[0])
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Removed registry %s", entry.Name), entry)
	},
}

func init() {
	vpkgAddCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgAddCmd.Flags().BoolVar(&vpkgAddPlanOnly, "plan", false, "Preview install plan only (dry-run, no file writes)")
	vpkgRemoveCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgRemoveCmd.Flags().BoolVar(&vpkgRemoveForce, "force", false, "Remove package even when ownership files are drifted")
	vpkgListCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgSearchCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgSearchCmd.Flags().StringVar(&vpkgSearchTier, "tier", "", "Filter by tier: official|verified|community")
	vpkgSearchCmd.Flags().IntVar(&vpkgSearchLimit, "limit", 10, "Limit number of search results (default 10; use 0 for all)")
	vpkgSearchCmd.Flags().IntVar(&vpkgSearchOffset, "offset", 0, "Skip N search results before showing output")
	vpkgSyncCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgDoctorCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgDoctorCmd.Flags().BoolVar(&vpkgDoctorFix, "fix", false, "Attempt safe auto-repair by re-syncing installed packages from lock cache")
	vpkgExecCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgExecAliasCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")

	vpkgCmd.AddCommand(vpkgAddCmd)
	vpkgCmd.AddCommand(vpkgRemoveCmd)
	vpkgCmd.AddCommand(vpkgListCmd)
	vpkgCmd.AddCommand(vpkgSearchCmd)
	vpkgCmd.AddCommand(vpkgSyncCmd)
	vpkgCmd.AddCommand(vpkgDoctorCmd)
	vpkgCmd.AddCommand(vpkgExecCmd)
	vpkgCmd.AddCommand(vpkgExecAliasCmd)
	vpkgRegistryCmd.AddCommand(vpkgRegistryAddCmd)
	vpkgRegistryCmd.AddCommand(vpkgRegistryListCmd)
	vpkgRegistryCmd.AddCommand(vpkgRegistryRemoveCmd)
	vpkgRegistryCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgRegistryAddCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgRegistryListCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgRegistryRemoveCmd.Flags().StringVar(&vpkgPath, "path", ".", "Project root path")
	vpkgCmd.AddCommand(vpkgRegistryCmd)
	RootCmd.AddCommand(vpkgCmd)
}

func printPlan(cmd *cobra.Command, plan []vpkg.AddPlanItem) {
	planLines := make([]string, 0, len(plan)+1)
	planLines = append(planLines, "Install plan:")
	for _, item := range plan {
		planLines = append(planLines, fmt.Sprintf("- %s -> %s (%s)", item.From, item.To, item.Action))
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), strings.Join(planLines, "\n"))
}

func confirmApplyPlan(cmd *cobra.Command) error {
	if !interactiveMode || assumeYes {
		return nil
	}
	if _, err := fmt.Fprint(cmd.ErrOrStderr(), "Apply this plan? [y/N]: "); err != nil {
		return err
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	if answer != "y" && answer != "yes" {
		return fmt.Errorf("aborted by user")
	}
	return nil
}

func renderDoctorIssueLines(report vpkg.DoctorReport) []string {
	lines := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		lines = append(lines, fmt.Sprintf("- [%s] %s (%s): %s", issue.Severity, issue.Package, issue.Check, issue.Message))
	}
	return lines
}

func paginateSearchItems(items []vpkg.SearchPackage, offset, limit int) ([]vpkg.SearchPackage, int, error) {
	if offset < 0 {
		return nil, 0, fmt.Errorf("--offset cannot be negative")
	}
	if limit < 0 {
		return nil, 0, fmt.Errorf("--limit cannot be negative")
	}
	total := len(items)
	if offset >= total {
		return []vpkg.SearchPackage{}, total, nil
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	return items[offset:end], total, nil
}
