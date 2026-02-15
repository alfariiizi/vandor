package vpkg

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	stateFileName = "vpkg.yaml"
	lockFileName  = "vpkg.lock"
	lockVersion   = 1

	defaultRegistryBaseURL = "https://vpkg.vercel.app"

	envRegistryOfficial  = "VANDOR_VPKG_REGISTRY_OFFICIAL"
	envRegistryVerified  = "VANDOR_VPKG_REGISTRY_VERIFIED"
	envRegistryCommunity = "VANDOR_VPKG_REGISTRY_COMMUNITY"
)

func LoadState(projectRoot string) (State, error) {
	path := filepath.Join(projectRoot, stateFileName)
	// nosemgrep: go-path-traversal -- path is fixed file name under project root.
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{Registries: defaultRegistries(projectRoot)}, nil
		}
		return State{}, err
	}
	var state State
	if err := yaml.Unmarshal(raw, &state); err != nil {
		return State{}, fmt.Errorf("parse %s: %w", stateFileName, err)
	}
	return state, nil
}

func SaveState(projectRoot string, state State) error {
	sort.Slice(state.Registries, func(i, j int) bool {
		return state.Registries[i].Name < state.Registries[j].Name
	})
	sort.Slice(state.Packages, func(i, j int) bool {
		return state.Packages[i].Source < state.Packages[j].Source
	})
	raw, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(projectRoot, stateFileName), raw, 0o644)
}

func LoadLock(projectRoot string) (Lock, error) {
	path := filepath.Join(projectRoot, lockFileName)
	// nosemgrep: go-path-traversal -- path is fixed file name under project root.
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Lock{LockVersion: lockVersion}, nil
		}
		return Lock{}, err
	}
	var lock Lock
	if err := yaml.Unmarshal(raw, &lock); err != nil {
		return Lock{}, fmt.Errorf("parse %s: %w", lockFileName, err)
	}
	if lock.LockVersion == 0 {
		lock.LockVersion = lockVersion
	}
	return lock, nil
}

func SaveLock(projectRoot string, lock Lock) error {
	lock.LockVersion = lockVersion
	sort.Slice(lock.Packages, func(i, j int) bool {
		return lock.Packages[i].Name < lock.Packages[j].Name
	})
	for i := range lock.Packages {
		sort.Slice(lock.Packages[i].Ownership.Files, func(a, b int) bool {
			return lock.Packages[i].Ownership.Files[a].Path < lock.Packages[i].Ownership.Files[b].Path
		})
		sort.Strings(lock.Packages[i].Ownership.Aliases)
		sort.Slice(lock.Packages[i].Actions, func(a, b int) bool {
			return lock.Packages[i].Actions[a].Name < lock.Packages[i].Actions[b].Name
		})
		sort.Slice(lock.Packages[i].Aliases, func(a, b int) bool {
			return lock.Packages[i].Aliases[a].Name < lock.Packages[i].Aliases[b].Name
		})
	}
	raw, err := yaml.Marshal(lock)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(projectRoot, lockFileName), raw, 0o644)
}

func EnsureStateDirs(projectRoot string) (string, error) {
	base := filepath.Join(projectRoot, ".vandor", "vpkg")
	cache := filepath.Join(base, "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	return base, nil
}

func upsertStatePackage(state *State, source string) {
	for _, p := range state.Packages {
		if p.Source == source {
			return
		}
	}
	state.Packages = append(state.Packages, StatePkgRef{Source: source})
}

func removeStatePackage(state *State, source string) {
	filtered := state.Packages[:0]
	for _, p := range state.Packages {
		if p.Source != source {
			filtered = append(filtered, p)
		}
	}
	state.Packages = filtered
}

func upsertLockedPackage(lock *Lock, pkg LockedPackage) {
	for i := range lock.Packages {
		if lock.Packages[i].Name == pkg.Name {
			lock.Packages[i] = pkg
			return
		}
	}
	lock.Packages = append(lock.Packages, pkg)
}

func removeLockedPackage(lock *Lock, name string) bool {
	idx := slices.IndexFunc(lock.Packages, func(p LockedPackage) bool { return p.Name == name })
	if idx < 0 {
		return false
	}
	lock.Packages = append(lock.Packages[:idx], lock.Packages[idx+1:]...)
	return true
}

func defaultRegistries(projectRoot string) []RegistryRef {
	return []RegistryRef{
		{Name: TierOfficial, URL: defaultRegistryURL(projectRoot, TierOfficial)},
		{Name: TierVerified, URL: defaultRegistryURL(projectRoot, TierVerified)},
		{Name: TierCommunity, URL: defaultRegistryURL(projectRoot, TierCommunity)},
	}
}

func defaultRegistryURL(projectRoot, tier string) string {
	switch tier {
	case TierOfficial:
		if value := strings.TrimSpace(os.Getenv(envRegistryOfficial)); value != "" {
			return value
		}
	case TierVerified:
		if value := strings.TrimSpace(os.Getenv(envRegistryVerified)); value != "" {
			return value
		}
	case TierCommunity:
		if value := strings.TrimSpace(os.Getenv(envRegistryCommunity)); value != "" {
			return value
		}
	}
	if local, ok := detectLocalRegistryRepo(projectRoot); ok {
		return local
	}
	return defaultRegistryBaseURL
}

func detectLocalRegistryRepo(projectRoot string) (string, bool) {
	candidates := []string{
		filepath.Join(projectRoot, "..", "vpkg"),
		filepath.Join(projectRoot, "..", "..", "vpkg"),
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
