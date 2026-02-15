package coregen

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

type AddContextOptions struct {
	Minimal         bool
	WithBuilder     bool
	WithoutReadRepo bool
}

func InitProject(baseDir, projectName, moduleName string, force bool) (string, error) {
	if projectName == "" {
		return "", errors.New("project name is required")
	}

	name, err := normalizeName(projectName)
	if err != nil {
		return "", fmt.Errorf("invalid project name: %w", err)
	}

	targetDir := filepath.Join(baseDir, name)
	if err := ensureCleanTarget(targetDir, force); err != nil {
		return "", err
	}

	if moduleName == "" {
		moduleName = fmt.Sprintf("github.com/yourorg/%s", name)
	}

	// #nosec G301 -- scaffolded project directories are intentionally user-readable.
	if err := os.MkdirAll(filepath.Join(targetDir, "internal", "core", "contexts"), 0o755); err != nil {
		return "", err
	}
	// #nosec G301 -- scaffolded project directories are intentionally user-readable.
	if err := os.MkdirAll(filepath.Join(targetDir, "internal", "core", "_gen"), 0o755); err != nil {
		return "", err
	}
	// #nosec G301 -- scaffolded project directories are intentionally user-readable.
	if err := os.MkdirAll(filepath.Join(targetDir, ".vandor"), 0o755); err != nil {
		return "", err
	}

	files := map[string]string{
		filepath.Join(targetDir, "README.md"): projectReadme(name),
		filepath.Join(targetDir, "go.mod"): strings.Join([]string{
			fmt.Sprintf("module %s", moduleName),
			"",
			"go 1.24.0",
			"",
			"require go.uber.org/fx v1.24.0",
			"",
		}, "\n"),
		filepath.Join(targetDir, ".gitignore"): strings.Join([]string{
			".env",
			".env.*",
			"bin/",
			"tmp/",
			"logs/",
			"",
		}, "\n"),
		filepath.Join(targetDir, ".vandor", "project.yaml"): strings.Join([]string{
			"name: " + name,
			"module: " + moduleName,
			"version: 0.4.0",
			"core_only: true",
			"",
		}, "\n"),
		filepath.Join(targetDir, "Taskfile.yml"): strings.Join([]string{
			"version: \"3\"",
			"",
			"tasks:",
			"  dev:app:",
			"    cmds:",
			"      - echo \"TODO: wire vpkg transport/infrastructure then run app dev mode\"",
			"  run:app:",
			"    cmds:",
			"      - echo \"TODO: wire vpkg transport/infrastructure then run app production mode\"",
			"  sync:core:",
			"    cmds:",
			"      - vandor sync core",
			"",
		}, "\n"),
	}

	for path, content := range files {
		if err := writeFileIfMissing(path, content); err != nil {
			return "", err
		}
	}
	if err := ensureCoreContracts(targetDir); err != nil {
		return "", err
	}

	if _, err := SyncCore(targetDir); err != nil {
		return "", err
	}

	return targetDir, nil
}

