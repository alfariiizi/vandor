package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVpkgLocalFlow(t *testing.T) {
	tmp := t.TempDir()

	out, err := runRootForTest("new", "vpkg_app", "--path", tmp, "--module", "github.com/acme/vpkg_app", "--tidy", "never")
	if err != nil {
		t.Fatalf("new command failed: %v\nout=%s", err, out)
	}
	projectRoot := filepath.Join(tmp, "vpkg_app")
	pkgPath := createLocalTestPackage(t)

	out, err = runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg add failed: %v\nout=%s", err, out)
	}

	targetFile := filepath.Join(projectRoot, "internal", "infrastructure", "hello", "config.txt")
	if _, err := os.Stat(targetFile); err != nil {
		t.Fatalf("expected copied target file: %v", err)
	}

	out, err = runRootForTest("vpkg", "list", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg list failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "official/infra-hello@0.1.0") {
		t.Fatalf("unexpected list output: %s", out)
	}

	out, err = runRootForTest("vpkg", "doctor", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg doctor failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "healthy") {
		t.Fatalf("unexpected doctor output: %s", out)
	}

	out, err = runRootForTest("vpkg", "exec", "official/infra-hello", "say-hello", "foo", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg exec failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "hello-action foo") {
		t.Fatalf("expected action output, got: %s", out)
	}

	out, err = runRootForTest("vpkg", "exec-alias", "add:hello", "bar", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg exec-alias failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "hello-action bar") {
		t.Fatalf("expected alias action output, got: %s", out)
	}

	out, err = runRootForTest("vpkg", "sync", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg sync failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "Synced 1 package(s)") {
		t.Fatalf("unexpected sync output: %s", out)
	}

	out, err = runRootForTest("vpkg", "remove", "official/infra-hello", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg remove failed: %v\nout=%s", err, out)
	}
	if _, err := os.Stat(targetFile); err == nil {
		t.Fatalf("expected target file removed")
	}
}

func TestVpkgDoctorDetectsDrift(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "drift_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "drift_app")
	pkgPath := createLocalTestPackage(t)
	if _, err := runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg add failed: %v", err)
	}
	targetFile := filepath.Join(projectRoot, "internal", "infrastructure", "hello", "config.txt")
	if err := os.WriteFile(targetFile, []byte("drifted"), 0o644); err != nil {
		t.Fatalf("write drift file: %v", err)
	}
	out, err := runRootForTest("vpkg", "doctor", "--path", projectRoot)
	if err == nil {
		t.Fatalf("expected doctor to fail on drift")
	}
	if !strings.Contains(out, "OWNERSHIP_FILE_HASH_MISMATCH") {
		t.Fatalf("expected doctor issue code in output, got: %s", out)
	}
}

func TestVpkgDoctorFixRepairsDrift(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "doctor_fix_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "doctor_fix_app")
	pkgPath := createLocalTestPackage(t)
	if _, err := runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg add failed: %v", err)
	}

	targetFile := filepath.Join(projectRoot, "internal", "infrastructure", "hello", "config.txt")
	if err := os.WriteFile(targetFile, []byte("drifted"), 0o644); err != nil {
		t.Fatalf("write drift file: %v", err)
	}

	if _, err := runRootForTest("vpkg", "doctor", "--path", projectRoot); err == nil {
		t.Fatalf("expected doctor to fail before fix")
	}

	out, err := runRootForTest("vpkg", "doctor", "--fix", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg doctor --fix failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "fixed") {
		t.Fatalf("expected fix output, got: %s", out)
	}

	finalOut, err := runRootForTest("vpkg", "doctor", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg doctor after fix failed: %v\nout=%s", err, finalOut)
	}
	if !strings.Contains(finalOut, "healthy") {
		t.Fatalf("expected healthy report after fix, got: %s", finalOut)
	}
	raw, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("read repaired file: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "hello" {
		t.Fatalf("expected repaired file content 'hello', got %q", string(raw))
	}
}

func TestVpkgAddPlanOnly(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "plan_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "plan_app")
	pkgPath := createLocalTestPackage(t)
	out, err := runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot, "--plan")
	if err != nil {
		t.Fatalf("vpkg add --plan failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "dry-run") {
		t.Fatalf("expected dry-run output, got: %s", out)
	}
	targetFile := filepath.Join(projectRoot, "internal", "infrastructure", "hello", "config.txt")
	if _, err := os.Stat(targetFile); err == nil {
		t.Fatalf("target file should not be created in --plan mode")
	}
}

