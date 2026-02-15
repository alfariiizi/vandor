package cmd

// ckeletin:allow-custom-command

import (
	"fmt"

	"github.com/alfariiizi/vandor/internal/coregen"
	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add core artifacts (context/domain/valueobject/usecase/service)",
}

var (
	addContextPath            string
	addContextMinimal         bool
	addContextWithBuilder     bool
	addContextWithoutReadRepo bool
	addContextTidyMode        string
)

var addContextCmd = &cobra.Command{
	Use:   "context <name>",
	Short: "Add a bounded context scaffold",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(addContextPath)
		if err != nil {
			return err
		}
		path, err := coregen.AddContext(root, args[0], coregen.AddContextOptions{
			Minimal:         addContextMinimal,
			WithBuilder:     addContextWithBuilder,
			WithoutReadRepo: addContextWithoutReadRepo,
		})
		if err != nil {
			return err
		}
		contexts, err := syncContextAndCore(root, args[0])
		if err != nil {
			return err
		}
		tidyStatus, err := runTidy(root, addContextTidyMode)
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Added context %s", args[0]), map[string]any{
			"path":        path,
			"contexts":    contexts,
			"tidy_status": tidyStatus,
		})
	},
}

var (
	addDomainPath        string
	addDomainWithBuilder bool
)

var addDomainCmd = &cobra.Command{
	Use:   "domain <context> <name>",
	Short: "Add a domain entity skeleton into a context",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(addDomainPath)
		if err != nil {
			return err
		}
		if _, err := readModuleFromGoMod(root); err != nil {
			return err
		}
		path, err := coregen.AddDomain(root, args[0], args[1], addDomainWithBuilder)
		if err != nil {
			return err
		}
		contexts, err := syncContextAndCore(root, args[0])
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Added domain %s to context %s", args[1], args[0]), map[string]any{
			"path":     path,
			"contexts": contexts,
		})
	},
}

var addUsecasePath string

var addUsecaseCmd = &cobra.Command{
	Use:   "usecase <context> <name>",
	Short: "Add a usecase skeleton into a context",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(addUsecasePath)
		if err != nil {
			return err
		}
		if _, err := readModuleFromGoMod(root); err != nil {
			return err
		}
		path, err := coregen.AddUsecase(root, args[0], args[1])
		if err != nil {
			return err
		}
		contexts, err := syncContextAndCore(root, args[0])
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Added usecase %s to context %s", args[1], args[0]), map[string]any{
			"path":     path,
			"contexts": contexts,
		})
	},
}

var addServicePath string

var addServiceCmd = &cobra.Command{
	Use:   "service <context> <name>",
	Short: "Add an application service skeleton into a context",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(addServicePath)
		if err != nil {
			return err
		}
		if _, err := readModuleFromGoMod(root); err != nil {
			return err
		}
		path, err := coregen.AddService(root, args[0], args[1])
		if err != nil {
			return err
		}
		contexts, err := syncContextAndCore(root, args[0])
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Added service %s to context %s", args[1], args[0]), map[string]any{
			"path":     path,
			"contexts": contexts,
		})
	},
}

var (
	addValueObjectPath string
	addValueObjectKind string
	addValueObjectEnum string
)

var addValueObjectCmd = &cobra.Command{
	Use:   "valueobject <context> <name>",
	Short: "Add a valueobject into a context",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := resolvePath(addValueObjectPath)
		if err != nil {
			return err
		}
		if _, err := readModuleFromGoMod(root); err != nil {
			return err
		}
		path, err := coregen.AddValueObjectWithEnum(root, args[0], args[1], addValueObjectKind, addValueObjectEnum)
		if err != nil {
			return err
		}
		contexts, err := syncContextAndCore(root, args[0])
		if err != nil {
			return err
		}
		return emitResult(cmd, fmt.Sprintf("Added valueobject %s to context %s", args[1], args[0]), map[string]any{
			"path":     path,
			"contexts": contexts,
		})
	},
}

func syncContextAndCore(root, context string) ([]string, error) {
	if _, err := coregen.SyncContext(root, context); err != nil {
		return nil, err
	}
	return coregen.SyncCore(root)
}

func init() {
	addContextCmd.Flags().StringVar(&addContextPath, "path", ".", "Project root path")
	addContextCmd.Flags().BoolVar(&addContextMinimal, "minimal", false, "Generate minimal context scaffold")
	addContextCmd.Flags().BoolVar(&addContextWithBuilder, "with-builder", false, "Generate optional entity builder skeleton")
	addContextCmd.Flags().BoolVar(&addContextWithoutReadRepo, "without-read-repo", false, "Skip read repository scaffold")
	addContextCmd.Flags().StringVar(&addContextTidyMode, "tidy", "auto", "Run go mod tidy mode: auto|always|never")

	addDomainCmd.Flags().StringVar(&addDomainPath, "path", ".", "Project root path")
	addDomainCmd.Flags().BoolVar(&addDomainWithBuilder, "with-builder", false, "Generate builder skeleton for new entity")

	addUsecaseCmd.Flags().StringVar(&addUsecasePath, "path", ".", "Project root path")
	addServiceCmd.Flags().StringVar(&addServicePath, "path", ".", "Project root path")
	addValueObjectCmd.Flags().StringVar(&addValueObjectPath, "path", ".", "Project root path")
	addValueObjectCmd.Flags().StringVar(&addValueObjectKind, "kind", "string", "Valueobject underlying kind: string|int|float64|bool|time")
	addValueObjectCmd.Flags().StringVar(&addValueObjectEnum, "enum", "", "Comma-separated enum values (string kind only), e.g. WHATSAPP,EMAIL,SMS")

	addCmd.AddCommand(addContextCmd)
	addCmd.AddCommand(addDomainCmd)
	addCmd.AddCommand(addValueObjectCmd)
	addCmd.AddCommand(addUsecaseCmd)
	addCmd.AddCommand(addServiceCmd)
	RootCmd.AddCommand(addCmd)
}
