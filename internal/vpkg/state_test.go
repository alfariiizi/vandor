package vpkg

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStateDefaultsWhenMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(envRegistryOfficial, "https://example.com/official")
	t.Setenv(envRegistryVerified, "https://example.com/verified")
	t.Setenv(envRegistryCommunity, "https://example.com/community")

	state, err := LoadState(tmp)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.Registries) != 3 {
		t.Fatalf("expected 3 default registries, got %d", len(state.Registries))
	}
	got := map[string]string{}
	for _, reg := range state.Registries {
		got[reg.Name] = reg.URL
	}
	if got[TierOfficial] != "https://example.com/official" {
		t.Fatalf("unexpected official registry: %q", got[TierOfficial])
	}
	if got[TierVerified] != "https://example.com/verified" {
		t.Fatalf("unexpected verified registry: %q", got[TierVerified])
	}
	if got[TierCommunity] != "https://example.com/community" {
		t.Fatalf("unexpected community registry: %q", got[TierCommunity])
	}
}

func TestLoadStateExplicitFileDoesNotInjectDefaults(t *testing.T) {
	tmp := t.TempDir()
	raw := []byte("registries:\n  - name: custom\n    url: ./custom\n")
	if err := os.WriteFile(filepath.Join(tmp, stateFileName), raw, 0o644); err != nil {
		t.Fatalf("write state file: %v", err)
	}

	state, err := LoadState(tmp)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if len(state.Registries) != 1 {
		t.Fatalf("expected 1 registry from file, got %d", len(state.Registries))
	}
	if state.Registries[0].Name != "custom" || state.Registries[0].URL != "./custom" {
		t.Fatalf("unexpected registry: %#v", state.Registries[0])
	}
}
