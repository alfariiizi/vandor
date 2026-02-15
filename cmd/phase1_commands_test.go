package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetGlobalFlagsForTest() {
	interactiveMode = false
	interactiveAlias = false
	noInputMode = false
	assumeYes = false
	outputMode = "text"

	newBasePath = "."
	newModule = ""
	newForce = false
	newTidyMode = "auto"

	vpkgPath = "."
	vpkgAddPlanOnly = false
	vpkgRemoveForce = false
	vpkgSearchTier = ""
	vpkgSearchRegistry = ""
	vpkgSearchLimit = 10
	vpkgSearchOffset = 0
	vpkgDoctorFix = false
}

func runRootForTest(args ...string) (string, error) {
	resetGlobalFlagsForTest()
	var out bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetErr(&out)
	RootCmd.SetIn(strings.NewReader(""))
	RootCmd.SetArgs(args)
	err := RootCmd.Execute()
	return out.String(), err
}

func runRootForTestWithInput(input string, args ...string) (string, error) {
	resetGlobalFlagsForTest()
	var out bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetErr(&out)
	RootCmd.SetIn(strings.NewReader(input))
	RootCmd.SetArgs(args)
	err := RootCmd.Execute()
	return out.String(), err
}

func TestNewAddSyncFlow(t *testing.T) {
	tmp := t.TempDir()

	out, err := runRootForTest("new", "acme_app", "--path", tmp, "--module", "github.com/acme/acme_app", "--tidy", "never")
	if err != nil {
		t.Fatalf("new command failed: %v\nout=%s", err, out)
	}

	projectRoot := filepath.Join(tmp, "acme_app")
	if _, err := os.Stat(filepath.Join(projectRoot, ".vandor", "project.yaml")); err != nil {
		t.Fatalf("expected project metadata file: %v", err)
	}

	out, err = runRootForTest("add", "context", "iam", "--path", projectRoot, "--with-builder")
	if err != nil {
		t.Fatalf("add context failed: %v\nout=%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "domain", "entity", "iam_builder.go")); err != nil {
		t.Fatalf("expected builder file: %v", err)
	}

	out, err = runRootForTest("add", "valueobject", "iam", "provider_kind", "--path", projectRoot, "--kind", "string", "--enum", "WHATSAPP,EMAIL,SMS")
	if err != nil {
		t.Fatalf("add valueobject failed: %v\nout=%s", err, out)
	}
	voPath := filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "domain", "valueobject", "provider_kind.go")
	if _, err := os.Stat(voPath); err != nil {
		t.Fatalf("expected valueobject file: %v", err)
	}
	voContent, err := os.ReadFile(voPath)
	if err != nil {
		t.Fatalf("failed reading valueobject file: %v", err)
	}
	if !strings.Contains(string(voContent), "ProviderKindWhatsapp ProviderKind = \"WHATSAPP\"") {
		t.Fatalf("expected enum constants in generated valueobject:\n%s", string(voContent))
	}

	out, err = runRootForTest("add", "usecase", "iam", "create_user", "--path", projectRoot)
	if err != nil {
		t.Fatalf("add usecase failed: %v\nout=%s", err, out)
	}
	moduleGenPath := filepath.Join(projectRoot, "internal", "core", "contexts", "iam", "module_gen.go")
	moduleGenContent, err := os.ReadFile(moduleGenPath)
	if err != nil {
		t.Fatalf("failed reading module_gen.go: %v", err)
	}
	if !strings.Contains(string(moduleGenContent), "usecase.NewCreateUserUseCase") {
		t.Fatalf("expected add usecase to auto-sync context module, got:\n%s", string(moduleGenContent))
	}

	out, err = runRootForTest("sync", "core", "--path", projectRoot)
	if err != nil {
		t.Fatalf("sync core failed: %v\nout=%s", err, out)
	}

	content, err := os.ReadFile(filepath.Join(projectRoot, "internal", "core", "_gen", "contexts_gen.go"))
	if err != nil {
		t.Fatalf("failed reading contexts_gen.go: %v", err)
	}
	if !strings.Contains(string(content), "\"iam\"") {
		t.Fatalf("expected generated contexts list to contain iam:\n%s", string(content))
	}
}

func TestInteractiveFlagsConflict(t *testing.T) {
	tmp := t.TempDir()
	_, err := runRootForTest("sync", "core", "--path", tmp, "--interactive", "--no-input")
	if err == nil {
		t.Fatalf("expected error when --interactive and --no-input are used together")
	}
}

func TestInteractiveShortHandIT(t *testing.T) {
	tmp := t.TempDir()
	out, err := runRootForTest("-it", "new", "acme_it", "--path", tmp, "--tidy", "never")
	if err != nil {
		t.Fatalf("expected -it shorthand to work, got error: %v\nout=%s", err, out)
	}
}

func TestNewInvalidTidyMode(t *testing.T) {
	tmp := t.TempDir()
	_, err := runRootForTest("new", "acme_app", "--path", tmp, "--tidy", "random")
	if err == nil {
		t.Fatalf("expected invalid tidy mode to fail")
	}
}

func TestNewJsonOutputUsesEffectiveModule(t *testing.T) {
	tmp := t.TempDir()
	out, err := runRootForTest("new", "acme_auto", "--path", tmp, "--tidy", "never", "--output", "json")
	if err != nil {
		t.Fatalf("new command failed: %v\nout=%s", err, out)
	}

	var resp map[string]any
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("failed parsing json output: %v\nout=%s", err, out)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("invalid json payload data: %#v", resp["data"])
	}
	module, _ := data["module"].(string)
	if module != "github.com/yourorg/acme_auto" {
		t.Fatalf("unexpected module in json output: %q", module)
	}
}