func AddContext(projectRoot, rawContext string, opts AddContextOptions) (string, error) {
	contextName, err := normalizeName(rawContext)
	if err != nil {
		return "", fmt.Errorf("invalid context name: %w", err)
	}

	contextDir := filepath.Join(projectRoot, "internal", "core", "contexts", contextName)
	if exists(contextDir) {
		return "", fmt.Errorf("context already exists: %s", contextName)
	}

	entityName := toPascal(contextName)
	ctxPkg := toSnake(contextName)
	modulePath, err := requireModulePath(projectRoot)
	if err != nil {
		return "", err
	}
	if err := ensureCoreContracts(projectRoot); err != nil {
		return "", err
	}

	dirs := []string{
		filepath.Join(contextDir, "domain", "entity"),
		filepath.Join(contextDir, "domain", "valueobject"),
		filepath.Join(contextDir, "application", "usecase"),
	}
	for _, dir := range dirs {
		// #nosec G301 -- generated project source directories are intentionally user-readable.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
	}

	files := map[string]string{
		filepath.Join(contextDir, "README.md"):                                                                 contextReadme(contextName),
		filepath.Join(contextDir, "module.go"):                                                                 moduleTemplate(ctxPkg),
		filepath.Join(contextDir, "domain", "entity", fmt.Sprintf("%s.go", toSnake(contextName))):              entityTemplate(entityName),
		filepath.Join(contextDir, "domain", "valueobject", fmt.Sprintf("%s_id.go", toSnake(contextName))):      valueObjectTemplate(entityName),
		filepath.Join(contextDir, "domain", "errors.go"):                                                       domainErrorsTemplate(),
		filepath.Join(contextDir, "domain", "repository.go"):                                                   repositoryTemplate(entityName),
		filepath.Join(contextDir, "application", "usecase", fmt.Sprintf("create_%s.go", toSnake(contextName))): createUsecaseTemplate(entityName, modulePath),
	}

	if !opts.WithoutReadRepo {
		files[filepath.Join(contextDir, "domain", "read_repository.go")] = readRepositoryTemplate(entityName)
	}
	if opts.WithBuilder {
		files[filepath.Join(contextDir, "domain", "entity", fmt.Sprintf("%s_builder.go", toSnake(contextName)))] = builderTemplate(entityName)
	}

	if opts.Minimal {
		delete(files, filepath.Join(contextDir, "domain", "read_repository.go"))
		delete(files, filepath.Join(contextDir, "domain", "valueobject", fmt.Sprintf("%s_id.go", toSnake(contextName))))
	}

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	for _, path := range paths {
		if err := writeFileIfMissing(path, files[path]); err != nil {
			return "", err
		}
	}

	return contextDir, nil
}

func AddDomain(projectRoot, rawContext, rawDomain string, withBuilder bool) (string, error) {
	contextName, err := normalizeName(rawContext)
	if err != nil {
		return "", fmt.Errorf("invalid context name: %w", err)
	}
	domainName, err := normalizeName(rawDomain)
	if err != nil {
		return "", fmt.Errorf("invalid domain name: %w", err)
	}

	contextDir := filepath.Join(projectRoot, "internal", "core", "contexts", contextName)
	if !exists(contextDir) {
		return "", fmt.Errorf("context not found: %s", contextName)
	}

	entityDir := filepath.Join(contextDir, "domain", "entity")
	// #nosec G301 -- generated project source directories are intentionally user-readable.
	if err := os.MkdirAll(entityDir, 0o755); err != nil {
		return "", err
	}

	entityName := toPascal(domainName)
	entityPath := filepath.Join(entityDir, fmt.Sprintf("%s.go", toSnake(domainName)))
	if err := writeFileIfMissing(entityPath, entityTemplate(entityName)); err != nil {
		return "", err
	}

	if withBuilder {
		builderPath := filepath.Join(entityDir, fmt.Sprintf("%s_builder.go", toSnake(domainName)))
		if err := writeFileIfMissing(builderPath, builderTemplate(entityName)); err != nil {
			return "", err
		}
	}

	return entityPath, nil
}

func AddValueObject(projectRoot, rawContext, rawValueObject, kind string) (string, error) {
	contextName, err := normalizeName(rawContext)
	if err != nil {
		return "", fmt.Errorf("invalid context name: %w", err)
	}
	valueObjectName, err := normalizeName(rawValueObject)
	if err != nil {
		return "", fmt.Errorf("invalid valueobject name: %w", err)
	}

	contextDir := filepath.Join(projectRoot, "internal", "core", "contexts", contextName)
	if !exists(contextDir) {
		return "", fmt.Errorf("context not found: %s", contextName)
	}

	voDir := filepath.Join(contextDir, "domain", "valueobject")
	// #nosec G301 -- generated project source directories are intentionally user-readable.
	if err := os.MkdirAll(voDir, 0o755); err != nil {
		return "", err
	}

	voTypeName := toPascal(valueObjectName)
	path := filepath.Join(voDir, fmt.Sprintf("%s.go", toSnake(valueObjectName)))
	content, err := valueObjectCustomTemplate(voTypeName, kind, nil)
	if err != nil {
		return "", err
	}
	if err := writeFileIfMissing(path, content); err != nil {
		return "", err
	}
	return path, nil
}

