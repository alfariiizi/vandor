package vpkg

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseGitSourceSpec(t *testing.T) {
	tests := []struct {
		input  string
		ok     bool
		repo   string
		subdir string
		ref    string
	}{
		{
			input:  "github.com/acme/vpkg//packages/infra-http@v1.2.0",
			ok:     true,
			repo:   "https://github.com/acme/vpkg",
			subdir: "packages/infra-http",
			ref:    "v1.2.0",
		},
		{
			input:  "https://github.com/acme/vpkg//packages/infra-http@v1.2.0",
			ok:     true,
			repo:   "https://github.com/acme/vpkg",
			subdir: "packages/infra-http",
			ref:    "v1.2.0",
		},
		{
			input: "not-a-git-source",
			ok:    false,
		},
	}
	for _, tc := range tests {
		got, ok := parseGitSourceSpec(tc.input)
		if ok != tc.ok {
			t.Fatalf("input=%q expected ok=%v got=%v", tc.input, tc.ok, ok)
		}
		if !ok {
			continue
		}
		if got.Repo != tc.repo || got.Subdir != tc.subdir || got.Ref != tc.ref {
			t.Fatalf("input=%q unexpected parsed value: %#v", tc.input, got)
		}
	}
}

func TestParseRegistryPackageSource(t *testing.T) {
	raw := []byte(`{
  "name":"official/transport-http",
  "latest":"1.1.0",
  "versions":[
    {"version":"1.0.0","source":"./packages/official/transport-http/v1.0.0"},
    {"version":"1.1.0","source":"./packages/official/transport-http/v1.1.0"}
  ]
}`)
	got, err := parseRegistryPackageSource(raw, "")
	if err != nil {
		t.Fatalf("parse latest: %v", err)
	}
	if got != "./packages/official/transport-http/v1.1.0" {
		t.Fatalf("unexpected latest source: %s", got)
	}
	got, err = parseRegistryPackageSource(raw, "1.0.0")
	if err != nil {
		t.Fatalf("parse versioned: %v", err)
	}
	if got != "./packages/official/transport-http/v1.0.0" {
		t.Fatalf("unexpected versioned source: %s", got)
	}
}

func TestResolveRegistryIndexSourceHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/packages/official/transport-http.json" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{
  "name":"official/transport-http",
  "versions":[{"version":"1.2.3","source":"github.com/acme/vpkg//packages/transport-http@v1.2.3"}]
}`))
	}))
	defer server.Close()

	m := NewManager(t.TempDir())
	source, err := m.resolveRegistryIndexSource(RegistryRef{
		Name: "official",
		URL:  server.URL,
	}, "official", "transport-http", "1.2.3")
	if err != nil {
		t.Fatalf("resolve registry index source: %v", err)
	}
	if source != "github.com/acme/vpkg//packages/transport-http@v1.2.3" {
		t.Fatalf("unexpected source: %s", source)
	}
}

func TestSearchFromRegistryIndex(t *testing.T) {
	projectRoot := t.TempDir()
	registryRoot := filepath.Join(t.TempDir(), "registry")
	if err := os.MkdirAll(filepath.Join(registryRoot, "packages", "official"), 0o755); err != nil {
		t.Fatalf("mkdir registry: %v", err)
	}
	index := `{
  "packages": [
    {
      "name": "official/http-humachi",
      "tier": "official",
      "latest": "0.1.0",
      "metadata": "packages/official/http-humachi.json"
    }
  ]
}`
	if err := os.WriteFile(filepath.Join(registryRoot, "index.json"), []byte(index), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	m := NewManager(projectRoot)
	if _, err := m.RegistryAdd("official", registryRoot); err != nil {
		t.Fatalf("registry add: %v", err)
	}

	results, err := m.Search("huma", "", "")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 search result, got %d", len(results))
	}
	if results[0].Name != "official/http-humachi" {
		t.Fatalf("unexpected search result: %#v", results[0])
	}
}

func TestSearchInvalidTier(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.Search("", "enterprise", "")
	if err == nil {
		t.Fatalf("expected invalid tier error")
	}
}

func TestSearchRegistryFilter(t *testing.T) {
	projectRoot := t.TempDir()
	officialRoot := filepath.Join(t.TempDir(), "official-reg")
	communityRoot := filepath.Join(t.TempDir(), "community-reg")
	if err := os.MkdirAll(officialRoot, 0o755); err != nil {
		t.Fatalf("mkdir official registry: %v", err)
	}
	if err := os.MkdirAll(communityRoot, 0o755); err != nil {
		t.Fatalf("mkdir community registry: %v", err)
	}
	officialIndex := `{"packages":[{"name":"official/http-humachi","tier":"official","latest":"0.1.0"}]}`
	communityIndex := `{"packages":[{"name":"community/http-light","tier":"community","latest":"0.2.0"}]}`
	if err := os.WriteFile(filepath.Join(officialRoot, "index.json"), []byte(officialIndex), 0o644); err != nil {
		t.Fatalf("write official index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(communityRoot, "index.json"), []byte(communityIndex), 0o644); err != nil {
		t.Fatalf("write community index: %v", err)
	}

	m := NewManager(projectRoot)
	if _, err := m.RegistryAdd("official", officialRoot); err != nil {
		t.Fatalf("registry add official: %v", err)
	}
	if _, err := m.RegistryAdd("community", communityRoot); err != nil {
		t.Fatalf("registry add community: %v", err)
	}

	results, err := m.Search("http", "", "official")
	if err != nil {
		t.Fatalf("search with registry filter failed: %v", err)
	}
	if len(results) != 1 || results[0].Registry != "official" {
		t.Fatalf("unexpected filtered search results: %#v", results)
	}
}

func TestLoadRegistryIndexRetries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"temporary"}`))
			return
		}
		_, _ = w.Write([]byte(`{"packages":[{"name":"official/http-humachi","tier":"official","latest":"0.1.0"}]}`))
	}))
	defer server.Close()

	m := NewManager(t.TempDir())
	m.HTTPTimeout = time.Second
	m.HTTPRetries = 2

	index, err := m.loadRegistryIndex(RegistryRef{
		Name: "official",
		URL:  server.URL,
	})
	if err != nil {
		t.Fatalf("load registry index should succeed after retry: %v", err)
	}
	if len(index.Packages) != 1 {
		t.Fatalf("unexpected index payload: %#v", index)
	}
	if attempts < 2 {
		t.Fatalf("expected retry attempts, got %d", attempts)
	}
}

func TestNormalizeRegistrySourceURLRawGitHubRelative(t *testing.T) {
	got := normalizeRegistrySourceURL("https://raw.githubusercontent.com/alfariiizi/vpkg/main", "./packages/official/http-humachi")
	want := "github.com/alfariiizi/vpkg//packages/official/http-humachi@main"
	if got != want {
		t.Fatalf("unexpected normalized source:\nwant: %s\ngot : %s", want, got)
	}
}

func TestNormalizeRegistrySourceURLNonRawRelative(t *testing.T) {
	got := normalizeRegistrySourceURL("https://registry.example.com", "./packages/official/http-humachi")
	want := "https://registry.example.com/./packages/official/http-humachi"
	if got != want {
		t.Fatalf("unexpected normalized source:\nwant: %s\ngot : %s", want, got)
	}
}

