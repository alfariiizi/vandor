package vpkg

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

const manifestFileName = "vpkg.manifest.yaml"

var packageNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*/[a-z0-9][a-z0-9-_]*$`)

func (a *Alias) UnmarshalYAML(node *yaml.Node) error {
	type aliasObj Alias
	if node.Kind == yaml.ScalarNode {
		parts := strings.SplitN(strings.TrimSpace(node.Value), "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid alias scalar format %q, expected <name>=<action>", node.Value)
		}
		a.Name = strings.TrimSpace(parts[0])
		a.Action = strings.TrimSpace(parts[1])
		return nil
	}
	var parsed aliasObj
	if err := node.Decode(&parsed); err != nil {
		return err
	}
	*a = Alias(parsed)
	return nil
}

func LoadManifestFromDir(packageRoot string) (Manifest, []byte, error) {
	path := filepath.Join(packageRoot, manifestFileName)
	// nosemgrep: go-path-traversal -- packageRoot is resolved from local/cache/git sources managed by vpkg resolver.
	raw, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, nil, err
	}
	var manifest Manifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return Manifest{}, nil, fmt.Errorf("parse manifest: %w", err)
	}
	manifest.applyDefaults()
	if err := manifest.Validate(); err != nil {
		return Manifest{}, nil, err
	}
	return manifest, raw, nil
}

func (m *Manifest) applyDefaults() {
	if m.APIVersion == "" {
		m.APIVersion = APIVersionV1
	}
	if m.Tier == "" {
		m.Tier = TierCommunity
	}
	if m.Kind == "" {
		m.Kind = KindRuntime
	}
	for i := range m.Targets {
		if m.Targets[i].Mode == "" {
			m.Targets[i].Mode = TargetModeMerge
		}
		if m.Targets[i].Conflict == "" {
			m.Targets[i].Conflict = ConflictError
		}
	}
}

func (m Manifest) Validate() error {
	errs := &ValidationErrors{}
	if m.APIVersion != APIVersionV1 {
		errs.Addf("apiVersion must be %q", APIVersionV1)
	}
	if !packageNamePattern.MatchString(m.Name) {
		errs.Addf("name must be <namespace>/<name> with lowercase slug, got %q", m.Name)
	}
	if strings.TrimSpace(m.Version) == "" {
		errs.Addf("version is required")
	}
	if !slices.Contains([]string{TierOfficial, TierVerified, TierCommunity}, m.Tier) {
		errs.Addf("tier must be one of official|verified|community")
	}
	if !slices.Contains([]string{KindRuntime, KindHelper, KindTooling}, m.Kind) {
		errs.Addf("kind must be one of runtime|helper|tooling")
	}
	actionSet := map[string]struct{}{}
	for i, action := range m.Actions {
		if strings.TrimSpace(action.Name) == "" {
			errs.Addf("actions[%d].name is required", i)
		}
		if strings.TrimSpace(action.Run) == "" {
			errs.Addf("actions[%d].run is required", i)
		}
		if _, exists := actionSet[action.Name]; exists {
			errs.Addf("duplicate action %q", action.Name)
		}
		actionSet[action.Name] = struct{}{}
	}
	aliasSet := map[string]struct{}{}
	for i, alias := range m.Aliases {
		if strings.TrimSpace(alias.Name) == "" || strings.TrimSpace(alias.Action) == "" {
			errs.Addf("aliases[%d] requires both name and action", i)
			continue
		}
		if _, exists := aliasSet[alias.Name]; exists {
			errs.Addf("duplicate alias %q", alias.Name)
		}
		aliasSet[alias.Name] = struct{}{}
	}
	for i, dep := range m.Dependencies {
		if strings.TrimSpace(dep.Source) == "" {
			errs.Addf("dependencies[%d].source is required", i)
		}
	}
	for i, target := range m.Targets {
		if strings.TrimSpace(target.From) == "" {
			errs.Addf("targets[%d].from is required", i)
		}
		if strings.TrimSpace(target.To) == "" {
			errs.Addf("targets[%d].to is required", i)
		}
		if !slices.Contains([]string{TargetModeCopy, TargetModeMerge, TargetModeTemplate}, target.Mode) {
			errs.Addf("targets[%d].mode must be copy|merge|template", i)
		}
		if !slices.Contains([]string{ConflictError, ConflictSkip, ConflictOverwrite, ConflictBackup}, target.Conflict) {
			errs.Addf("targets[%d].conflict must be error|skip|overwrite|backup", i)
		}
	}
	if errs.HasIssues() {
		return errs
	}
	return nil
}

func sha256Hex(input []byte) string {
	sum := sha256.Sum256(input)
	return hex.EncodeToString(sum[:])
}