func AddValueObjectWithEnum(projectRoot, rawContext, rawValueObject, kind, enumRaw string) (string, error) {
	enumValues, err := parseEnumValues(enumRaw)
	if err != nil {
		return "", err
	}
	contextName, err := normalizeName(rawContext)
	if err != nil {
		return "", fmt.Errorf("invalid context name: %w", err)
	}
	valueObjectName, err := normalizeName(rawValueObject)
	if err != nil {
		return "", fmt.Errorf("invalid valueobject name: %w", err)
	}

	contextDir := filepath.Join(projectRoot, "internal", "core", "contexts", contextName)
	if !exists(contextDir) {
		return "", fmt.Errorf("context not found: %s", contextName)
	}

	voDir := filepath.Join(contextDir, "domain", "valueobject")
	if err := os.MkdirAll(voDir, 0o755); err != nil {
		return "", err
	}

	voTypeName := toPascal(valueObjectName)
	path := filepath.Join(voDir, fmt.Sprintf("%s.go", toSnake(valueObjectName)))
	content, err := valueObjectCustomTemplate(voTypeName, kind, enumValues)
	if err != nil {
		return "", err
	}
	if err := writeFileIfMissing(path, content); err != nil {
		return "", err
	}
	return path, nil
}

func AddUsecase(projectRoot, rawContext, rawUsecase string) (string, error) {
	contextName, err := normalizeName(rawContext)
	if err != nil {
		return "", fmt.Errorf("invalid context name: %w", err)
	}
	usecaseName, err := normalizeName(rawUsecase)
	if err != nil {
		return "", fmt.Errorf("invalid usecase name: %w", err)
	}

	contextDir := filepath.Join(projectRoot, "internal", "core", "contexts", contextName)
	if !exists(contextDir) {
		return "", fmt.Errorf("context not found: %s", contextName)
	}

	dir := filepath.Join(contextDir, "application", "usecase")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	modulePath, err := requireModulePath(projectRoot)
	if err != nil {
		return "", err
	}
	commandImport := fmt.Sprintf("%q", modulePath+"/internal/core/contracts")

	typeName := toPascal(usecaseName)
	useCaseType := typeName + "UseCase"
	depsType := useCaseType + "Deps"
	usecaseInterface := typeName
	inputType := typeName + "Input"
	outputType := typeName + "Output"
	path := filepath.Join(dir, fmt.Sprintf("%s.go", toSnake(usecaseName)))
	content := strings.Join([]string{
		"package usecase",
		"",
		"import (",
		"\t\"context\"",
		"\t" + commandImport,
		"",
		"\t\"go.uber.org/fx\"",
		")",
		"",
		fmt.Sprintf("type %s struct{}", inputType),
		"",
		fmt.Sprintf("type %s struct{}", outputType),
		"",
		fmt.Sprintf("type %s interface {", usecaseInterface),
		fmt.Sprintf("\tcontracts.Command[%s, %s]", inputType, outputType),
		"}",
		"",
		fmt.Sprintf("type %s struct {", useCaseType),
		"\ttxManager      contracts.TxManager",
		"\teventPublisher contracts.EventPublisher",
		"}",
		"",
		fmt.Sprintf("type %s struct {", depsType),
		"\tfx.In",
		"",
		"\tTxManager      contracts.TxManager",
		"\tEventPublisher contracts.EventPublisher",
		"\t// TODO: add dependencies from other modules here.",
		"}",
		"",
		fmt.Sprintf("func New%s(deps %s) *%s {", useCaseType, depsType, useCaseType),
		fmt.Sprintf("\treturn &%s{", useCaseType),
		"\t\ttxManager:      deps.TxManager,",
		"\t\teventPublisher: deps.EventPublisher,",
		"\t}",
		"}",
		"",
		fmt.Sprintf("func (uc *%s) Execute(ctx context.Context, input %s) (*%s, error) {", useCaseType, inputType, outputType),
		"\t_ = ctx",
		"\t_ = input",
		"\t// TODO: implement usecase behavior and typed input/output.",
		fmt.Sprintf("\treturn &%s{}, nil", outputType),
		"}",
		"",
	}, "\n")

	if err := writeFileIfMissing(path, content); err != nil {
		return "", err
	}
	return path, nil
}