func TestVpkgGitSourceFlow(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "git_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "git_app")
	gitSource := createLocalGitSourcePackage(t)
	out, err := runRootForTest("vpkg", "add", gitSource, "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg add git source failed: %v\nout=%s", err, out)
	}
	targetFile := filepath.Join(projectRoot, "internal", "infrastructure", "hello", "config.txt")
	if _, err := os.Stat(targetFile); err != nil {
		t.Fatalf("expected file from git package: %v", err)
	}
}

func TestVpkgRegistryCommands(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "registry_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "registry_app")
	out, err := runRootForTest("vpkg", "registry", "add", "official", "./vpkg-registry", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg registry add failed: %v\nout=%s", err, out)
	}
	out, err = runRootForTest("vpkg", "registry", "list", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg registry list failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "official => ./vpkg-registry") {
		t.Fatalf("unexpected registry list output: %s", out)
	}
	out, err = runRootForTest("vpkg", "registry", "remove", "official", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg registry remove failed: %v\nout=%s", err, out)
	}
}

func TestVpkgSearch(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "search_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "search_app")
	pkgPath := createLocalTestPackage(t)
	registryRoot := createLocalRegistryIndexForPackage(t, "infra-hello", pkgPath)
	if _, err := runRootForTest("vpkg", "registry", "add", "official", registryRoot, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg registry add failed: %v", err)
	}

	out, err := runRootForTest("vpkg", "search", "hello", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg search failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "official/infra-hello@0.1.0") {
		t.Fatalf("expected package in search output:\n%s", out)
	}
	if !strings.Contains(out, "metadata=packages/official/infra-hello.json") {
		t.Fatalf("expected metadata in search output:\n%s", out)
	}
	if !strings.Contains(out, "showing 1 of 1") {
		t.Fatalf("expected search summary output, got:\n%s", out)
	}

	out, err = runRootForTest("vpkg", "search", "hello", "--limit", "1", "--offset", "1", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg search with pagination should succeed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "No vpkg packages found") {
		t.Fatalf("expected empty paginated output, got:\n%s", out)
	}

	out, err = runRootForTest("vpkg", "search", "hello", "--offset", "-1", "--path", projectRoot)
	if err == nil {
		t.Fatalf("expected negative offset to fail; out=%s", out)
	}
	if !strings.Contains(out, "--offset cannot be negative") {
		t.Fatalf("unexpected negative offset error output:\n%s", out)
	}

	out, err = runRootForTest("vpkg", "search", "--tier", "community", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg search --tier failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "No vpkg packages found") {
		t.Fatalf("unexpected tier-filtered search output:\n%s", out)
	}

	out, err = runRootForTest("vpkg", "search", "hello", "--registry", "official", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg search --registry failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "official/infra-hello@0.1.0") {
		t.Fatalf("unexpected registry-filtered search output:\n%s", out)
	}

	out, err = runRootForTest("vpkg", "search", "hello", "--registry", "does-not-exist", "--path", projectRoot)
	if err == nil {
		t.Fatalf("expected unknown registry to fail; out=%s", out)
	}
	if !strings.Contains(out, "registry \"does-not-exist\" not found") {
		t.Fatalf("unexpected unknown registry error output:\n%s", out)
	}
}

func TestVpkgInfo(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "info_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "info_app")
	pkgPath := createLocalTestPackage(t)
	registryRoot := createLocalRegistryIndexForPackage(t, "infra-hello", pkgPath)
	if _, err := runRootForTest("vpkg", "registry", "add", "official", registryRoot, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg registry add failed: %v", err)
	}

	out, err := runRootForTest("vpkg", "info", "official/infra-hello", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg info before install failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "- installed: false") {
		t.Fatalf("expected info output to show not installed:\n%s", out)
	}
	if !strings.Contains(out, "Install:") || !strings.Contains(out, "vandor vpkg add official/infra-hello") {
		t.Fatalf("expected install hint in info output:\n%s", out)
	}

	out, err = runRootForTest("vpkg", "add", "official/infra-hello", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg add failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "Next steps:") || !strings.Contains(out, "vandor vpkg info official/infra-hello") {
		t.Fatalf("expected add output next-step hint:\n%s", out)
	}

	out, err = runRootForTest("vpkg", "info", "official/infra-hello", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg info after install failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "- installed: true") {
		t.Fatalf("expected info output to show installed:\n%s", out)
	}
	if !strings.Contains(out, "Actions:") || !strings.Contains(out, "vandor vpkg exec official/infra-hello say-hello") {
		t.Fatalf("expected actions usage in info output:\n%s", out)
	}
	if !strings.Contains(out, "Aliases:") || !strings.Contains(out, "vandor add:hello") {
		t.Fatalf("expected aliases usage in info output:\n%s", out)
	}
}

