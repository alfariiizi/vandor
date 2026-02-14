package coregen

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncContextGeneratesModuleGen(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if _, err := InitProject(projectRoot, "acme-sync", "github.com/acme/acme-sync", false); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(projectRoot, "acme_sync")
	if _, err := AddContext(root, "iam", AddContextOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := AddUsecase(root, "iam", "create_user"); err != nil {
		t.Fatal(err)
	}
	if _, err := AddService(root, "iam", "token_issuer"); err != nil {
		t.Fatal(err)
	}

	if _, err := SyncContext(root, "iam"); err != nil {
		t.Fatalf("SyncContext() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(root, "internal", "core", "contexts", "iam", "module_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "usecase.NewCreateIamUseCase") {
		t.Fatalf("module_gen.go missing add-generated usecase constructor:\n%s", text)
	}
	if !strings.Contains(text, "usecase.NewCreateUserUseCase") {
		t.Fatalf("module_gen.go missing add-generated usecase constructor:\n%s", text)
	}
	if !strings.Contains(text, "service.NewTokenIssuerService") {
		t.Fatalf("module_gen.go missing add-generated service constructor:\n%s", text)
	}
}

func TestSyncContextAllowsHelperFilesWithoutConstructor(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	contextDir := filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "application", "usecase")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module github.com/acme/test\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextDir, "bad.go"), []byte("package usecase\n\nfunc Bad() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := SyncContext(projectRoot, "iam"); err != nil {
		t.Fatalf("expected helper-only files to be ignored by strict sync, got error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "module_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "var generatedModule = fx.Options()") {
		t.Fatalf("expected empty generated module for helper-only files:\n%s", string(data))
	}
}

func TestSyncContextStrictRequiresFxInDeps(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	contextDir := filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "application", "service")
	if err := os.MkdirAll(contextDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module github.com/acme/test\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	invalid := `package service

type TokenService struct{}

func NewTokenService() *TokenService { return &TokenService{} }
`
	if err := os.WriteFile(filepath.Join(contextDir, "token.go"), []byte(invalid), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := SyncContext(projectRoot, "iam"); err == nil {
		t.Fatalf("expected strict sync error when fx.In deps pattern is missing")
	}
}

func TestSyncCoreDeterministic(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module github.com/acme/test\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	contextsDir := filepath.Join(projectRoot, "internal", "core", "contexts")
	for _, name := range []string{"zeta", "alpha"} {
		if err := os.MkdirAll(filepath.Join(contextsDir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(contextsDir, name, "module.go"), []byte("package "+name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := SyncCore(projectRoot)
	if err != nil {
		t.Fatalf("SyncCore() error = %v", err)
	}
	if len(got) != 2 || got[0] != "alpha" || got[1] != "zeta" {
		t.Fatalf("unexpected contexts order: %#v", got)
	}

	contextsContent, err := os.ReadFile(filepath.Join(projectRoot, "internal", "core", "_gen", "contexts_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contextsContent), "\"alpha\"") || !strings.Contains(string(contextsContent), "\"zeta\"") {
		t.Fatalf("unexpected contexts_gen.go:\n%s", string(contextsContent))
	}

	modulesContent, err := os.ReadFile(filepath.Join(projectRoot, "internal", "core", "_gen", "modules_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(modulesContent), "alpha.Module") || !strings.Contains(string(modulesContent), "zeta.Module") {
		t.Fatalf("unexpected modules_gen.go:\n%s", string(modulesContent))
	}
}

func TestSyncCoreIgnoresDirectoriesWithoutModule(t *testing.T) {
	t.Parallel()

	projectRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectRoot, "go.mod"), []byte("module github.com/acme/test\n\ngo 1.24.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	contextsDir := filepath.Join(projectRoot, "internal", "core", "contexts")
	if err := os.MkdirAll(filepath.Join(contextsDir, "iam"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(contextsDir, "_notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contextsDir, "iam", "module.go"), []byte("package iam\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	contexts, err := SyncCore(projectRoot)
	if err != nil {
		t.Fatalf("SyncCore() error = %v", err)
	}
	if len(contexts) != 1 || contexts[0] != "iam" {
		t.Fatalf("expected only iam context, got: %#v", contexts)
	}

	data, err := os.ReadFile(filepath.Join(projectRoot, "internal", "core", "_gen", "contexts_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "\"iam\"") {
		t.Fatalf("expected contexts_gen.go to contain iam:\n%s", text)
	}
	if strings.Contains(text, "\"_notes\"") {
		t.Fatalf("did not expect non-context folder in contexts_gen.go:\n%s", text)
	}
}
