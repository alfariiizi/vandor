package vpkg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifestFromDir(t *testing.T) {
	tmp := t.TempDir()
	content := `apiVersion: vpkg.v1
name: official/transport-http
version: 1.0.0
tier: official
kind: runtime
targets:
  - from: templates/http
    to: internal/transport/http
actions:
  - name: add-handler
    run: go run ./tools/add_handler/main.go
aliases:
  - name: add:http-handler
    action: add-handler
permissions:
  exec: true
`
	if err := os.WriteFile(filepath.Join(tmp, manifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	m, _, err := LoadManifestFromDir(tmp)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if m.Name != "official/transport-http" {
		t.Fatalf("unexpected name: %s", m.Name)
	}
	if m.Targets[0].Mode != TargetModeMerge {
		t.Fatalf("expected default mode merge, got %s", m.Targets[0].Mode)
	}
	if m.Targets[0].Conflict != ConflictError {
		t.Fatalf("expected default conflict error, got %s", m.Targets[0].Conflict)
	}
}

func TestLoadManifestAliasScalar(t *testing.T) {
	tmp := t.TempDir()
	content := `apiVersion: vpkg.v1
name: official/transport-http
version: 1.0.0
aliases:
  - add:http-handler=add-handler
`
	if err := os.WriteFile(filepath.Join(tmp, manifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	m, _, err := LoadManifestFromDir(tmp)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if len(m.Aliases) != 1 || m.Aliases[0].Name != "add:http-handler" || m.Aliases[0].Action != "add-handler" {
		t.Fatalf("unexpected aliases: %#v", m.Aliases)
	}
}

func TestManifestValidationFail(t *testing.T) {
	tmp := t.TempDir()
	content := `apiVersion: vpkg.v1
name: INVALID
version: ""
`
	if err := os.WriteFile(filepath.Join(tmp, manifestFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if _, _, err := LoadManifestFromDir(tmp); err == nil {
		t.Fatalf("expected validation error")
	}
}