func TestVpkgAddBareAliasResolvesFromRegistry(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "alias_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "alias_app")
	pkgPath := createLocalTestPackage(t)
	registryRoot := createLocalRegistryIndexForPackage(t, "infra-hello", pkgPath)
	if _, err := runRootForTest("vpkg", "registry", "add", "official", registryRoot, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg registry add failed: %v", err)
	}

	out, err := runRootForTest("vpkg", "add", "infra-hello", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg add bare alias failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "Installed package official/infra-hello@0.1.0") {
		t.Fatalf("unexpected add output: %s", out)
	}
	targetFile := filepath.Join(projectRoot, "internal", "infrastructure", "hello", "config.txt")
	if _, err := os.Stat(targetFile); err != nil {
		t.Fatalf("expected file from bare alias install: %v", err)
	}
}

func TestVpkgAutoInstallDependencies(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "deps_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "deps_app")
	mainPkgPath, depPkgPath := createLocalPackageWithDependency(t)
	out, err := runRootForTest("vpkg", "add", mainPkgPath, "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg add with dependency failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "Installed package official/infra-main@0.1.0") {
		t.Fatalf("unexpected add output: %s", out)
	}
	mainTarget := filepath.Join(projectRoot, "internal", "infrastructure", "main", "config.txt")
	depTarget := filepath.Join(projectRoot, "internal", "infrastructure", "dep", "config.txt")
	if _, err := os.Stat(mainTarget); err != nil {
		t.Fatalf("expected main target file: %v", err)
	}
	if _, err := os.Stat(depTarget); err != nil {
		t.Fatalf("expected dependency target file: %v", err)
	}
	out, err = runRootForTest("vpkg", "list", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg list failed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "official/infra-main@0.1.0") || !strings.Contains(out, "official/infra-dep@0.1.0") {
		t.Fatalf("expected both packages installed; output=%s", out)
	}
	if !strings.Contains(out, depPkgPath) {
		t.Fatalf("expected dependency source in list output; output=%s", out)
	}

	out, err = runRootForTest("vpkg", "remove", "official/infra-dep", "--path", projectRoot)
	if err == nil {
		t.Fatalf("expected remove dependency package to fail when still required; out=%s", out)
	}
	if !strings.Contains(out, "required by") {
		t.Fatalf("unexpected remove error output: %s", out)
	}

	out, err = runRootForTest("vpkg", "remove", "official/infra-dep", "--force", "--path", projectRoot)
	if err != nil {
		t.Fatalf("force remove dependency package should succeed: %v\nout=%s", err, out)
	}
	if !strings.Contains(out, "Force remove preview for official/infra-dep") {
		t.Fatalf("expected force remove preview output, got: %s", out)
	}
}

func TestVpkgInteractiveConfirmAbort(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "interactive_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "interactive_app")
	pkgPath := createLocalTestPackage(t)
	_, err := runRootForTestWithInput("n\n", "-it", "vpkg", "add", pkgPath, "--path", projectRoot)
	if err == nil {
		t.Fatalf("expected interactive add to abort when user answers no")
	}
	targetFile := filepath.Join(projectRoot, "internal", "infrastructure", "hello", "config.txt")
	if _, statErr := os.Stat(targetFile); statErr == nil {
		t.Fatalf("target file should not be created when add aborted")
	}
}