func TestDoctorDetectsDependencyAndContractDrift(t *testing.T) {
	projectRoot := t.TempDir()
	depPkg := filepath.Join(t.TempDir(), "dep")
	mainPkg := filepath.Join(t.TempDir(), "main")

	writeDoctorPackage(t, depPkg, `apiVersion: vpkg.v1
name: official/dep
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
`)

	writeDoctorPackage(t, mainPkg, `apiVersion: vpkg.v1
name: official/main
version: 0.1.0
tier: official
kind: runtime
dependencies:
  - source: `+depPkg+`
targets:
  - from: templates/main/config.txt
    to: internal/infrastructure/main/config.txt
    mode: copy
    conflict: overwrite
actions:
  - name: run-main
    run: echo main
aliases:
  - name: add:main
    action: run-main
permissions:
  write:
    - internal/infrastructure/**
  exec: true
`)

	m := NewManager(projectRoot)
	if _, err := m.Add(mainPkg, AddOptions{}); err != nil {
		t.Fatalf("add main package: %v", err)
	}

	lock, err := LoadLock(projectRoot)
	if err != nil {
		t.Fatalf("load lock: %v", err)
	}
	filtered := lock.Packages[:0]
	for _, pkg := range lock.Packages {
		if pkg.Name == "official/main" {
			pkg.Actions = []Action{{Name: "run-main", Run: "echo CHANGED"}}
			pkg.Aliases = []Alias{{Name: "add:main", Action: "run-missing"}}
			filtered = append(filtered, pkg)
			continue
		}
		if pkg.Name == "official/dep" {
			continue
		}
		filtered = append(filtered, pkg)
	}
	lock.Packages = filtered
	if err := SaveLock(projectRoot, lock); err != nil {
		t.Fatalf("save tampered lock: %v", err)
	}

	report, err := m.Doctor()
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if report.Healthy {
		t.Fatalf("expected unhealthy report")
	}
	if !hasDoctorIssue(report, "dependency", "missing dependency") {
		t.Fatalf("expected missing dependency issue, got %+v", report.Issues)
	}
	if !hasDoctorIssueCode(report, "DEP_MISSING") {
		t.Fatalf("expected DEP_MISSING code, got %+v", report.Issues)
	}
	if !hasDoctorIssue(report, "action", "run mismatch") {
		t.Fatalf("expected action run mismatch issue, got %+v", report.Issues)
	}
	if !hasDoctorIssue(report, "alias", "action mismatch") {
		t.Fatalf("expected alias mismatch issue, got %+v", report.Issues)
	}
}

func TestDoctorDetectsExecContractIssue(t *testing.T) {
	projectRoot := t.TempDir()
	pkgRoot := filepath.Join(t.TempDir(), "exec-contract")
	writeDoctorPackage(t, pkgRoot, `apiVersion: vpkg.v1
name: official/exec-contract
version: 0.1.0
tier: official
kind: runtime
targets:
  - from: templates/exec_contract/config.txt
    to: internal/infrastructure/exec_contract/config.txt
    mode: copy
    conflict: overwrite
actions:
  - name: do-thing
    run: echo hi
permissions:
  write:
    - internal/infrastructure/**
  exec: false
`)

	m := NewManager(projectRoot)
	if _, err := m.Add(pkgRoot, AddOptions{}); err != nil {
		t.Fatalf("add package: %v", err)
	}

	report, err := m.Doctor()
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if report.Healthy {
		t.Fatalf("expected unhealthy report")
	}
	if !hasDoctorIssue(report, "action", "permissions.exec=false") {
		t.Fatalf("expected exec contract issue, got %+v", report.Issues)
	}
}

func TestExecAlias(t *testing.T) {
	projectRoot := t.TempDir()
	pkgRoot := filepath.Join(t.TempDir(), "alias-package")
	writeDoctorPackage(t, pkgRoot, `apiVersion: vpkg.v1
name: official/alias-package
version: 0.1.0
tier: official
kind: runtime
targets:
  - from: templates/alias_package/config.txt
    to: internal/infrastructure/alias_package/config.txt
    mode: copy
    conflict: overwrite
actions:
  - name: say-hello
    run: echo hello-alias
aliases:
  - name: add:alias-package
    action: say-hello
permissions:
  write:
    - internal/infrastructure/**
  exec: true
`)

	m := NewManager(projectRoot)
	if _, err := m.Add(pkgRoot, AddOptions{}); err != nil {
		t.Fatalf("add package: %v", err)
	}

	var out bytes.Buffer
	result, err := m.ExecAlias("add:alias-package", []string{"foo"}, &out, &out)
	if err != nil {
		t.Fatalf("exec alias failed: %v", err)
	}
	if result.Package != "official/alias-package" || result.Action != "say-hello" {
		t.Fatalf("unexpected exec alias result: %#v", result)
	}
	if !strings.Contains(out.String(), "hello-alias foo") {
		t.Fatalf("expected alias command output, got: %s", out.String())
	}
}

