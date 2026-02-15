package vpkg

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 4 * time.Second
	defaultHTTPRetries = 1
)

type Manager struct {
	ProjectRoot string
	HTTPTimeout time.Duration
	HTTPRetries int
}

type AddOptions struct {
	PlanOnly bool
}

type AddPlanItem struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Exists bool   `json:"exists"`
	Action string `json:"action"`
}

type AddResult struct {
	Name                  string        `json:"name"`
	Version               string        `json:"version"`
	Tier                  string        `json:"tier"`
	Kind                  string        `json:"kind"`
	Source                string        `json:"source"`
	Files                 int           `json:"files"`
	CachePath             string        `json:"cache_path"`
	PlanOnly              bool          `json:"plan_only"`
	Plan                  []AddPlanItem `json:"plan"`
	InstalledDependencies []string      `json:"installed_dependencies,omitempty"`
}

type RemoveOptions struct {
	Force bool
}

type RemoveResult struct {
	Name         string `json:"name"`
	RemovedFiles int    `json:"removed_files"`
	DriftedFiles int    `json:"drifted_files"`
}

type RemovePreview struct {
	Name              string   `json:"name"`
	DependentPackages []string `json:"dependent_packages,omitempty"`
	OwnedFiles        int      `json:"owned_files"`
	SampleFiles       []string `json:"sample_files,omitempty"`
}

type PackageInfo struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Tier         string   `json:"tier"`
	Kind         string   `json:"kind"`
	Source       string   `json:"source"`
	Installed    bool     `json:"installed"`
	Description  string   `json:"description,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Actions      []Action `json:"actions,omitempty"`
	Aliases      []Alias  `json:"aliases,omitempty"`
	ReadmePath   string   `json:"readme_path,omitempty"`
}

type SyncResult struct {
	Packages int `json:"packages"`
	Files    int `json:"files"`
}

type DoctorReport struct {
	Healthy bool          `json:"healthy"`
	Issues  []DoctorIssue `json:"issues"`
}

type DoctorIssue struct {
	Severity string `json:"severity"`
	Package  string `json:"package"`
	Check    string `json:"check"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type ExecResult struct {
	Package  string `json:"package"`
	Action   string `json:"action"`
	ExitCode int    `json:"exit_code"`
}

type resolvedSource struct {
	Raw              string
	Kind             string
	LocalPath        string
	RequestedVersion string
	Repo             string
	Ref              string
	Commit           string
	CleanupPath      string
}

func NewManager(projectRoot string) *Manager {
	timeout := parseDurationEnv("VANDOR_VPKG_HTTP_TIMEOUT", defaultHTTPTimeout)
	retries := parseIntEnv("VANDOR_VPKG_HTTP_RETRIES", defaultHTTPRetries)
	if retries < 0 {
		retries = 0
	}
	return &Manager{
		ProjectRoot: projectRoot,
		HTTPTimeout: timeout,
		HTTPRetries: retries,
	}
}

func (m *Manager) Add(source string, opts AddOptions) (AddResult, error) {
	return m.addWithStack(source, opts, map[string]bool{})
}

//nolint:gocyclo // install flow intentionally handles resolution, dependencies, and lock-state updates in one transaction-like path.
func (m *Manager) addWithStack(source string, opts AddOptions, stack map[string]bool) (AddResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return AddResult{}, fmt.Errorf("source is required")
	}
	if stack[source] {
		return AddResult{}, fmt.Errorf("dependency cycle detected at %q", source)
	}
	stack[source] = true
	defer delete(stack, source)

	if _, err := EnsureStateDirs(m.ProjectRoot); err != nil {
		return AddResult{}, err
	}
	state, err := LoadState(m.ProjectRoot)
	if err != nil {
		return AddResult{}, err
	}
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return AddResult{}, err
	}
	resolved, err := m.resolveSource(source, state)
	if err != nil {
		return AddResult{}, err
	}
	if resolved.CleanupPath != "" {
		defer func() {
			_ = os.RemoveAll(resolved.CleanupPath)
		}()
	}
	manifest, manifestRaw, err := LoadManifestFromDir(resolved.LocalPath)
	if err != nil {
		return AddResult{}, err
	}
	if resolved.RequestedVersion != "" && resolved.RequestedVersion != manifest.Version {
		return AddResult{}, fmt.Errorf("requested version %q does not match manifest version %q", resolved.RequestedVersion, manifest.Version)
	}
	if err := m.validateAliasConflicts(lock, manifest); err != nil {
		return AddResult{}, err
	}
	installedDeps := make([]string, 0, len(manifest.Dependencies))
	dependencyPlan := make([]AddPlanItem, 0, len(manifest.Dependencies))
	if len(manifest.Dependencies) > 0 {
		for _, dep := range manifest.Dependencies {
			if dependencyInstalled(lock, dep.Source) {
				continue
			}
			depResult, err := m.addWithStack(dep.Source, AddOptions{PlanOnly: opts.PlanOnly}, stack)
			if err != nil {
				return AddResult{}, fmt.Errorf("install dependency %q: %w", dep.Source, err)
			}
			installedDeps = append(installedDeps, depResult.Name+"@"+depResult.Version)
			dependencyPlan = append(dependencyPlan, depResult.Plan...)
			if !opts.PlanOnly {
				lock, err = LoadLock(m.ProjectRoot)
				if err != nil {
					return AddResult{}, err
				}
				state, err = LoadState(m.ProjectRoot)
				if err != nil {
					return AddResult{}, err
				}
			}
		}
		if !opts.PlanOnly {
			if err := validateDependenciesInstalled(lock, manifest.Dependencies); err != nil {
				return AddResult{}, err
			}
		}
	}
	plan, err := m.buildAddPlan(resolved.LocalPath, manifest, false)
	if err != nil {
		return AddResult{}, err
	}
	result := AddResult{
		Name:                  manifest.Name,
		Version:               manifest.Version,
		Tier:                  manifest.Tier,
		Kind:                  manifest.Kind,
		Source:                source,
		PlanOnly:              opts.PlanOnly,
		Plan:                  append(dependencyPlan, plan...),
		InstalledDependencies: installedDeps,
	}
	if opts.PlanOnly {
		return result, nil
	}

	cacheAbs := m.cachePathFor(manifest)
	if old, ok := findLockedPackage(lock, manifest.Name); ok {
		if _, _, err := m.uninstallLockedPackage(old, true); err != nil {
			return AddResult{}, err
		}
		removeStatePackage(&state, old.Source)
	}
	_ = os.RemoveAll(cacheAbs)
	if err := copyDir(resolved.LocalPath, cacheAbs); err != nil {
		return AddResult{}, err
	}

	owned, err := m.applyTargets(cacheAbs, manifest, false)
	if err != nil {
		return AddResult{}, err
	}
	contentHash, err := treeSHA256(cacheAbs)
	if err != nil {
		return AddResult{}, err
	}
	cacheRel, err := makeRelative(m.ProjectRoot, cacheAbs)
	if err != nil {
		cacheRel = filepath.ToSlash(cacheAbs)
	}
	pkg := LockedPackage{
		Name:    manifest.Name,
		Tier:    manifest.Tier,
		Kind:    manifest.Kind,
		Version: manifest.Version,
		Source:  source,
		Resolved: ResolvedSource{
			Kind:      resolved.Kind,
			LocalPath: resolved.LocalPath,
			Repo:      resolved.Repo,
			Ref:       resolved.Ref,
			Commit:    resolved.Commit,
			CachePath: cacheRel,
		},
		Integrity: Integrity{
			ManifestSHA256: sha256Hex(manifestRaw),
			ContentSHA256:  contentHash,
		},
		Ownership: Ownership{
			Files:   owned,
			Aliases: aliasNames(manifest.Aliases),
		},
		Actions: append([]Action(nil), manifest.Actions...),
		Aliases: append([]Alias(nil), manifest.Aliases...),
	}
	upsertLockedPackage(&lock, pkg)
	upsertStatePackage(&state, source)

	if err := SaveLock(m.ProjectRoot, lock); err != nil {
		return AddResult{}, err
	}
	if err := SaveState(m.ProjectRoot, state); err != nil {
		return AddResult{}, err
	}
	result.Files = len(owned)
	result.CachePath = cacheRel
	return result, nil
}