func TestVpkgOfficialStarterFlowFromWorkspaceRegistry(t *testing.T) {
	registryRoot, ok := findWorkspaceVpkgRepo()
	if !ok {
		t.Skip("workspace vpkg registry not found; skipping official starter integration flow")
	}

	tmp := t.TempDir()
	if _, err := runRootForTest("new", "official_starter_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "official_starter_app")

	t.Setenv("VANDOR_VPKG_REGISTRY_OFFICIAL", registryRoot)
	t.Setenv("VANDOR_VPKG_REGISTRY_VERIFIED", registryRoot)
	t.Setenv("VANDOR_VPKG_REGISTRY_COMMUNITY", registryRoot)

	steps := []struct {
		source string
		expect string
	}{
		{source: "@official/http-humachi", expect: "official/http-humachi@0.1.0"},
		{source: "@official/entgo", expect: "official/entgo@0.1.0"},
		{source: "@official/atlas", expect: "official/atlas@0.1.0"},
	}
	for _, step := range steps {
		out, err := runRootForTest("vpkg", "add", step.source, "--path", projectRoot)
		if err != nil {
			t.Fatalf("vpkg add %s failed: %v\nout=%s", step.source, err, out)
		}
		if !strings.Contains(out, step.expect) {
			t.Fatalf("unexpected add output for %s:\n%s", step.source, out)
		}
	}

	expectedFiles := []string{
		filepath.Join(projectRoot, "internal", "transport", "http", "module.go"),
		filepath.Join(projectRoot, "internal", "infrastructure", "entgo", "module.go"),
		filepath.Join(projectRoot, "internal", "infrastructure", "atlas", "atlas.go"),
		filepath.Join(projectRoot, "config", "fragments", "atlas.yaml"),
		filepath.Join(projectRoot, "tools", "vpkg", "http-humachi", "add_handler", "main.go"),
		filepath.Join(projectRoot, "tools", "vpkg", "entgo", "entgo", "main.go"),
		filepath.Join(projectRoot, "tools", "vpkg", "atlas", "atlas", "main.go"),
	}
	for _, path := range expectedFiles {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated file %s: %v", path, err)
		}
	}

	listOut, err := runRootForTest("vpkg", "list", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg list failed: %v\nout=%s", err, listOut)
	}
	if !strings.Contains(listOut, "official/http-humachi@0.1.0") ||
		!strings.Contains(listOut, "official/entgo@0.1.0") ||
		!strings.Contains(listOut, "official/atlas@0.1.0") {
		t.Fatalf("expected all official starters in list output:\n%s", listOut)
	}
}

func createLocalTestPackage(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "infra-hello")
	if err := os.MkdirAll(filepath.Join(root, "templates", "hello"), 0o755); err != nil {
		t.Fatalf("mkdir package: %v", err)
	}
	manifest := `apiVersion: vpkg.v1
name: official/infra-hello
version: 0.1.0
tier: official
kind: runtime
targets:
  - from: templates/hello/config.txt
    to: internal/infrastructure/hello/config.txt
    mode: copy
    conflict: overwrite
actions:
  - name: say-hello
    run: echo hello-action
aliases:
  - name: add:hello
    action: say-hello
permissions:
  write:
    - internal/infrastructure/**
  exec: true
`
	if err := os.WriteFile(filepath.Join(root, "vpkg.manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "templates", "hello", "config.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}
	return root
}

func createLocalGitSourcePackage(t *testing.T) string {
	t.Helper()
	repoRoot := filepath.Join(t.TempDir(), "vpkg-repo")
	pkgRoot := filepath.Join(repoRoot, "packages", "infra-hello")
	if err := os.MkdirAll(filepath.Join(pkgRoot, "templates", "hello"), 0o755); err != nil {
		t.Fatalf("mkdir package: %v", err)
	}
	manifest := `apiVersion: vpkg.v1
name: official/infra-hello
version: 0.1.1
tier: official
kind: runtime
targets:
  - from: templates/hello/config.txt
    to: internal/infrastructure/hello/config.txt
    mode: copy
    conflict: overwrite
permissions:
  write:
    - internal/infrastructure/**
  exec: false
`
	if err := os.WriteFile(filepath.Join(pkgRoot, "vpkg.manifest.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pkgRoot, "templates", "hello", "config.txt"), []byte("from-git"), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}
	if out, err := exec.Command("git", "init", repoRoot).CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v output=%s", err, string(out))
	}
	cmd := exec.Command("git", "-C", repoRoot, "add", ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v output=%s", err, string(out))
	}
	commitCmd := exec.Command("git", "-C", repoRoot, "commit", "-m", "init")
	commitCmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=vpkg-test",
		"GIT_AUTHOR_EMAIL=vpkg-test@example.com",
		"GIT_COMMITTER_NAME=vpkg-test",
		"GIT_COMMITTER_EMAIL=vpkg-test@example.com",
	)
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v output=%s", err, string(out))
	}
	return "file://" + repoRoot + "//packages/infra-hello"
}

