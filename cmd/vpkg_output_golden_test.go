package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVpkgDoctorTextOutputGolden(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "golden_doctor_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "golden_doctor_app")
	pkgPath := createLocalTestPackage(t)
	if _, err := runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg add failed: %v", err)
	}

	out, err := runRootForTest("vpkg", "doctor", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg doctor failed: %v\nout=%s", err, out)
	}
	assertGoldenOutput(t, "doctor_healthy.txt", out)
}

func TestVpkgSearchTextOutputGolden(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "golden_search_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "golden_search_app")
	pkgPath := createLocalTestPackage(t)
	registryRoot := createLocalRegistryIndexForPackage(t, "infra-hello", pkgPath)
	if _, err := runRootForTest("vpkg", "registry", "add", "official", registryRoot, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg registry add failed: %v", err)
	}

	out, err := runRootForTest("vpkg", "search", "hello", "--registry", "official", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg search failed: %v\nout=%s", err, out)
	}
	assertGoldenOutput(t, "search_basic.txt", out)
}

func TestVpkgExecAliasTextOutputGolden(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "golden_exec_alias_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "golden_exec_alias_app")
	pkgPath := createLocalTestPackage(t)
	if _, err := runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg add failed: %v", err)
	}

	out, err := runRootForTest("vpkg", "exec-alias", "add:hello", "golden", "--path", projectRoot)
	if err != nil {
		t.Fatalf("vpkg exec-alias failed: %v\nout=%s", err, out)
	}
	assertGoldenOutput(t, "exec_alias.txt", out)
}

func assertGoldenOutput(t *testing.T, filename, got string) {
	t.Helper()
	goldenPath := filepath.Join("testdata", "golden", filename)
	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file %s: %v", goldenPath, err)
	}
	want := normalizeGolden(string(wantBytes))
	normalizedGot := normalizeGolden(got)
	if normalizedGot != want {
		t.Fatalf("golden mismatch for %s\n--- want ---\n%s\n--- got ---\n%s", filename, want, normalizedGot)
	}
}

func normalizeGolden(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.TrimSpace(value) + "\n"
}