func (m *Manager) Remove(nameOrSource string, opts RemoveOptions) (RemoveResult, error) {
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return RemoveResult{}, err
	}
	state, err := LoadState(m.ProjectRoot)
	if err != nil {
		return RemoveResult{}, err
	}

	pkg, ok := findLockedPackageByNameOrSource(lock, nameOrSource)
	if !ok {
		return RemoveResult{}, fmt.Errorf("package %q is not installed", nameOrSource)
	}
	if !opts.Force {
		dependents, depErr := m.findDependentPackages(lock, pkg)
		if depErr != nil {
			return RemoveResult{}, depErr
		}
		if len(dependents) > 0 {
			return RemoveResult{}, fmt.Errorf("package %q is required by: %s (use --force to remove anyway)", pkg.Name, strings.Join(dependents, ", "))
		}
	}
	removed, drifted, err := m.uninstallLockedPackage(pkg, opts.Force)
	if err != nil {
		return RemoveResult{}, err
	}
	_ = removeLockedPackage(&lock, pkg.Name)
	removeStatePackage(&state, pkg.Source)

	if err := SaveLock(m.ProjectRoot, lock); err != nil {
		return RemoveResult{}, err
	}
	if err := SaveState(m.ProjectRoot, state); err != nil {
		return RemoveResult{}, err
	}
	return RemoveResult{
		Name:         pkg.Name,
		RemovedFiles: removed,
		DriftedFiles: drifted,
	}, nil
}

func (m *Manager) PreviewRemove(nameOrSource string) (RemovePreview, error) {
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return RemovePreview{}, err
	}
	pkg, ok := findLockedPackageByNameOrSource(lock, nameOrSource)
	if !ok {
		return RemovePreview{}, fmt.Errorf("package %q is not installed", nameOrSource)
	}
	dependents, err := m.findDependentPackages(lock, pkg)
	if err != nil {
		return RemovePreview{}, err
	}
	sampleLimit := 5
	sample := make([]string, 0, sampleLimit)
	for _, owned := range pkg.Ownership.Files {
		sample = append(sample, owned.Path)
		if len(sample) == sampleLimit {
			break
		}
	}
	return RemovePreview{
		Name:              pkg.Name,
		DependentPackages: dependents,
		OwnedFiles:        len(pkg.Ownership.Files),
		SampleFiles:       sample,
	}, nil
}

func (m *Manager) Info(nameOrSource string) (PackageInfo, error) {
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return PackageInfo{}, err
	}
	if pkg, ok := findLockedPackageByNameOrSource(lock, nameOrSource); ok {
		cacheAbs := m.resolveCachePath(pkg.Resolved.CachePath)
		manifest, _, err := LoadManifestFromDir(cacheAbs)
		if err != nil {
			return PackageInfo{}, fmt.Errorf("load installed package manifest: %w", err)
		}
		return m.packageInfoFromManifest(manifest, pkg.Source, true, cacheAbs), nil
	}

	state, err := LoadState(m.ProjectRoot)
	if err != nil {
		return PackageInfo{}, err
	}
	resolved, err := m.resolveSource(nameOrSource, state)
	if err != nil {
		return PackageInfo{}, err
	}
	if resolved.CleanupPath != "" {
		defer func() {
			_ = os.RemoveAll(resolved.CleanupPath)
		}()
	}
	manifest, _, err := LoadManifestFromDir(resolved.LocalPath)
	if err != nil {
		return PackageInfo{}, err
	}
	return m.packageInfoFromManifest(manifest, nameOrSource, false, resolved.LocalPath), nil
}

func (m *Manager) packageInfoFromManifest(manifest Manifest, source string, installed bool, packageRoot string) PackageInfo {
	info := PackageInfo{
		Name:         manifest.Name,
		Version:      manifest.Version,
		Tier:         manifest.Tier,
		Kind:         manifest.Kind,
		Source:       source,
		Installed:    installed,
		Description:  strings.TrimSpace(manifest.Description),
		Capabilities: append([]string(nil), manifest.Capabilities...),
		Actions:      append([]Action(nil), manifest.Actions...),
		Aliases:      append([]Alias(nil), manifest.Aliases...),
	}
	readmePath := filepath.Join(packageRoot, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		if rel, relErr := makeRelative(m.ProjectRoot, readmePath); relErr == nil && !strings.HasPrefix(rel, "../") {
			info.ReadmePath = rel
		} else {
			info.ReadmePath = filepath.ToSlash(readmePath)
		}
	}
	return info
}

func (m *Manager) findDependentPackages(lock Lock, target LockedPackage) ([]string, error) {
	dependents := make([]string, 0)
	for _, candidate := range lock.Packages {
		if candidate.Name == target.Name {
			continue
		}
		cacheAbs := m.resolveCachePath(candidate.Resolved.CachePath)
		manifest, _, err := LoadManifestFromDir(cacheAbs)
		if err != nil {
			return nil, fmt.Errorf("cannot inspect dependencies for package %q: %w", candidate.Name, err)
		}
		for _, dep := range manifest.Dependencies {
			if dependencyReferencesPackage(dep.Source, target) {
				dependents = append(dependents, candidate.Name)
				break
			}
		}
	}
	sort.Strings(dependents)
	return dependents, nil
}

func (m *Manager) List() ([]LockedPackage, error) {
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return nil, err
	}
	sort.Slice(lock.Packages, func(i, j int) bool {
		return lock.Packages[i].Name < lock.Packages[j].Name
	})
	return lock.Packages, nil
}

func (m *Manager) RegistryList() ([]RegistryRef, error) {
	state, err := LoadState(m.ProjectRoot)
	if err != nil {
		return nil, err
	}
	sort.Slice(state.Registries, func(i, j int) bool {
		return state.Registries[i].Name < state.Registries[j].Name
	})
	return state.Registries, nil
}

func (m *Manager) RegistryAdd(name, url string) (RegistryRef, error) {
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if name == "" {
		return RegistryRef{}, fmt.Errorf("registry name is required")
	}
	if url == "" {
		return RegistryRef{}, fmt.Errorf("registry url is required")
	}
	state, err := LoadState(m.ProjectRoot)
	if err != nil {
		return RegistryRef{}, err
	}
	entry := RegistryRef{Name: name, URL: url}
	replaced := false
	for i := range state.Registries {
		if state.Registries[i].Name == name {
			state.Registries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		state.Registries = append(state.Registries, entry)
	}
	if err := SaveState(m.ProjectRoot, state); err != nil {
		return RegistryRef{}, err
	}
	return entry, nil
}

func (m *Manager) RegistryRemove(name string) (RegistryRef, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return RegistryRef{}, fmt.Errorf("registry name is required")
	}
	state, err := LoadState(m.ProjectRoot)
	if err != nil {
		return RegistryRef{}, err
	}
	removed := RegistryRef{}
	found := false
	filtered := state.Registries[:0]
	for _, item := range state.Registries {
		if item.Name == name {
			found = true
			removed = item
			continue
		}
		filtered = append(filtered, item)
	}
	if !found {
		return RegistryRef{}, fmt.Errorf("registry %q not found", name)
	}
	state.Registries = filtered
	if err := SaveState(m.ProjectRoot, state); err != nil {
		return RegistryRef{}, err
	}
	return removed, nil
}