func AddService(projectRoot, rawContext, rawService string) (string, error) {
	contextName, err := normalizeName(rawContext)
	if err != nil {
		return "", fmt.Errorf("invalid context name: %w", err)
	}
	serviceName, err := normalizeName(rawService)
	if err != nil {
		return "", fmt.Errorf("invalid service name: %w", err)
	}

	contextDir := filepath.Join(projectRoot, "internal", "core", "contexts", contextName)
	if !exists(contextDir) {
		return "", fmt.Errorf("context not found: %s", contextName)
	}

	dir := filepath.Join(contextDir, "application", "service")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	typeName := toPascal(serviceName)
	serviceTypeName := typeName + "Service"
	depsTypeName := serviceTypeName + "Deps"
	path := filepath.Join(dir, fmt.Sprintf("%s.go", toSnake(serviceName)))
	content := strings.Join([]string{
		"package service",
		"",
		`import "go.uber.org/fx"`,
		"",
		fmt.Sprintf("type %s struct {}", serviceTypeName),
		"",
		fmt.Sprintf("type %s struct {", depsTypeName),
		"\tfx.In",
		"\t// TODO: add dependencies from other modules here.",
		"}",
		"",
		fmt.Sprintf("func New%s(deps %s) *%s {", serviceTypeName, depsTypeName, serviceTypeName),
		"\t_ = deps",
		fmt.Sprintf("\treturn &%s{}", serviceTypeName),
		"}",
		"",
		"// TODO: add domain-driven methods for this service.",
		"",
	}, "\n")

	if err := writeFileIfMissing(path, content); err != nil {
		return "", err
	}
	return path, nil
}

func ensureCleanTarget(path string, force bool) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("target exists and is not directory: %s", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("target directory is not empty: %s (use --force)", path)
	}
	return nil
}

func normalizeName(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("name cannot be empty")
	}
	s = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, " ", "_"), "-", "_"))
	if !namePattern.MatchString(s) {
		return "", fmt.Errorf("name %q must match %s", raw, namePattern.String())
	}
	return s, nil
}

func toSnake(name string) string {
	s := strings.TrimSpace(name)
	s = strings.ReplaceAll(s, "-", "_")
	s = strings.ReplaceAll(s, " ", "_")
	return strings.ToLower(s)
}

