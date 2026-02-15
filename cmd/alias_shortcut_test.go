package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTryHandleAliasShortcut(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "alias_shortcut_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "alias_shortcut_app")
	pkgPath := createLocalTestPackage(t)
	if _, err := runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg add failed: %v", err)
	}

	var out bytes.Buffer
	handled, err := tryHandleAliasShortcut(
		fmt.Errorf("unknown command %q for %q", "add:hello", "vandor"),
		[]string{"--path", projectRoot, "add:hello", "foo"},
		&out,
		&out,
	)
	if !handled {
		t.Fatalf("expected alias shortcut to be handled")
	}
	if err != nil {
		t.Fatalf("expected alias shortcut to succeed: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "hello-action foo") {
		t.Fatalf("expected alias command output, got: %s", got)
	}
	if !strings.Contains(got, "Executed alias add:hello") {
		t.Fatalf("expected alias success message, got: %s", got)
	}
}

func TestTryHandleAliasShortcutJSON(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "alias_shortcut_json_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "alias_shortcut_json_app")
	pkgPath := createLocalTestPackage(t)
	if _, err := runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg add failed: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	handled, err := tryHandleAliasShortcut(
		fmt.Errorf("unknown command %q for %q", "add:hello", "vandor"),
		[]string{"--output", "json", "--path", projectRoot, "add:hello"},
		&out,
		&errOut,
	)
	if !handled {
		t.Fatalf("expected alias shortcut to be handled")
	}
	if err != nil {
		t.Fatalf("expected alias shortcut to succeed: %v", err)
	}

	var payload map[string]any
	if decodeErr := json.Unmarshal(out.Bytes(), &payload); decodeErr != nil {
		t.Fatalf("decode json output: %v, raw=%s", decodeErr, out.String())
	}
	message, _ := payload["message"].(string)
	if !strings.Contains(message, "Executed alias add:hello") {
		t.Fatalf("unexpected json message: %v", payload["message"])
	}
	if !strings.Contains(errOut.String(), "hello-action") {
		t.Fatalf("expected action output to be redirected to stderr in json mode, got: %s", errOut.String())
	}
}

func TestTryHandleAliasShortcutSkipsNonAlias(t *testing.T) {
	var out bytes.Buffer
	handled, err := tryHandleAliasShortcut(
		fmt.Errorf("unknown command %q for %q", "foobar", "vandor"),
		[]string{"foobar"},
		&out,
		&out,
	)
	if handled {
		t.Fatalf("expected non-alias unknown command to be ignored")
	}
	if err != nil {
		t.Fatalf("expected nil error for ignored command, got: %v", err)
	}
}

func TestExecuteAliasShortcutFallback(t *testing.T) {
	tmp := t.TempDir()
	if _, err := runRootForTest("new", "alias_execute_app", "--path", tmp, "--tidy", "never"); err != nil {
		t.Fatalf("new command failed: %v", err)
	}
	projectRoot := filepath.Join(tmp, "alias_execute_app")
	pkgPath := createLocalTestPackage(t)
	if _, err := runRootForTest("vpkg", "add", pkgPath, "--path", projectRoot); err != nil {
		t.Fatalf("vpkg add failed: %v", err)
	}

	resetGlobalFlagsForTest()
	var out bytes.Buffer
	var errOut bytes.Buffer
	RootCmd.SetOut(&out)
	RootCmd.SetErr(&errOut)
	RootCmd.SetIn(strings.NewReader(""))
	RootCmd.SetArgs(nil)

	previousArgs := os.Args
	defer func() { os.Args = previousArgs }()
	os.Args = []string{"vandor", "--path", projectRoot, "add:hello", "world"}

	if err := Execute(); err != nil {
		t.Fatalf("execute should fallback to alias shortcut: %v\nstdout=%s\nstderr=%s", err, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "hello-action world") {
		t.Fatalf("expected alias action output on stdout, got: %s", out.String())
	}
	if !strings.Contains(out.String(), "Executed alias add:hello") {
		t.Fatalf("expected alias success message on stdout, got: %s", out.String())
	}
	if strings.Contains(strings.ToLower(errOut.String()), "unknown command") {
		t.Fatalf("did not expect unknown command noise on stderr when alias fallback succeeds, got: %s", errOut.String())
	}
}
