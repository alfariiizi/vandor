package coregen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitProject(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target, err := InitProject(dir, "acme-app", "github.com/acme/acme-app", false)
	if err != nil {
		t.Fatalf("InitProject() error = %v", err)
	}

	required := []string{
		filepath.Join(target, "README.md"),
		filepath.Join(target, "go.mod"),
		filepath.Join(target, "Taskfile.yml"),
		filepath.Join(target, ".vandor", "project.yaml"),
		filepath.Join(target, "internal", "core", "contracts", "command.go"),
		filepath.Join(target, "internal", "core", "contracts", "tx_contract.go"),
		filepath.Join(target, "internal", "core", "contracts", "event_contract.go"),
		filepath.Join(target, "internal", "core", "_gen", "contexts_gen.go"),
	}

	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("required file %s not created: %v", path, err)
		}
	}
}

func TestAddContext(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "internal", "core", "contexts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module github.com/acme/test\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := AddContext(projectRoot, "iam", AddContextOptions{WithBuilder: true})
	if err != nil {
		t.Fatalf("AddContext() error = %v", err)
	}

	required := []string{
		filepath.Join(projectRoot, "internal", "core", "contracts", "command.go"),
		filepath.Join(projectRoot, "internal", "core", "contracts", "tx_contract.go"),
		filepath.Join(projectRoot, "internal", "core", "contracts", "event_contract.go"),
		filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "domain", "entity", "iam.go"),
		filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "domain", "entity", "iam_builder.go"),
		filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "domain", "read_repository.go"),
		filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "application", "usecase", "create_iam.go"),
	}

	for _, path := range required {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("required file %s not created: %v", path, err)
		}
	}
}

func TestAddContextFailsFastWithoutGoMod(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "internal", "core", "contexts"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := AddContext(projectRoot, "iam", AddContextOptions{})
	if err == nil {
		t.Fatalf("expected AddContext to fail when go.mod/module is missing")
	}

	if _, statErr := os.Stat(filepath.Join(projectRoot, "internal", "core", "contexts", "iam")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no context files to be written on preflight failure, got err=%v", statErr)
	}
}

func TestAddValueObject(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "internal", "core", "contexts", "iam"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := AddValueObject(projectRoot, "iam", "provider_kind", "string")
	if err != nil {
		t.Fatalf("AddValueObject() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("valueobject file not created: %v", err)
	}
}

func TestAddValueObjectWithEnum(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "internal", "core", "contexts", "iam"), 0o755); err != nil {
		t.Fatal(err)
	}

	path, err := AddValueObjectWithEnum(projectRoot, "iam", "provider_kind", "string", "WHATSAPP,EMAIL,SMS")
	if err != nil {
		t.Fatalf("AddValueObjectWithEnum() error = %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading valueobject file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "ProviderKindWhatsapp ProviderKind = \"WHATSAPP\"") {
		t.Fatalf("expected enum constant for WHATSAPP, got:\n%s", text)
	}
	if !strings.Contains(text, "invalid ProviderKind") {
		t.Fatalf("expected enum validation message, got:\n%s", text)
	}
}

func TestAddValueObjectWithEnumInvalidKind(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "internal", "core", "contexts", "iam"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := AddValueObjectWithEnum(projectRoot, "iam", "provider_kind", "int", "1,2,3")
	if err == nil {
		t.Fatalf("expected AddValueObjectWithEnum to fail for non-string kind")
	}
}

func TestAddUsecaseFxFirstTemplate(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "internal", "core", "contexts", "iam"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module github.com/acme/test\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := AddUsecase(projectRoot, "iam", "create_user")
	if err != nil {
		t.Fatalf("AddUsecase() error = %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading usecase file: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "contracts.Command[CreateUserInput, CreateUserOutput]") {
		t.Fatalf("expected command generic contract in generated usecase:\n%s", text)
	}
	if !strings.Contains(text, "fx.In") {
		t.Fatalf("expected fx.In deps in generated usecase:\n%s", text)
	}
	if !strings.Contains(text, "TxManager      contracts.TxManager") {
		t.Fatalf("expected TxManager dependency in generated usecase:\n%s", text)
	}
	if !strings.Contains(text, "EventPublisher contracts.EventPublisher") {
		t.Fatalf("expected EventPublisher dependency in generated usecase:\n%s", text)
	}
	if !strings.Contains(text, "Execute(ctx context.Context, input CreateUserInput) (*CreateUserOutput, error)") {
		t.Fatalf("expected typed Execute signature in generated usecase:\n%s", text)
	}
}

func TestAddUsecaseRequiresModulePath(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectRoot, "internal", "core", "contexts", "iam"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := AddUsecase(projectRoot, "iam", "create_user")
	if err == nil {
		t.Fatalf("expected AddUsecase to fail when go.mod/module is missing")
	}
}
