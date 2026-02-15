package vpkg

import (
	"fmt"
	"sort"
	"strings"
)

const (
	APIVersionV1 = "vpkg.v1"

	TierOfficial  = "official"
	TierVerified  = "verified"
	TierCommunity = "community"

	KindRuntime = "runtime"
	KindHelper  = "helper"
	KindTooling = "tooling"

	TargetModeCopy     = "copy"
	TargetModeMerge    = "merge"
	TargetModeTemplate = "template"

	ConflictError     = "error"
	ConflictSkip      = "skip"
	ConflictOverwrite = "overwrite"
	ConflictBackup    = "backup"
)

type Manifest struct {
	APIVersion   string       `yaml:"apiVersion"`
	Name         string       `yaml:"name"`
	Version      string       `yaml:"version"`
	Tier         string       `yaml:"tier"`
	Description  string       `yaml:"description"`
	License      string       `yaml:"license"`
	Vandor       string       `yaml:"vandor"`
	Kind         string       `yaml:"kind"`
	Capabilities []string     `yaml:"capabilities"`
	Targets      []Target     `yaml:"targets"`
	Dependencies []Dependency `yaml:"dependencies"`
	Actions      []Action     `yaml:"actions"`
	Aliases      []Alias      `yaml:"aliases"`
	Exports      Exports      `yaml:"exports"`
	Permissions  Permissions  `yaml:"permissions"`
}

type Target struct {
	From     string `yaml:"from"`
	To       string `yaml:"to"`
	Mode     string `yaml:"mode"`
	Conflict string `yaml:"conflict"`
}

type Dependency struct {
	Source string `yaml:"source"`
}

type Action struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

type Alias struct {
	Name   string `yaml:"name"`
	Action string `yaml:"action"`
}

type Exports struct {
	Modules []string `yaml:"modules"`
	Helpers []string `yaml:"helpers"`
}

type Permissions struct {
	Write   []string `yaml:"write"`
	Exec    bool     `yaml:"exec"`
	Network bool     `yaml:"network"`
}

type State struct {
	Registries []RegistryRef `yaml:"registries"`
	Packages   []StatePkgRef `yaml:"packages"`
}

type RegistryRef struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
}

type StatePkgRef struct {
	Source string `yaml:"source"`
}

type Lock struct {
	LockVersion int             `yaml:"lockVersion"`
	Packages    []LockedPackage `yaml:"packages"`
}

type LockedPackage struct {
	Name      string         `yaml:"name"`
	Tier      string         `yaml:"tier"`
	Kind      string         `yaml:"kind"`
	Version   string         `yaml:"version"`
	Source    string         `yaml:"source"`
	Resolved  ResolvedSource `yaml:"resolved"`
	Integrity Integrity      `yaml:"integrity"`
	Ownership Ownership      `yaml:"ownership"`
	Actions   []Action       `yaml:"actions"`
	Aliases   []Alias        `yaml:"aliases"`
}

type ResolvedSource struct {
	Kind      string `yaml:"kind"`
	LocalPath string `yaml:"localPath,omitempty"`
	Repo      string `yaml:"repo,omitempty"`
	Ref       string `yaml:"ref,omitempty"`
	Commit    string `yaml:"commit,omitempty"`
	CachePath string `yaml:"cachePath"`
}

type Integrity struct {
	ManifestSHA256 string `yaml:"manifestSha256"`
	ContentSHA256  string `yaml:"contentSha256"`
}

type Ownership struct {
	Files   []OwnedFile `yaml:"files"`
	Aliases []string    `yaml:"aliases"`
}

type OwnedFile struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}

type SearchPackage struct {
	Name     string `json:"name"`
	Tier     string `json:"tier"`
	Latest   string `json:"latest"`
	Registry string `json:"registry"`
	Metadata string `json:"metadata,omitempty"`
}

type ValidationErrors struct {
	Issues []string
}

func (e *ValidationErrors) Addf(format string, args ...any) {
	e.Issues = append(e.Issues, fmt.Sprintf(format, args...))
}

func (e *ValidationErrors) HasIssues() bool {
	return len(e.Issues) > 0
}

func (e *ValidationErrors) Error() string {
	if len(e.Issues) == 0 {
		return "validation failed"
	}
	copied := append([]string(nil), e.Issues...)
	sort.Strings(copied)
	return "validation failed: " + strings.Join(copied, "; ")
}