func (m *Manager) Search(query, tier, registry string) ([]SearchPackage, error) {
	state, err := LoadState(m.ProjectRoot)
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	tier = strings.ToLower(strings.TrimSpace(tier))
	registry = strings.TrimSpace(registry)
	if tier != "" && tier != TierOfficial && tier != TierVerified && tier != TierCommunity {
		return nil, fmt.Errorf("invalid tier %q (allowed: official|verified|community)", tier)
	}
	if registry != "" {
		if _, ok := findRegistry(state, registry); !ok {
			return nil, fmt.Errorf("registry %q not found", registry)
		}
	}

	registries := append([]RegistryRef(nil), state.Registries...)
	sort.SliceStable(registries, func(i, j int) bool {
		left := tierRank(registries[i].Name)
		right := tierRank(registries[j].Name)
		if left == right {
			return registries[i].Name < registries[j].Name
		}
		return left < right
	})

	found := make(map[string]SearchPackage)
	var failed int
	for _, reg := range registries {
		if registry != "" && reg.Name != registry {
			continue
		}
		index, idxErr := m.loadRegistryIndex(reg)
		if idxErr != nil {
			failed++
			continue
		}
		for _, item := range index.Packages {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			itemTier := strings.ToLower(strings.TrimSpace(item.Tier))
			if itemTier == "" {
				itemTier = registryTierFromName(name)
			}
			if itemTier == "" {
				itemTier = strings.ToLower(strings.TrimSpace(reg.Name))
			}
			if tier != "" && itemTier != tier {
				continue
			}
			if query != "" {
				nameMatch := strings.Contains(strings.ToLower(name), query)
				tierMatch := strings.Contains(strings.ToLower(itemTier), query)
				if !nameMatch && !tierMatch {
					continue
				}
			}
			next := SearchPackage{
				Name:     name,
				Tier:     itemTier,
				Latest:   strings.TrimSpace(item.Latest),
				Registry: reg.Name,
				Metadata: strings.TrimSpace(item.Metadata),
			}
			prev, exists := found[name]
			if !exists || tierRank(next.Tier) < tierRank(prev.Tier) {
				found[name] = next
			}
		}
	}

	if len(found) == 0 {
		if len(registries) > 0 && failed == len(registries) {
			return nil, fmt.Errorf("failed to load package index from all configured registries")
		}
		return []SearchPackage{}, nil
	}

	results := make([]SearchPackage, 0, len(found))
	for _, item := range found {
		results = append(results, item)
	}
	sort.Slice(results, func(i, j int) bool {
		left := tierRank(results[i].Tier)
		right := tierRank(results[j].Tier)
		if left != right {
			return left < right
		}
		return results[i].Name < results[j].Name
	})
	return results, nil
}

func (m *Manager) Sync() (SyncResult, error) {
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return SyncResult{}, err
	}
	var totalFiles int
	for i := range lock.Packages {
		pkg := &lock.Packages[i]
		cacheAbs := m.resolveCachePath(pkg.Resolved.CachePath)
		manifest, manifestRaw, err := LoadManifestFromDir(cacheAbs)
		if err != nil {
			return SyncResult{}, fmt.Errorf("sync %s: %w", pkg.Name, err)
		}
		owned, err := m.applyTargets(cacheAbs, manifest, true)
		if err != nil {
			return SyncResult{}, fmt.Errorf("sync %s: %w", pkg.Name, err)
		}
		contentHash, err := treeSHA256(cacheAbs)
		if err != nil {
			return SyncResult{}, err
		}
		pkg.Integrity.ManifestSHA256 = sha256Hex(manifestRaw)
		pkg.Integrity.ContentSHA256 = contentHash
		pkg.Ownership.Files = owned
		pkg.Actions = append([]Action(nil), manifest.Actions...)
		pkg.Aliases = append([]Alias(nil), manifest.Aliases...)
		pkg.Ownership.Aliases = aliasNames(manifest.Aliases)
		totalFiles += len(owned)
	}
	if err := SaveLock(m.ProjectRoot, lock); err != nil {
		return SyncResult{}, err
	}
	return SyncResult{Packages: len(lock.Packages), Files: totalFiles}, nil
}

func (m *Manager) loadRegistryIndex(reg RegistryRef) (registryIndex, error) {
	if basePath, ok := registryLocalPath(m.ProjectRoot, reg.URL); ok {
		raw, err := os.ReadFile(filepath.Join(basePath, "index.json"))
		if err != nil {
			return registryIndex{}, err
		}
		return parseRegistryIndex(raw)
	}

	endpoint := strings.TrimRight(reg.URL, "/") + "/index.json"
	raw, err := m.fetchURL(endpoint)
	if err != nil {
		return registryIndex{}, err
	}
	return parseRegistryIndex(raw)
}

//nolint:gocyclo // doctor intentionally aggregates many independent validations into one report pass.
func (m *Manager) Doctor() (DoctorReport, error) {
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return DoctorReport{}, err
	}
	report := DoctorReport{Healthy: true}
	aliasOwners := map[string]string{}
	for _, pkg := range lock.Packages {
		cacheAbs := m.resolveCachePath(pkg.Resolved.CachePath)
		manifest, manifestRaw, err := LoadManifestFromDir(cacheAbs)
		if err != nil {
			report.add("error", pkg.Name, "cache", "CACHE_MANIFEST_INVALID", fmt.Sprintf("invalid cache/manifest: %v", err))
			continue
		}
		if manifest.Name != pkg.Name {
			report.add("error", pkg.Name, "manifest", "MANIFEST_NAME_MISMATCH", "manifest name mismatch with lock")
		}
		if manifest.Version != pkg.Version {
			report.add("error", pkg.Name, "manifest", "MANIFEST_VERSION_MISMATCH", "manifest version mismatch with lock")
		}
		if len(manifest.Actions) > 0 && !manifest.Permissions.Exec {
			report.add("warn", pkg.Name, "action", "ACTION_EXEC_DISABLED", "manifest declares actions but permissions.exec=false")
		}
		for _, dep := range manifest.Dependencies {
			depSource := strings.TrimSpace(dep.Source)
			if depSource == "" {
				report.add("error", pkg.Name, "dependency", "DEP_EMPTY_SOURCE", "dependency has empty source")
				continue
			}
			if depSource == pkg.Name || depSource == pkg.Source {
				report.add("error", pkg.Name, "dependency", "DEP_SELF_REFERENCE", fmt.Sprintf("self dependency is not allowed: %s", depSource))
				continue
			}
			if !dependencyInstalled(lock, depSource) {
				report.add("error", pkg.Name, "dependency", "DEP_MISSING", fmt.Sprintf("missing dependency: %s", depSource))
			}
		}
		for _, issue := range compareActionContract(pkg.Actions, manifest.Actions) {
			report.add("error", pkg.Name, "action", "ACTION_CONTRACT_MISMATCH", issue)
		}
		for _, issue := range compareAliasContract(pkg.Aliases, manifest.Aliases) {
			report.add("error", pkg.Name, "alias", "ALIAS_CONTRACT_MISMATCH", issue)
		}
		manifestActionSet := actionNameSet(manifest.Actions)
		for _, alias := range manifest.Aliases {
			if isReservedAlias(alias.Name) {
				report.add("error", pkg.Name, "alias", "ALIAS_RESERVED", fmt.Sprintf("manifest declares reserved alias: %s", alias.Name))
			}
			if strings.Contains(alias.Action, ":") {
				continue
			}
			if _, ok := manifestActionSet[alias.Action]; !ok {
				report.add("error", pkg.Name, "alias", "ALIAS_UNKNOWN_ACTION", fmt.Sprintf("manifest alias %q points to unknown action %q", alias.Name, alias.Action))
			}
		}
		if !equalStringSlices(sortedCopy(pkg.Ownership.Aliases), aliasNames(manifest.Aliases)) {
			report.add("warn", pkg.Name, "ownership", "OWNERSHIP_ALIAS_MISMATCH", "owned alias list mismatch with manifest aliases")
		}
		if sha256Hex(manifestRaw) != pkg.Integrity.ManifestSHA256 {
			report.add("error", pkg.Name, "integrity", "INTEGRITY_MANIFEST_HASH_MISMATCH", "manifest hash mismatch")
		}
		contentHash, err := treeSHA256(cacheAbs)
		if err != nil {
			report.add("error", pkg.Name, "integrity", "INTEGRITY_CONTENT_HASH_ERROR", fmt.Sprintf("cannot hash cache: %v", err))
		} else if contentHash != pkg.Integrity.ContentSHA256 {
			report.add("error", pkg.Name, "integrity", "INTEGRITY_CONTENT_HASH_MISMATCH", "content hash mismatch")
		}
		for _, owned := range pkg.Ownership.Files {
			abs := filepath.Join(m.ProjectRoot, filepath.FromSlash(owned.Path))
			sum, err := fileSHA256(abs)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					report.add("warn", pkg.Name, "ownership", "OWNERSHIP_FILE_MISSING", fmt.Sprintf("missing file: %s", owned.Path))
					continue
				}
				report.add("error", pkg.Name, "ownership", "OWNERSHIP_FILE_READ_ERROR", fmt.Sprintf("cannot read %s: %v", owned.Path, err))
				continue
			}
			if sum != owned.SHA256 {
				report.add("warn", pkg.Name, "ownership", "OWNERSHIP_FILE_HASH_MISMATCH", fmt.Sprintf("drifted file: %s", owned.Path))
			}
		}
		for _, alias := range pkg.Aliases {
			if isReservedAlias(alias.Name) {
				report.add("error", pkg.Name, "alias", "ALIAS_RESERVED", fmt.Sprintf("reserved alias: %s", alias.Name))
			}
			if owner, exists := aliasOwners[alias.Name]; exists && owner != pkg.Name {
				report.add("error", pkg.Name, "alias", "ALIAS_COLLISION", fmt.Sprintf("alias collision with %s: %s", owner, alias.Name))
			}
			aliasOwners[alias.Name] = pkg.Name
		}
	}
	sort.Slice(report.Issues, func(i, j int) bool {
		left := report.Issues[i]
		right := report.Issues[j]
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.Severity != right.Severity {
			return left.Severity < right.Severity
		}
		if left.Check != right.Check {
			return left.Check < right.Check
		}
		return left.Message < right.Message
	})
	return report, nil
}