func toPascal(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i := range parts {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + strings.ToLower(parts[i][1:])
	}
	return strings.Join(parts, "")
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeFileIfMissing(path, content string) error {
	if exists(path) {
		return fmt.Errorf("file already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// #nosec G306 -- generated source and config templates should be readable by project collaborators.
	return os.WriteFile(path, []byte(content), 0o644)
}

func ensureCoreContracts(projectRoot string) error {
	for path, content := range coreContractsFiles(projectRoot) {
		if exists(path) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		// #nosec G306 -- generated core contract files should be readable by project collaborators.
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func coreContractsFiles(projectRoot string) map[string]string {
	return map[string]string{
		filepath.Join(projectRoot, "internal", "core", "contracts", "command.go"): strings.Join([]string{
			"package contracts",
			"",
			`import "context"`,
			"",
			"type Command[I any, O any] interface {",
			"\tExecute(ctx context.Context, input I) (*O, error)",
			"}",
			"",
		}, "\n"),
		filepath.Join(projectRoot, "internal", "core", "contracts", "tx_contract.go"): strings.Join([]string{
			"package contracts",
			"",
			`import "context"`,
			"",
			"type TxManager interface {",
			"\tWithinTx(ctx context.Context, fn func(ctx context.Context) error) error",
			"}",
			"",
		}, "\n"),
		filepath.Join(projectRoot, "internal", "core", "contracts", "event_contract.go"): strings.Join([]string{
			"package contracts",
			"",
			"import (",
			"\t\"context\"",
			"\t\"time\"",
			")",
			"",
			"// DomainEvent is transport-agnostic and can be adapted to Watermill message.Message.",
			"type DomainEvent interface {",
			"\tEventID() string",
			"\tEventName() string",
			"\tOccurredAt() time.Time",
			"\tPayload() []byte",
			"\tMetadata() map[string]string",
			"}",
			"",
			"type EventPublisher interface {",
			"\tPublish(ctx context.Context, events ...DomainEvent) error",
			"}",
			"",
		}, "\n"),
	}
}

func readModulePath(projectRoot string) string {
	// #nosec G304 -- read path is constrained to project root + go.mod.
	// nosemgrep: go-path-traversal -- projectRoot is vandor-managed workspace root.
	data, err := os.ReadFile(filepath.Join(projectRoot, "go.mod"))
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func requireModulePath(projectRoot string) (string, error) {
	modulePath := readModulePath(projectRoot)
	if modulePath == "" {
		return "", fmt.Errorf("go.mod module path not found in %s", projectRoot)
	}
	return modulePath, nil
}

func projectReadme(name string) string {
	return strings.Join([]string{
		fmt.Sprintf("# %s", name),
		"",
		"Generated by `vandor new` in core-only mode.",
		"",
		"## Next",
		"",
		"1. Add your first context with `vandor add context <name>`.",
		"2. Sync generated registries with `vandor sync core`.",
		"3. Add transport/infrastructure through `vandor vpkg add ...`.",
		"",
	}, "\n")
}

func contextReadme(name string) string {
	return strings.Join([]string{
		fmt.Sprintf("# Context %s", name),
		"",
		"Generated scaffold for this bounded context.",
		"",
		"## Rules",
		"",
		"1. Keep domain storage-agnostic.",
		"2. Keep usecase access through `Execute(ctx, input)`.",
		"3. Implement write/read repositories in vpkg-provided infrastructure layer.",
		"",
	}, "\n")
}

func moduleTemplate(pkg string) string {
	return strings.Join([]string{
		"package " + pkg,
		"",
		`import "go.uber.org/fx"`,
		"",
		"// Module wires this context into runtime DI graph.",
		"// TODO: add fx.Provide/fx.Invoke for real usecases/services.",
		"var Module = fx.Options()",
		"",
	}, "\n")
}

func entityTemplate(entityName string) string {
	return strings.Join([]string{
		"package entity",
		"",
		`import (`,
		"\t\"errors\"",
		"\t\"time\"",
		`)`,
		"",
		"type New" + entityName + "Params struct {",
		"\tID string",
		"}",
		"",
		"type " + entityName + " struct {",
		"\tid        string",
		"\tcreatedAt time.Time",
		"\tupdatedAt time.Time",
		"}",
		"",
		"func New" + entityName + "(p New" + entityName + "Params) (*" + entityName + ", error) {",
		"\tif p.ID == \"\" {",
		"\t\treturn nil, errors.New(\"invalid entity id\")",
		"\t}",
		"\tnow := time.Now().UTC()",
		"\treturn &" + entityName + "{id: p.ID, createdAt: now, updatedAt: now}, nil",
		"}",
		"",
		"func (e *" + entityName + ") ID() string { return e.id }",
		"",
		"func (e *" + entityName + ") Touch(now time.Time) {",
		"\te.updatedAt = now.UTC()",
		"}",
		"",
	}, "\n")
}

func builderTemplate(entityName string) string {
	return strings.Join([]string{
		"package entity",
		"",
		"type " + entityName + "Builder struct {",
		"\tparams New" + entityName + "Params",
		"}",
		"",
		"func New" + entityName + "Builder() *" + entityName + "Builder {",
		"\treturn &" + entityName + "Builder{}",
		"}",
		"",
		"func (b *" + entityName + "Builder) WithID(id string) *" + entityName + "Builder {",
		"\tb.params.ID = id",
		"\treturn b",
		"}",
		"",
		"func (b *" + entityName + "Builder) Build() (*" + entityName + ", error) {",
		"\treturn New" + entityName + "(b.params)",
		"}",
		"",
	}, "\n")
}

func valueObjectTemplate(entityName string) string {
	return strings.Join([]string{
		"package valueobject",
		"",
		`import "fmt"`,
		"",
		"type " + entityName + "ID string",
		"",
		"func New" + entityName + "ID(raw string) (" + entityName + "ID, error) {",
		"\tif raw == \"\" {",
		"\t\treturn \"\", fmt.Errorf(\"" + strings.ToLower(entityName) + " id cannot be empty\")",
		"\t}",
		"\treturn " + entityName + "ID(raw), nil",
		"}",
		"",
		"func (id " + entityName + "ID) String() string { return string(id) }",
		"",
	}, "\n")
}

func valueObjectCustomTemplate(valueObjectName, kind string, enumValues []string) (string, error) {
	baseKind, imports, validateCond, validateMsg, err := valueObjectKindSpec(kind)
	if err != nil {
		return "", err
	}
	if len(enumValues) > 0 && baseKind != "string" {
		return "", fmt.Errorf("--enum is only supported for kind=string")
	}

	importSet := map[string]struct{}{}
	for _, imp := range imports {
		importSet[imp] = struct{}{}
	}
	if len(enumValues) > 0 {
		importSet["fmt"] = struct{}{}
		importSet["strings"] = struct{}{}
	}

	imports = imports[:0]
	for imp := range importSet {
		imports = append(imports, imp)
	}
	sort.Strings(imports)

	lines := []string{
		"package valueobject",
		"",
	}
	if len(imports) > 0 {
		lines = append(lines, "import (")
		for _, imp := range imports {
			lines = append(lines, "\t"+fmt.Sprintf("%q", imp))
		}
		lines = append(lines, ")")
		lines = append(lines, "")
	}
	lines = append(lines,
		fmt.Sprintf("type %s %s", valueObjectName, baseKind),
		"",
	)

	if len(enumValues) > 0 {
		lines = append(lines, "const (")
		for _, v := range enumValues {
			lines = append(lines, fmt.Sprintf("\t%s%s %s = %q", valueObjectName, enumConstSuffix(v), valueObjectName, v))
		}
		lines = append(lines, ")")
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("var valid%s = map[%s]struct{}{", valueObjectName, valueObjectName))
		for _, v := range enumValues {
			lines = append(lines, fmt.Sprintf("\t%s%s: {},", valueObjectName, enumConstSuffix(v)))
		}
		lines = append(lines, "}")
		lines = append(lines, "")
	}

	lines = append(lines,
		fmt.Sprintf("func New%s(value %s) (%s, error) {", valueObjectName, baseKind, valueObjectName),
	)
	if validateCond != "" {
		lines = append(lines, fmt.Sprintf("\tif %s {", validateCond))
		lines = append(lines, fmt.Sprintf("\t\tvar zero %s", valueObjectName))
		lines = append(lines, fmt.Sprintf("\t\treturn zero, fmt.Errorf(%q)", validateMsg))
		lines = append(lines, "\t}")
	}
	if len(enumValues) > 0 {
		lines = append(lines, fmt.Sprintf("\tv := %s(strings.TrimSpace(value))", valueObjectName))
		lines = append(lines, fmt.Sprintf("\tif _, ok := valid%s[v]; !ok {", valueObjectName))
		lines = append(lines, fmt.Sprintf("\t\tvar zero %s", valueObjectName))
		lines = append(lines, fmt.Sprintf("\t\treturn zero, fmt.Errorf(\"invalid %s: %%s\", value)", valueObjectName))
		lines = append(lines, "\t}")
		lines = append(lines, "\treturn v, nil")
		lines = append(lines, "}")
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("func (v %s) Value() %s {", valueObjectName, baseKind))
		lines = append(lines, fmt.Sprintf("\treturn %s(v)", baseKind))
		lines = append(lines, "}")
		lines = append(lines, "")
		return strings.Join(lines, "\n"), nil
	}
	lines = append(lines,
		fmt.Sprintf("\treturn %s(value), nil", valueObjectName),
		"}",
		"",
		fmt.Sprintf("func (v %s) Value() %s {", valueObjectName, baseKind),
		fmt.Sprintf("\treturn %s(v)", baseKind),
		"}",
		"",
	)
	return strings.Join(lines, "\n"), nil
}

func parseEnumValues(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			return nil, fmt.Errorf("invalid --enum value: empty item in %q", raw)
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		values = append(values, v)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("invalid --enum value: no valid entries")
	}
	return values, nil
}

func enumConstSuffix(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	s := strings.TrimSpace(b.String())
	if s == "" {
		return "Unknown"
	}
	return toPascal(s)
}

func valueObjectKindSpec(kind string) (baseKind string, imports []string, validateCondition string, validateMessage string, err error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "string":
		return "string", []string{"fmt", "strings"}, `strings.TrimSpace(value) == ""`, "value cannot be empty", nil
	case "int":
		return "int", nil, "", "", nil
	case "float", "float64":
		return "float64", nil, "", "", nil
	case "bool", "boolean":
		return "bool", nil, "", "", nil
	case "time", "datetime":
		return "time.Time", []string{"fmt", "time"}, `value.IsZero()`, "value cannot be zero", nil
	default:
		return "", nil, "", "", fmt.Errorf("invalid kind %q: allowed kinds are string|int|float64|bool|time", kind)
	}
}

func domainErrorsTemplate() string {
	return strings.Join([]string{
		"package domain",
		"",
		`import "errors"`,
		"",
		"var (",
		"\tErrNotFound = errors.New(\"not found\")",
		"\tErrConflict = errors.New(\"conflict\")",
		")",
		"",
	}, "\n")
}

func repositoryTemplate(entityName string) string {
	return strings.Join([]string{
		"package domain",
		"",
		`import "context"`,
		"",
		"type Repository interface {",
		"\tCreate(ctx context.Context, agg any) error",
		"\tSave(ctx context.Context, agg any) error",
		"\tFindByID(ctx context.Context, id string) (any, error)",
		"\t// TODO: replace any with *entity." + entityName + " after adapting your package import strategy.",
		"}",
		"",
	}, "\n")
}

func readRepositoryTemplate(entityName string) string {
	return strings.Join([]string{
		"package domain",
		"",
		`import "context"`,
		"",
		"type Query struct {",
		"\tLimit  int",
		"\tOffset int",
		"\tSortBy string",
		"\tOrder  string",
		"}",
		"",
		"type ReadRepository interface {",
		"\tFindByID(ctx context.Context, id string) (any, error)",
		"\tList(ctx context.Context, q Query) ([]any, error)",
		"\t// TODO: replace any with read-model dto for " + entityName + ".",
		"}",
		"",
	}, "\n")
}

func createUsecaseTemplate(entityName, modulePath string) string {
	commandImport := "\"internal/core/contracts\""
	if modulePath != "" {
		commandImport = fmt.Sprintf("%q", modulePath+"/internal/core/contracts")
	}

	return strings.Join([]string{
		"package usecase",
		"",
		"import (",
		"\t\"context\"",
		"\t" + commandImport,
		"",
		"\t\"go.uber.org/fx\"",
		")",
		"",
		"type Create" + entityName + "Input struct {",
		"\tID string `validate:\"required\"`",
		"}",
		"",
		"type Create" + entityName + "Output struct {",
		"\tID string",
		"}",
		"",
		"type Create" + entityName + " interface {",
		"\tcontracts.Command[Create" + entityName + "Input, Create" + entityName + "Output]",
		"}",
		"",
		"type Create" + entityName + "UseCaseDeps struct {",
		"\tfx.In",
		"",
		"\tTxManager      contracts.TxManager",
		"\tEventPublisher contracts.EventPublisher",
		"\t// TODO: add dependencies from other modules here (repo, client, etc).",
		"}",
		"",
		"type Create" + entityName + "UseCase struct {",
		"\ttxManager      contracts.TxManager",
		"\teventPublisher contracts.EventPublisher",
		"\t// TODO: inject write repository via constructor.",
		"}",
		"",
		"func NewCreate" + entityName + "UseCase(deps Create" + entityName + "UseCaseDeps) *Create" + entityName + "UseCase {",
		"\treturn &Create" + entityName + "UseCase{",
		"\t\ttxManager:      deps.TxManager,",
		"\t\teventPublisher: deps.EventPublisher,",
		"\t}",
		"}",
		"",
		"func (uc *Create" + entityName + "UseCase) Execute(ctx context.Context, input Create" + entityName + "Input) (*Create" + entityName + "Output, error) {",
		"\t_ = ctx",
		"\t_ = input",
		"\t// TODO: map input DTO, enforce domain invariants, then persist aggregate.",
		"\t// TODO: publish domain events through uc.eventPublisher when needed.",
		"\treturn &Create" + entityName + "Output{ID: input.ID}, nil",
		"}",
		"",
	}, "\n")
}