func TestRemoveBlocksWhenPackageHasDependents(t *testing.T) {
	projectRoot := t.TempDir()
	depPkg := filepath.Join(t.TempDir(), "dep")
	mainPkg := filepath.Join(t.TempDir(), "main")

	writeDoctorPackage(t, depPkg, `apiVersion: vpkg.v1
name: official/dep
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
`)

	writeDoctorPackage(t, mainPkg, `apiVersion: vpkg.v1
name: official/main
version: 0.1.0
tier: official
kind: runtime
dependencies:
  - source: `+depPkg+`
targets:
  - from: templates/main/config.txt
    to: internal/infrastructure/main/config.txt
    mode: copy
    conflict: overwrite
permissions:
  write:
    - internal/infrastructure/**
  exec: false
`)

	m := NewManager(projectRoot)
	if _, err := m.Add(mainPkg, AddOptions{}); err != nil {
		t.Fatalf("add main package: %v", err)
	}

	_, err := m.Remove("official/dep", RemoveOptions{})
	if err == nil {
		t.Fatalf("expected remove to fail when dependent package exists")
	}
	if !strings.Contains(err.Error(), "required by") {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := m.Remove("official/dep", RemoveOptions{Force: true}); err != nil {
		t.Fatalf("force remove should succeed: %v", err)
	}
}

func TestInfoInstalledPackage(t *testing.T) {
	projectRoot := t.TempDir()
	pkgRoot := filepath.Join(t.TempDir(), "info-package")
	writeDoctorPackage(t, pkgRoot, `apiVersion: vpkg.v1
name: official/info-package
version: 0.1.0
tier: official
kind: runtime
description: package info test
capabilities:
  - command:test
targets:
  - from: templates/pkg/config.txt
    to: internal/infrastructure/info_package/config.txt
    mode: copy
    conflict: overwrite
actions:
  - name: say-info
    run: echo info
aliases:
  - name: add:info-package
    action: say-info
permissions:
  write:
    - internal/infrastructure/**
  exec: true
`)
	if err := os.WriteFile(filepath.Join(pkgRoot, "README.md"), []byte("# info package"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	m := NewManager(projectRoot)
	if _, err := m.Add(pkgRoot, AddOptions{}); err != nil {
		t.Fatalf("add package: %v", err)
	}

	info, err := m.Info("official/info-package")
	if err != nil {
		t.Fatalf("info failed: %v", err)
	}
	if !info.Installed {
		t.Fatalf("expected installed info")
	}
	if info.Name != "official/info-package" {
		t.Fatalf("unexpected info name: %s", info.Name)
	}
	if len(info.Actions) != 1 || info.Actions[0].Name != "say-info" {
		t.Fatalf("unexpected actions: %+v", info.Actions)
	}
	if len(info.Aliases) != 1 || info.Aliases[0].Name != "add:info-package" {
		t.Fatalf("unexpected aliases: %+v", info.Aliases)
	}
	if info.ReadmePath == "" {
		t.Fatalf("expected readme path in info payload")
	}
}

func TestDependencyInstalledMatchesAliasVariants(t *testing.T) {
	lock := Lock{
		Packages: []LockedPackage{
			{
				Name:   "official/http-humachi",
				Source: "@official/http-humachi@0.1.0",
			},
		},
	}
	cases := []struct {
		source string
		want   bool
	}{
		{source: "@official/http-humachi@0.1.0", want: true},
		{source: "@official/http-humachi", want: true},
		{source: "official/http-humachi@0.1.0", want: true},
		{source: "official/http-humachi", want: true},
		{source: "@verified/http-humachi", want: false},
		{source: "github.com/acme/repo//packages/http-humachi@main", want: false},
	}
	for _, tc := range cases {
		got := dependencyInstalled(lock, tc.source)
		if got != tc.want {
			t.Fatalf("source=%q expected %v got %v", tc.source, tc.want, got)
		}
	}
}

func TestRemoveFailsWhenDependentManifestCannotBeLoaded(t *testing.T) {
	projectRoot := t.TempDir()
	m := NewManager(projectRoot)

	depCache := filepath.Join(projectRoot, ".vandor", "vpkg", "cache", "official_dep_0_1_0")
	mainCache := filepath.Join(projectRoot, ".vandor", "vpkg", "cache", "official_main_0_1_0")
	if err := os.MkdirAll(depCache, 0o755); err != nil {
		t.Fatalf("mkdir dep cache: %v", err)
	}
	if err := os.MkdirAll(mainCache, 0o755); err != nil {
		t.Fatalf("mkdir main cache: %v", err)
	}
	depManifest := `apiVersion: vpkg.v1
name: official/dep
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
	if err := os.MkdirAll(filepath.Join(depCache, "templates", "dep"), 0o755); err != nil {
		t.Fatalf("mkdir dep templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depCache, "templates", "dep", "config.txt"), []byte("dep"), 0o644); err != nil {
		t.Fatalf("write dep template: %v", err)
	}
	if err := os.WriteFile(filepath.Join(depCache, manifestFileName), []byte(depManifest), 0o644); err != nil {
		t.Fatalf("write dep manifest: %v", err)
	}
	// Do not write main manifest to simulate corrupted cache.

	lock := Lock{
		LockVersion: 1,
		Packages: []LockedPackage{
			{
				Name:    "official/dep",
				Source:  "@official/dep@0.1.0",
				Version: "0.1.0",
				Resolved: ResolvedSource{
					CachePath: ".vandor/vpkg/cache/official_dep_0_1_0",
				},
			},
			{
				Name:    "official/main",
				Source:  "@official/main@0.1.0",
				Version: "0.1.0",
				Resolved: ResolvedSource{
					CachePath: ".vandor/vpkg/cache/official_main_0_1_0",
				},
			},
		},
	}
	if err := SaveLock(projectRoot, lock); err != nil {
		t.Fatalf("save lock: %v", err)
	}

	_, err := m.Remove("official/dep", RemoveOptions{})
	if err == nil {
		t.Fatalf("expected remove to fail when dependent manifest is unreadable")
	}
	if !strings.Contains(err.Error(), "cannot inspect dependencies for package") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func writeDoctorPackage(t *testing.T, root, manifest string) {
	t.Helper()
	targetFile := "templates/pkg/config.txt"
	switch {
	case strings.Contains(manifest, "templates/dep/config.txt"):
		targetFile = "templates/dep/config.txt"
	case strings.Contains(manifest, "templates/main/config.txt"):
		targetFile = "templates/main/config.txt"
	case strings.Contains(manifest, "templates/exec_contract/config.txt"):
		targetFile = "templates/exec_contract/config.txt"
	case strings.Contains(manifest, "templates/alias_package/config.txt"):
		targetFile = "templates/alias_package/config.txt"
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.Dir(targetFile)), 0o755); err != nil {
		t.Fatalf("mkdir package templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, manifestFileName), []byte(manifest), 0o644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, targetFile), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write template file: %v", err)
	}
}

func hasDoctorIssue(report DoctorReport, check, contains string) bool {
	for _, issue := range report.Issues {
		if issue.Check != check {
			continue
		}
		if strings.Contains(issue.Message, contains) {
			return true
		}
	}
	return false
}

func hasDoctorIssueCode(report DoctorReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