func (m *Manager) Exec(packageName, actionName string, args []string, stdout, stderr io.Writer) (ExecResult, error) {
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return ExecResult{}, err
	}
	pkg, ok := findLockedPackage(lock, packageName)
	if !ok {
		return ExecResult{}, fmt.Errorf("package %q is not installed", packageName)
	}
	action, ok := findAction(pkg.Actions, actionName)
	if !ok {
		return ExecResult{}, fmt.Errorf("action %q not found in package %s", actionName, packageName)
	}
	cacheAbs := m.resolveCachePath(pkg.Resolved.CachePath)
	manifest, _, err := LoadManifestFromDir(cacheAbs)
	if err != nil {
		return ExecResult{}, err
	}
	if !manifest.Permissions.Exec {
		return ExecResult{}, fmt.Errorf("package %s does not allow exec actions (permissions.exec=false)", packageName)
	}

	// #nosec G204 -- action.Run is an explicit package action contract executed by user intent via `vpkg exec`.
	cmd := exec.Command("sh", append([]string{"-c", action.Run + " \"$@\"", "vandor-vpkg-action"}, args...)...)
	cmd.Dir = cacheAbs
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = append(os.Environ(),
		"VANDOR_PROJECT_ROOT="+m.ProjectRoot,
		"VANDOR_PACKAGE="+pkg.Name,
		"VANDOR_ACTION="+action.Name,
	)
	err = cmd.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
		return ExecResult{Package: pkg.Name, Action: action.Name, ExitCode: exitCode}, err
	}
	return ExecResult{Package: pkg.Name, Action: action.Name, ExitCode: 0}, nil
}

func (m *Manager) ExecAlias(aliasName string, args []string, stdout, stderr io.Writer) (ExecResult, error) {
	lock, err := LoadLock(m.ProjectRoot)
	if err != nil {
		return ExecResult{}, err
	}
	return m.execAlias(lock, aliasName, args, stdout, stderr, map[string]bool{})
}

func (m *Manager) execAlias(lock Lock, aliasName string, args []string, stdout, stderr io.Writer, stack map[string]bool) (ExecResult, error) {
	aliasName = strings.TrimSpace(aliasName)
	if aliasName == "" {
		return ExecResult{}, fmt.Errorf("alias name is required")
	}
	if stack[aliasName] {
		return ExecResult{}, fmt.Errorf("alias cycle detected at %q", aliasName)
	}
	stack[aliasName] = true
	defer delete(stack, aliasName)

	pkgName, actionName, err := resolveAlias(lock, aliasName)
	if err != nil {
		return ExecResult{}, err
	}
	if strings.Contains(actionName, ":") {
		return m.execAlias(lock, actionName, args, stdout, stderr, stack)
	}
	return m.Exec(pkgName, actionName, args, stdout, stderr)
}