func createLocalPackageWithDependency(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	depRoot := filepath.Join(root, "infra-dep")
	mainRoot := filepath.Join(root, "infra-main")
	if err := os.MkdirAll(filepath.Join(depRoot, "templates", "dep"), 0o755); err != nil {
		t.Fatalf("mkdir dep package: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(mainRoot, "templates", "main"), 0o755); err != nil {
		t.Fatalf("mkdir main package: %v", err)
	}
	depManifest := `apiVersion: vpkg.v1
name: official/infra-dep
version: 0.1.0
tier: official
kind: runtime
targets:
  - from: templates/dep/config.txt
    to: internal/infrastructure/dep/config.txt
    mode: copy
    conflict: overwrite
permissions:
  write:
    - internal/infrastructure/**
  exec: false
`
	mainManifest := `apiVersion: vpkg.v1
name: official/infra-main
version: 0.1.0
tier: official
kind: runtime
dependencies:
  - source: ` + depRoot + `
targets:
  - from: templates/main/config.txt
    to: internal/infrastructure/main/config.txt
    mode: copy
    conflict: overwrite
permissions:
  write:
    - internal/infrastructure/**
  exec: false
`
	if err := os.WriteFile(filepath.Join(depRoot, "vpkg.manifest.yaml"), []byte(depManifest), 0o644); err != nil {
		t.Fatalf("write dep manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainRoot, "vpkg.manifest.yaml"), []byte(mainManifest), 0o644); err != nil {
		t.Fatalf("write main manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depRoot, "templates", "dep", "config.txt"), []byte("dep"), 0o644); err != nil {
		t.Fatalf("write dep template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mainRoot, "templates", "main", "config.txt"), []byte("main"), 0o644); err != nil {
		t.Fatalf("write main template: %v", err)
	}
	return mainRoot, depRoot
}

func createLocalRegistryIndexForPackage(t *testing.T, pkgName, pkgSource string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "registry")
	indexPath := filepath.Join(root, "packages", "official")
	if err := os.MkdirAll(indexPath, 0o755); err != nil {
		t.Fatalf("mkdir registry index: %v", err)
	}
	rootIndex := fmt.Sprintf(`{
  "packages": [
    {
      "name": "official/%s",
      "tier": "official",
      "latest": "0.1.0",
      "metadata": "packages/official/%s.json"
    }
  ]
}`, pkgName, pkgName)
	if err := os.WriteFile(filepath.Join(root, "index.json"), []byte(rootIndex), 0o644); err != nil {
		t.Fatalf("write registry root index: %v", err)
	}
	record := fmt.Sprintf(`{
  "name": "official/%s",
  "latest": "0.1.0",
  "versions": [
    {"version":"0.1.0","source":"%s"}
  ]
}`, pkgName, filepath.ToSlash(pkgSource))
	if err := os.WriteFile(filepath.Join(indexPath, pkgName+".json"), []byte(record), 0o644); err != nil {
		t.Fatalf("write registry package metadata: %v", err)
	}
	return root
}

func findWorkspaceVpkgRepo() (string, bool) {
	if env := strings.TrimSpace(os.Getenv("VANDOR_TEST_VPKG_REPO")); env != "" {
		if stat, err := os.Stat(env); err == nil && stat.IsDir() {
			if _, err := os.Stat(filepath.Join(env, "packages")); err == nil {
				return env, true
			}
		}
	}

	candidates := []string{
		"../vpkg",
		"../../vpkg",
		"/home/alfarizi/dev/golang/vandor/alfariiizi-vandor/vandor-0.4/vpkg",
	}
	for _, candidate := range candidates {
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		stat, err := os.Stat(abs)
		if err != nil || !stat.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(abs, "packages")); err != nil {
			continue
		}
		return abs, true
	}
	return "", false
}
