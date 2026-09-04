package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/turushan/nccli/internal/secrets"
)

func TestResolveDefaultsToSandboxAndUsesKeychain(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := secrets.NewMemory()
	if err := store.Set("personal", "not-a-real-key"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	resolver := Resolver{
		Path:    filepath.Join(directory, "config.toml"),
		Getenv:  func(string) string { return "" },
		Secrets: store,
	}
	if err := resolver.SaveProfile("personal", ProfileRecord{
		APIUser:     "maker",
		ClientIP:    "203.0.113.10",
		Environment: Sandbox,
	}, true); err == nil || !strings.Contains(err.Error(), "public IPv4") {
		t.Fatalf("expected documentation IP to be rejected, got %v", err)
	}
	if err := resolver.SaveProfile("personal", ProfileRecord{
		APIUser:     "maker",
		ClientIP:    "8.8.8.8",
		Environment: Sandbox,
	}, true); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	profile, err := resolver.Resolve("", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if profile.Name != "personal" || profile.Environment != Sandbox || profile.APIKeySource != "keychain" {
		t.Fatalf("unexpected profile: %+v", profile.Public())
	}
	if profile.Username != "maker" {
		t.Fatalf("username = %q, want API user fallback", profile.Username)
	}
	if profile.Public().ClientIP != "8.8.x.x" {
		t.Fatalf("masked client IP = %q", profile.Public().ClientIP)
	}
}

func TestEnvironmentOverridesProfileAndKeychain(t *testing.T) {
	t.Parallel()

	values := map[string]string{
		"NCCLI_PROFILE":     "ci",
		"NCCLI_API_USER":    "environment-user",
		"NCCLI_API_KEY":     "environment-key",
		"NCCLI_CLIENT_IP":   "1.1.1.1",
		"NCCLI_ENVIRONMENT": "production",
	}
	resolver := Resolver{
		Path: filepath.Join(t.TempDir(), "missing.toml"),
		Getenv: func(name string) string {
			return values[name]
		},
		Secrets: secrets.NewMemory(),
	}
	profile, err := resolver.Resolve("", "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if profile.Name != "ci" || profile.APIUser != "environment-user" || profile.Environment != Production {
		t.Fatalf("unexpected profile: %+v", profile.Public())
	}
	if profile.APIKeySource != "NCCLI_API_KEY" {
		t.Fatalf("API key source = %q", profile.APIKeySource)
	}
}

func TestSaveProfileUsesPrivateFileMode(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "nested", "config.toml")
	resolver := Resolver{Path: path, Getenv: func(string) string { return "" }}
	if err := resolver.SaveProfile("sandbox", ProfileRecord{
		APIUser:     "maker",
		ClientIP:    "8.8.4.4",
		Environment: Sandbox,
	}, true); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		got := info.Mode().Perm()
		t.Fatalf("config mode = %o, want 600", got)
	}
}

func TestFirstProductionProfileIsNotImplicitDefault(t *testing.T) {
	t.Parallel()

	resolver := Resolver{Path: filepath.Join(t.TempDir(), "config.toml"), Getenv: func(string) string { return "" }}
	if err := resolver.SaveProfile("production", ProfileRecord{
		APIUser:     "maker",
		ClientIP:    "8.8.8.8",
		Environment: Production,
	}, false); err != nil {
		t.Fatalf("save production profile: %v", err)
	}
	file, err := resolver.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if file.DefaultProfile != "" {
		t.Fatalf("default profile = %q, want empty", file.DefaultProfile)
	}
	if selected, err := resolver.SelectedProfileName(""); err != nil || selected != DefaultProfile {
		t.Fatalf("selected profile = %q, %v; want %q", selected, err, DefaultProfile)
	}
}

func TestFirstSandboxProfileIsImplicitDefault(t *testing.T) {
	t.Parallel()

	resolver := Resolver{Path: filepath.Join(t.TempDir(), "config.toml"), Getenv: func(string) string { return "" }}
	if err := resolver.SaveProfile("personal", ProfileRecord{
		APIUser:     "maker",
		ClientIP:    "8.8.8.8",
		Environment: Sandbox,
	}, false); err != nil {
		t.Fatalf("save sandbox profile: %v", err)
	}
	file, err := resolver.Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if file.DefaultProfile != "personal" {
		t.Fatalf("default profile = %q, want personal", file.DefaultProfile)
	}
}

func TestResolvedProfileNeverSerializesAPIKey(t *testing.T) {
	t.Parallel()

	profile := Profile{APIKey: "secret-value"}
	if output := profile.Public(); strings.Contains(output.APIKeySource, profile.APIKey) {
		t.Fatal("public profile exposed API key")
	}
}