func (m *Manager) resolveSource(source string, state State) (resolvedSource, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return resolvedSource{}, fmt.Errorf("source is required")
	}
	if filepath.IsAbs(source) || strings.HasPrefix(source, ".") || strings.HasPrefix(source, "/") || strings.HasPrefix(source, `\`) {
		path := source
		if !filepath.IsAbs(path) {
			path = filepath.Join(m.ProjectRoot, path)
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return resolvedSource{}, err
		}
		return resolvedSource{Raw: source, Kind: "local", LocalPath: abs}, nil
	}
	if strings.HasPrefix(source, "@") {
		return m.resolveAliasSource(source, state)
	}
	if gitSpec, ok := parseGitSourceSpec(source); ok {
		return m.resolveGitSource(source, gitSpec)
	}
	if strings.Contains(source, "github.com/") || strings.Contains(source, "://") {
		return resolvedSource{}, fmt.Errorf("unsupported source %q", source)
	}
	path := filepath.Join(m.ProjectRoot, source)
	if _, err := os.Stat(path); err == nil {
		abs, err := filepath.Abs(path)
		if err != nil {
			return resolvedSource{}, err
		}
		return resolvedSource{Raw: source, Kind: "local", LocalPath: abs}, nil
	}
	aliasResolved, aliasErr := m.resolveAliasSource("@"+source, state)
	if aliasErr == nil {
		return aliasResolved, nil
	}
	return resolvedSource{}, aliasErr
}

func (m *Manager) resolveAliasSource(source string, state State) (resolvedSource, error) {
	body := strings.TrimPrefix(source, "@")
	version := ""
	if at := strings.LastIndex(body, "@"); at > 0 {
		version = body[at+1:]
		body = body[:at]
	}
	parts := strings.SplitN(body, "/", 2)
	registryName := ""
	pkgName := body
	if len(parts) == 2 {
		registryName = parts[0]
		pkgName = parts[1]
	}

	candidates := []string{registryName}
	if registryName == "" {
		candidates = []string{TierOfficial, TierVerified, TierCommunity}
	}
	for _, regName := range candidates {
		reg, ok := findRegistry(state, regName)
		if !ok {
			continue
		}
		if basePath, ok := registryLocalPath(m.ProjectRoot, reg.URL); ok {
			roots := []string{
				filepath.Join(basePath, pkgName),
				filepath.Join(basePath, regName, pkgName),
				filepath.Join(basePath, "packages", pkgName),
				filepath.Join(basePath, "packages", regName, pkgName),
			}
			for _, root := range roots {
				manifest, _, err := LoadManifestFromDir(root)
				if err != nil {
					continue
				}
				if version != "" && version != manifest.Version {
					continue
				}
				abs, err := filepath.Abs(root)
				if err != nil {
					return resolvedSource{}, err
				}
				return resolvedSource{
					Raw:              source,
					Kind:             "registry",
					LocalPath:        abs,
					RequestedVersion: version,
				}, nil
			}
		}
		indexSource, err := m.resolveRegistryIndexSource(reg, regName, pkgName, version)
		if err == nil && strings.TrimSpace(indexSource) != "" {
			resolved, resolveErr := m.resolveSource(indexSource, state)
			if resolveErr != nil {
				return resolvedSource{}, fmt.Errorf("registry index resolved %q but source failed: %w", indexSource, resolveErr)
			}
			resolved.Kind = "registry"
			if resolved.RequestedVersion == "" {
				resolved.RequestedVersion = version
			}
			return resolved, nil
		}
		if err != nil {
			continue
		}
	}
	if registryName == "" {
		return resolvedSource{}, fmt.Errorf("cannot resolve %q in official/verified/community registries", source)
	}
	return resolvedSource{}, fmt.Errorf("cannot resolve %q in registry %q", source, registryName)
}

func (m *Manager) resolveGitSource(raw string, spec gitSourceSpec) (resolvedSource, error) {
	cacheBase := filepath.Join(m.ProjectRoot, ".vandor", "vpkg", "tmp")
	if err := os.MkdirAll(cacheBase, 0o755); err != nil {
		return resolvedSource{}, err
	}
	cloneDir, err := os.MkdirTemp(cacheBase, "git-src-*")
	if err != nil {
		return resolvedSource{}, err
	}
	cleanup := func(e error) (resolvedSource, error) {
		_ = os.RemoveAll(cloneDir)
		return resolvedSource{}, e
	}
	cloneArgs := []string{"clone", "--quiet"}
	if spec.Ref != "" {
		cloneArgs = append(cloneArgs, "--branch", spec.Ref)
	}
	cloneArgs = append(cloneArgs, spec.Repo, cloneDir)
	// #nosec G204 -- git clone arguments are derived from parsed git source syntax and executed intentionally.
	if out, err := exec.Command("git", cloneArgs...).CombinedOutput(); err != nil {
		if spec.Ref != "" {
			// #nosec G204 -- fallback clone with validated git source data.
			if out2, err2 := exec.Command("git", "clone", "--quiet", spec.Repo, cloneDir).CombinedOutput(); err2 != nil {
				return cleanup(fmt.Errorf("git clone failed: %s", strings.TrimSpace(string(out2))))
			}
			// #nosec G204 -- checkout target ref is parsed from explicit source input.
			if out3, err3 := exec.Command("git", "-C", cloneDir, "checkout", "--quiet", spec.Ref).CombinedOutput(); err3 != nil {
				return cleanup(fmt.Errorf("git checkout %q failed: %s", spec.Ref, strings.TrimSpace(string(out3))))
			}
		} else {
			return cleanup(fmt.Errorf("git clone failed: %s", strings.TrimSpace(string(out))))
		}
	}
	// #nosec G204 -- rev-parse reads commit from cloned repository path.
	commitOut, err := exec.Command("git", "-C", cloneDir, "rev-parse", "HEAD").CombinedOutput()
	if err != nil {
		return cleanup(fmt.Errorf("git rev-parse failed: %s", strings.TrimSpace(string(commitOut))))
	}
	commit := strings.TrimSpace(string(commitOut))
	pkgRoot := cloneDir
	if spec.Subdir != "" && spec.Subdir != "." {
		pkgRoot = filepath.Join(cloneDir, filepath.FromSlash(spec.Subdir))
	}
	if stat, err := os.Stat(pkgRoot); err != nil || !stat.IsDir() {
		return cleanup(fmt.Errorf("package subdir %q not found in git source", spec.Subdir))
	}
	return resolvedSource{
		Raw:         raw,
		Kind:        "git",
		LocalPath:   pkgRoot,
		Repo:        spec.Repo,
		Ref:         spec.Ref,
		Commit:      commit,
		CleanupPath: cloneDir,
	}, nil
}

func (m *Manager) resolveRegistryIndexSource(reg RegistryRef, regName, pkgName, version string) (string, error) {
	if reg.URL == "" {
		return "", fmt.Errorf("registry URL empty")
	}
	if basePath, ok := registryLocalPath(m.ProjectRoot, reg.URL); ok {
		candidates := []string{
			filepath.Join(basePath, "packages", regName, pkgName+".json"),
			filepath.Join(basePath, "packages", pkgName+".json"),
		}
		for _, candidate := range candidates {
			raw, err := os.ReadFile(candidate)
			if err != nil {
				continue
			}
			source, err := parseRegistryPackageSource(raw, version)
			if err != nil {
				return "", fmt.Errorf("registry index %s invalid: %w", candidate, err)
			}
			return normalizeRegistrySource(basePath, source), nil
		}
		return "", fmt.Errorf("registry index not found")
	}
	baseURL := strings.TrimRight(reg.URL, "/")
	candidates := []string{
		fmt.Sprintf("%s/packages/%s/%s.json", baseURL, regName, pkgName),
		fmt.Sprintf("%s/packages/%s.json", baseURL, pkgName),
	}
	for _, endpoint := range candidates {
		raw, err := m.fetchURL(endpoint)
		if err != nil {
			continue
		}
		source, parseErr := parseRegistryPackageSource(raw, version)
		if parseErr != nil {
			return "", fmt.Errorf("registry index %s invalid: %w", endpoint, parseErr)
		}
		return normalizeRegistrySourceURL(baseURL, source), nil
	}
	return "", fmt.Errorf("registry index not found")
}

func (m *Manager) fetchURL(url string) ([]byte, error) {
	var lastErr error
	attempts := m.HTTPRetries + 1
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		raw, err := m.fetchURLOnce(url)
		if err == nil {
			return raw, nil
		}
		lastErr = err
		if attempt < attempts-1 {
			time.Sleep(time.Duration(attempt+1) * 150 * time.Millisecond)
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("request failed")
	}
	return nil, lastErr
}

func (m *Manager) fetchURLOnce(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: m.HTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

type registryPackageVersion struct {
	Version string `json:"version"`
	Source  string `json:"source"`
}

type registryIndexItem struct {
	Name     string `json:"name"`
	Tier     string `json:"tier"`
	Latest   string `json:"latest"`
	Metadata string `json:"metadata"`
}

type registryIndex struct {
	Packages []registryIndexItem `json:"packages"`
}

type registryPackageRecord struct {
	Name     string                   `json:"name"`
	Source   string                   `json:"source"`
	Latest   string                   `json:"latest"`
	Versions []registryPackageVersion `json:"versions"`
}

func parseRegistryIndex(raw []byte) (registryIndex, error) {
	var idx registryIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		return registryIndex{}, err
	}
	if idx.Packages == nil {
		idx.Packages = []registryIndexItem{}
	}
	return idx, nil
}

func parseRegistryPackageSource(raw []byte, version string) (string, error) {
	var record registryPackageRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return "", err
	}
	if version != "" {
		for _, item := range record.Versions {
			if item.Version == version && strings.TrimSpace(item.Source) != "" {
				return item.Source, nil
			}
		}
	}
	if version == "" && record.Latest != "" {
		for _, item := range record.Versions {
			if item.Version == record.Latest && strings.TrimSpace(item.Source) != "" {
				return item.Source, nil
			}
		}
	}
	if strings.TrimSpace(record.Source) != "" {
		return record.Source, nil
	}
	if len(record.Versions) > 0 && strings.TrimSpace(record.Versions[len(record.Versions)-1].Source) != "" {
		return record.Versions[len(record.Versions)-1].Source, nil
	}
	return "", fmt.Errorf("no source found in registry package metadata")
}

func tierRank(tier string) int {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case TierOfficial:
		return 0
	case TierVerified:
		return 1
	case TierCommunity:
		return 2
	default:
		return 100
	}
}

func registryTierFromName(name string) string {
	parts := strings.SplitN(strings.TrimSpace(name), "/", 2)
	if len(parts) == 0 {
		return ""
	}
	prefix := strings.ToLower(strings.TrimSpace(parts[0]))
	switch prefix {
	case TierOfficial, TierVerified, TierCommunity:
		return prefix
	default:
		return ""
	}
}

func normalizeRegistrySource(basePath, source string) string {
	if source == "" {
		return source
	}
	if strings.HasPrefix(source, ".") {
		return filepath.Join(basePath, source)
	}
	if filepath.IsAbs(source) {
		return source
	}
	if strings.Contains(source, "://") || strings.HasPrefix(source, "github.com/") || strings.HasPrefix(source, "@") {
		return source
	}
	return filepath.Join(basePath, source)
}

func normalizeRegistrySourceURL(baseURL, source string) string {
	if source == "" {
		return source
	}
	if strings.Contains(source, "://") || strings.HasPrefix(source, "github.com/") || strings.HasPrefix(source, "@") {
		return source
	}
	if gitSource, ok := convertRawGitHubRelativeSource(baseURL, source); ok {
		return gitSource
	}
	if strings.HasPrefix(source, "/") {
		u, err := neturl.Parse(baseURL)
		if err != nil {
			return source
		}
		u.Path = filepath.ToSlash(source)
		return u.String()
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(source, "/")
}

func convertRawGitHubRelativeSource(baseURL, source string) (string, bool) {
	if source == "" || strings.HasPrefix(source, "/") {
		return "", false
	}
	u, err := neturl.Parse(baseURL)
	if err != nil {
		return "", false
	}
	if !strings.EqualFold(u.Host, "raw.githubusercontent.com") {
		return "", false
	}
	segments := strings.Split(strings.Trim(strings.TrimSpace(u.Path), "/"), "/")
	if len(segments) < 3 {
		return "", false
	}
	owner := strings.TrimSpace(segments[0])
	repo := strings.TrimSpace(segments[1])
	ref := strings.TrimSpace(segments[2])
	if owner == "" || repo == "" || ref == "" {
		return "", false
	}
	rel := strings.TrimPrefix(filepath.ToSlash(strings.TrimSpace(source)), "./")
	if rel == "" {
		return "", false
	}
	return fmt.Sprintf("github.com/%s/%s//%s@%s", owner, repo, rel, ref), true
}

type gitSourceSpec struct {
	Repo   string
	Subdir string
	Ref    string
}

func parseGitSourceSpec(source string) (gitSourceSpec, bool) {
	ref := ""
	base := source
	if at := strings.LastIndex(source, "@"); at > 0 {
		base = source[:at]
		ref = source[at+1:]
	}
	repoPart := base
	subdirPart := ""
	schemeIdx := strings.Index(base, "://")
	lastDoubleSlash := strings.LastIndex(base, "//")
	if lastDoubleSlash >= 0 && (schemeIdx < 0 || lastDoubleSlash > schemeIdx+2) {
		idx := lastDoubleSlash
		repoPart = base[:idx]
		subdirPart = strings.TrimPrefix(base[idx+2:], "/")
	}
	if repoPart == "" {
		return gitSourceSpec{}, false
	}
	if !isLikelyGitRepo(repoPart) {
		return gitSourceSpec{}, false
	}
	repo := normalizeRepoURL(repoPart)
	subdirPart = strings.TrimSpace(filepath.ToSlash(subdirPart))
	return gitSourceSpec{
		Repo:   repo,
		Subdir: subdirPart,
		Ref:    ref,
	}, true
}

func isLikelyGitRepo(input string) bool {
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") ||
		strings.HasPrefix(input, "ssh://") || strings.HasPrefix(input, "git@") ||
		strings.HasPrefix(input, "file://") || strings.HasPrefix(input, "github.com/") {
		return true
	}
	if filepath.IsAbs(input) {
		return true
	}
	if strings.Contains(input, "/") {
		if info, err := os.Stat(input); err == nil && info.IsDir() {
			return true
		}
	}
	return false
}

func normalizeRepoURL(input string) string {
	if strings.HasPrefix(input, "github.com/") {
		return "https://" + input
	}
	return input
}

func (m *Manager) buildAddPlan(sourceRoot string, manifest Manifest, forceOverwrite bool) ([]AddPlanItem, error) {
	plan := make([]AddPlanItem, 0, 32)
	for _, target := range manifest.Targets {
		sourceAbs := filepath.Join(sourceRoot, filepath.FromSlash(target.From))
		info, err := os.Stat(sourceAbs)
		if err != nil {
			return nil, fmt.Errorf("target source not found: %s", target.From)
		}
		baseDestAbs := filepath.Join(m.ProjectRoot, filepath.FromSlash(target.To))
		if err := m.ensureAllowedTarget(baseDestAbs, manifest); err != nil {
			return nil, err
		}
		conflictMode := target.Conflict
		if forceOverwrite {
			conflictMode = ConflictOverwrite
		}
		if info.IsDir() {
			err := filepath.WalkDir(sourceAbs, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				relSrc, err := filepath.Rel(sourceRoot, path)
				if err != nil {
					return err
				}
				relPart, err := filepath.Rel(sourceAbs, path)
				if err != nil {
					return err
				}
				destAbs := filepath.Join(baseDestAbs, relPart)
				destRel, err := makeRelative(m.ProjectRoot, destAbs)
				if err != nil {
					return err
				}
				_, existsErr := os.Stat(destAbs)
				exists := existsErr == nil
				action := "create"
				if exists {
					action = conflictMode
				}
				plan = append(plan, AddPlanItem{
					From:   filepath.ToSlash(relSrc),
					To:     destRel,
					Exists: exists,
					Action: action,
				})
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		destAbs := baseDestAbs
		if info, err := os.Stat(destAbs); err == nil && info.IsDir() {
			destAbs = filepath.Join(destAbs, filepath.Base(sourceAbs))
		}
		srcRel, err := makeRelative(sourceRoot, sourceAbs)
		if err != nil {
			return nil, err
		}
		destRel, err := makeRelative(m.ProjectRoot, destAbs)
		if err != nil {
			return nil, err
		}
		_, existsErr := os.Stat(destAbs)
		exists := existsErr == nil
		action := "create"
		if exists {
			action = conflictMode
		}
		plan = append(plan, AddPlanItem{
			From:   srcRel,
			To:     destRel,
			Exists: exists,
			Action: action,
		})
	}
	sort.Slice(plan, func(i, j int) bool {
		if plan[i].To == plan[j].To {
			return plan[i].From < plan[j].From
		}
		return plan[i].To < plan[j].To
	})
	return plan, nil
}

func (m *Manager) applyTargets(cacheAbs string, manifest Manifest, forceOverwrite bool) ([]OwnedFile, error) {
	ownedMap := map[string]OwnedFile{}
	for _, target := range manifest.Targets {
		sourceAbs := filepath.Join(cacheAbs, filepath.FromSlash(target.From))
		info, err := os.Stat(sourceAbs)
		if err != nil {
			return nil, fmt.Errorf("target source not found: %s", target.From)
		}
		baseDestAbs := filepath.Join(m.ProjectRoot, filepath.FromSlash(target.To))
		if err := m.ensureAllowedTarget(baseDestAbs, manifest); err != nil {
			return nil, err
		}
		conflictMode := target.Conflict
		if forceOverwrite {
			conflictMode = ConflictOverwrite
		}
		if info.IsDir() {
			err := filepath.WalkDir(sourceAbs, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if d.IsDir() {
					return nil
				}
				rel, err := filepath.Rel(sourceAbs, path)
				if err != nil {
					return err
				}
				destAbs := filepath.Join(baseDestAbs, rel)
				written, err := copyWithConflict(path, destAbs, conflictMode)
				if err != nil {
					return err
				}
				if written {
					owned, err := newOwnedFile(m.ProjectRoot, destAbs)
					if err != nil {
						return err
					}
					ownedMap[owned.Path] = owned
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		destAbs := baseDestAbs
		if info, err := os.Stat(destAbs); err == nil && info.IsDir() {
			destAbs = filepath.Join(destAbs, filepath.Base(sourceAbs))
		}
		written, err := copyWithConflict(sourceAbs, destAbs, conflictMode)
		if err != nil {
			return nil, err
		}
		if written {
			owned, err := newOwnedFile(m.ProjectRoot, destAbs)
			if err != nil {
				return nil, err
			}
			ownedMap[owned.Path] = owned
		}
	}
	owned := make([]OwnedFile, 0, len(ownedMap))
	for _, file := range ownedMap {
		owned = append(owned, file)
	}
	sort.Slice(owned, func(i, j int) bool {
		return owned[i].Path < owned[j].Path
	})
	return owned, nil
}

func (m *Manager) ensureAllowedTarget(destAbs string, manifest Manifest) error {
	if !isSubpath(m.ProjectRoot, destAbs) {
		return fmt.Errorf("target %s escapes project root", destAbs)
	}
	rel, err := makeRelative(m.ProjectRoot, destAbs)
	if err != nil {
		return err
	}
	blocked := []string{"internal/core/", "cmd/", "database/"}
	for _, prefix := range blocked {
		if strings.HasPrefix(rel, prefix) || rel == strings.TrimSuffix(prefix, "/") {
			return fmt.Errorf("target path %s is blocked by policy", rel)
		}
	}
	defaultAllowed := []string{
		"internal/transport/",
		"internal/infrastructure/",
		"internal/bootstrap/runtime/",
		"config/fragments/",
		"tools/",
	}
	for _, prefix := range defaultAllowed {
		if strings.HasPrefix(rel, prefix) || rel == strings.TrimSuffix(prefix, "/") {
			return nil
		}
	}
	for _, pattern := range manifest.Permissions.Write {
		if matchTargetPattern(rel, pattern) {
			return nil
		}
	}
	return fmt.Errorf("target path %s not allowed by policy", rel)
}

func copyWithConflict(src, dst, conflict string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, err
	}
	if _, err := os.Stat(dst); err == nil {
		switch conflict {
		case ConflictError:
			return false, fmt.Errorf("target file exists: %s", dst)
		case ConflictSkip:
			return false, nil
		case ConflictOverwrite:
		case ConflictBackup:
			backup := fmt.Sprintf("%s.bak.%d", dst, time.Now().UnixNano())
			if err := os.Rename(dst, backup); err != nil {
				return false, err
			}
		default:
			return false, fmt.Errorf("unsupported conflict mode %q", conflict)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := copyFile(src, dst); err != nil {
		return false, err
	}
	return true, nil
}

func newOwnedFile(projectRoot, absolutePath string) (OwnedFile, error) {
	rel, err := makeRelative(projectRoot, absolutePath)
	if err != nil {
		return OwnedFile{}, err
	}
	sum, err := fileSHA256(absolutePath)
	if err != nil {
		return OwnedFile{}, err
	}
	return OwnedFile{Path: rel, SHA256: sum}, nil
}

func (m *Manager) uninstallLockedPackage(pkg LockedPackage, force bool) (removed int, drifted int, err error) {
	if !force {
		for _, owned := range pkg.Ownership.Files {
			abs := filepath.Join(m.ProjectRoot, filepath.FromSlash(owned.Path))
			sum, sumErr := fileSHA256(abs)
			if sumErr != nil {
				if errors.Is(sumErr, os.ErrNotExist) {
					drifted++
					continue
				}
				return 0, drifted, sumErr
			}
			if sum != owned.SHA256 {
				drifted++
			}
		}
		if drifted > 0 {
			return 0, drifted, fmt.Errorf("drift detected for package %s (%d file(s)); retry with --force", pkg.Name, drifted)
		}
	}
	for _, owned := range pkg.Ownership.Files {
		abs := filepath.Join(m.ProjectRoot, filepath.FromSlash(owned.Path))
		if _, statErr := os.Stat(abs); statErr != nil {
			if errors.Is(statErr, os.ErrNotExist) {
				drifted++
				continue
			}
			return removed, drifted, statErr
		}
		if err := os.Remove(abs); err != nil {
			return removed, drifted, err
		}
		removed++
		removeEmptyParents(filepath.Dir(abs), m.ProjectRoot)
	}
	cacheAbs := m.resolveCachePath(pkg.Resolved.CachePath)
	_ = os.RemoveAll(cacheAbs)
	return removed, drifted, nil
}

func (m *Manager) cachePathFor(manifest Manifest) string {
	safe := strings.ReplaceAll(manifest.Name, "/", "__")
	return filepath.Join(m.ProjectRoot, ".vandor", "vpkg", "cache", fmt.Sprintf("%s@%s", safe, manifest.Version))
}

func (m *Manager) resolveCachePath(cachePath string) string {
	if filepath.IsAbs(cachePath) {
		return cachePath
	}
	return filepath.Join(m.ProjectRoot, filepath.FromSlash(cachePath))
}

func (m *Manager) validateAliasConflicts(lock Lock, manifest Manifest) error {
	existing := map[string]string{}
	for _, pkg := range lock.Packages {
		for _, alias := range pkg.Aliases {
			existing[alias.Name] = pkg.Name
		}
	}
	actionSet := map[string]struct{}{}
	for _, action := range manifest.Actions {
		actionSet[action.Name] = struct{}{}
	}
	for _, alias := range manifest.Aliases {
		if isReservedAlias(alias.Name) {
			return fmt.Errorf("alias %q is reserved", alias.Name)
		}
		if owner, ok := existing[alias.Name]; ok && owner != manifest.Name {
			return fmt.Errorf("alias %q already owned by %s", alias.Name, owner)
		}
		if strings.Contains(alias.Action, ":") {
			continue
		}
		if _, ok := actionSet[alias.Action]; !ok {
			return fmt.Errorf("alias %q points to unknown action %q", alias.Name, alias.Action)
		}
	}
	return nil
}

func (r *DoctorReport) add(severity, pkg, check, code, message string) {
	r.Healthy = false
	r.Issues = append(r.Issues, DoctorIssue{
		Severity: severity,
		Package:  pkg,
		Check:    check,
		Code:     code,
		Message:  message,
	})
}

func validateDependenciesInstalled(lock Lock, deps []Dependency) error {
	if len(deps) == 0 {
		return nil
	}
	for _, dep := range deps {
		if dependencyInstalled(lock, dep.Source) {
			continue
		}
		return fmt.Errorf("dependency %q is not installed yet; install it first", dep.Source)
	}
	return nil
}

func dependencyInstalled(lock Lock, source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		return false
	}
	normalizedSource, hasNormalized := normalizeDependencySourceName(source)
	for _, pkg := range lock.Packages {
		if pkg.Name == source || pkg.Source == source {
			return true
		}
		if hasNormalized && pkg.Name == normalizedSource {
			return true
		}
	}
	return false
}

func dependencyReferencesPackage(depSource string, pkg LockedPackage) bool {
	depSource = strings.TrimSpace(depSource)
	if depSource == "" {
		return false
	}
	if depSource == pkg.Source || depSource == pkg.Name {
		return true
	}
	depName, okDep := normalizeDependencySourceName(depSource)
	if okDep && depName == pkg.Name {
		return true
	}
	pkgSourceName, okPkgSource := normalizeDependencySourceName(pkg.Source)
	if okPkgSource && depName == pkgSourceName {
		return true
	}
	return false
}

func normalizeDependencySourceName(source string) (string, bool) {
	body := strings.TrimSpace(source)
	if body == "" {
		return "", false
	}
	if strings.HasPrefix(body, "@") {
		body = strings.TrimPrefix(body, "@")
		if at := strings.LastIndex(body, "@"); at > 0 {
			body = body[:at]
		}
	}
	body = strings.TrimSpace(body)
	if !isTierQualifiedName(body) {
		return "", false
	}
	if at := strings.LastIndex(body, "@"); at > 0 {
		body = body[:at]
	}
	return body, true
}

func isTierQualifiedName(name string) bool {
	parts := strings.SplitN(strings.TrimSpace(name), "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
		return false
	}
	switch strings.TrimSpace(parts[0]) {
	case TierOfficial, TierVerified, TierCommunity:
		return true
	default:
		return false
	}
}

func aliasNames(aliases []Alias) []string {
	names := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		names = append(names, alias.Name)
	}
	sort.Strings(names)
	return names
}

func compareActionContract(locked []Action, manifest []Action) []string {
	issues := make([]string, 0)
	lockMap := map[string]string{}
	manifestMap := map[string]string{}
	for _, action := range manifest {
		name := strings.TrimSpace(action.Name)
		if name == "" {
			continue
		}
		manifestMap[name] = strings.TrimSpace(action.Run)
	}
	for _, action := range locked {
		name := strings.TrimSpace(action.Name)
		if name == "" {
			issues = append(issues, "lock contains action with empty name")
			continue
		}
		if _, exists := lockMap[name]; exists {
			issues = append(issues, fmt.Sprintf("lock contains duplicate action %q", name))
			continue
		}
		lockMap[name] = strings.TrimSpace(action.Run)
	}
	for name, run := range manifestMap {
		lockRun, ok := lockMap[name]
		if !ok {
			issues = append(issues, fmt.Sprintf("lock missing action %q", name))
			continue
		}
		if lockRun != run {
			issues = append(issues, fmt.Sprintf("action %q run mismatch between lock and manifest", name))
		}
	}
	for name := range lockMap {
		if _, ok := manifestMap[name]; !ok {
			issues = append(issues, fmt.Sprintf("lock has stale action %q not present in manifest", name))
		}
	}
	sort.Strings(issues)
	return issues
}

func compareAliasContract(locked []Alias, manifest []Alias) []string {
	issues := make([]string, 0)
	lockMap := map[string]string{}
	manifestMap := map[string]string{}
	for _, alias := range manifest {
		name := strings.TrimSpace(alias.Name)
		if name == "" {
			continue
		}
		manifestMap[name] = strings.TrimSpace(alias.Action)
	}
	for _, alias := range locked {
		name := strings.TrimSpace(alias.Name)
		if name == "" {
			issues = append(issues, "lock contains alias with empty name")
			continue
		}
		if _, exists := lockMap[name]; exists {
			issues = append(issues, fmt.Sprintf("lock contains duplicate alias %q", name))
			continue
		}
		lockMap[name] = strings.TrimSpace(alias.Action)
	}
	for name, action := range manifestMap {
		lockAction, ok := lockMap[name]
		if !ok {
			issues = append(issues, fmt.Sprintf("lock missing alias %q", name))
			continue
		}
		if lockAction != action {
			issues = append(issues, fmt.Sprintf("alias %q action mismatch between lock and manifest", name))
		}
	}
	for name := range lockMap {
		if _, ok := manifestMap[name]; !ok {
			issues = append(issues, fmt.Sprintf("lock has stale alias %q not present in manifest", name))
		}
	}
	sort.Strings(issues)
	return issues
}

func actionNameSet(actions []Action) map[string]struct{} {
	set := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		name := strings.TrimSpace(action.Name)
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	return set
}

func sortedCopy(values []string) []string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return copied
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func findAction(actions []Action, name string) (Action, bool) {
	for _, action := range actions {
		if action.Name == name {
			return action, true
		}
	}
	return Action{}, false
}

func resolveAlias(lock Lock, aliasName string) (string, string, error) {
	type aliasTarget struct {
		pkg    string
		action string
	}
	matches := make([]aliasTarget, 0, 1)
	for _, pkg := range lock.Packages {
		for _, alias := range pkg.Aliases {
			if strings.TrimSpace(alias.Name) != aliasName {
				continue
			}
			matches = append(matches, aliasTarget{
				pkg:    pkg.Name,
				action: strings.TrimSpace(alias.Action),
			})
		}
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("alias %q is not installed", aliasName)
	}
	if len(matches) > 1 {
		owners := make([]string, 0, len(matches))
		for _, item := range matches {
			owners = append(owners, item.pkg)
		}
		sort.Strings(owners)
		return "", "", fmt.Errorf("alias %q is ambiguous across packages: %s", aliasName, strings.Join(owners, ", "))
	}
	if matches[0].action == "" {
		return "", "", fmt.Errorf("alias %q has empty action target", aliasName)
	}
	return matches[0].pkg, matches[0].action, nil
}

func findLockedPackage(lock Lock, name string) (LockedPackage, bool) {
	for _, pkg := range lock.Packages {
		if pkg.Name == name {
			return pkg, true
		}
	}
	return LockedPackage{}, false
}

func findLockedPackageByNameOrSource(lock Lock, query string) (LockedPackage, bool) {
	for _, pkg := range lock.Packages {
		if pkg.Name == query || pkg.Source == query {
			return pkg, true
		}
	}
	return LockedPackage{}, false
}

func findRegistry(state State, name string) (RegistryRef, bool) {
	for _, reg := range state.Registries {
		if reg.Name == name {
			return reg, true
		}
	}
	return RegistryRef{}, false
}

func registryLocalPath(projectRoot, registryURL string) (string, bool) {
	switch {
	case strings.HasPrefix(registryURL, "http://"), strings.HasPrefix(registryURL, "https://"):
		return "", false
	case strings.HasPrefix(registryURL, "file://"):
		return strings.TrimPrefix(registryURL, "file://"), true
	default:
		if filepath.IsAbs(registryURL) {
			return registryURL, true
		}
		return filepath.Join(projectRoot, registryURL), true
	}
}

func removeEmptyParents(dir, stopAt string) {
	for {
		if dir == stopAt || dir == "/" || dir == "." {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) > 0 {
			return
		}
		_ = os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

func isReservedAlias(name string) bool {
	switch name {
	case "new", "add", "sync", "vpkg", "tui":
		return true
	}
	return strings.HasPrefix(name, "run:") || strings.HasPrefix(name, "dev:")
}

func parseDurationEnv(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	if seconds, err := strconv.Atoi(raw); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	return fallback
}

func parseIntEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func matchTargetPattern(rel, pattern string) bool {
	rel = filepath.ToSlash(rel)
	pattern = filepath.ToSlash(pattern)
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		return strings.HasPrefix(rel, base+"/") || rel == base
	}
	ok, _ := filepath.Match(filepath.FromSlash(pattern), filepath.FromSlash(rel))
	return ok
}
