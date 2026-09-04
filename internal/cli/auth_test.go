package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turushan/cheep/internal/buildinfo"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/secrets"
)

type transactionalSecretStore struct {
	values        map[string]string
	setErr        error
	setCalls      int
	afterFirstSet func()
}

func (s *transactionalSecretStore) Get(profile string) (string, error) {
	value, ok := s.values[profile]
	if !ok {
		return "", secrets.ErrNotFound
	}
	return value, nil
}

func (s *transactionalSecretStore) Set(profile string, value string) error {
	s.setCalls++
	if s.setErr != nil {
		return s.setErr
	}
	s.values[profile] = value
	if s.setCalls == 1 && s.afterFirstSet != nil {
		s.afterFirstSet()
	}
	return nil
}

func (s *transactionalSecretStore) Delete(profile string) error {
	if _, ok := s.values[profile]; !ok {
		return secrets.ErrNotFound
	}
	delete(s.values, profile)
	return nil
}

func TestAuthConfigureValidatesBeforeWritingSecret(t *testing.T) {
	t.Parallel()

	store := &transactionalSecretStore{values: make(map[string]string)}
	configPath := filepath.Join(t.TempDir(), "config.toml")
	status := executeAuthConfigure(configPath, store, "not-public", "new-key")
	if status != exitcode.Usage {
		t.Fatalf("status = %d, want %d", status, exitcode.Usage)
	}
	if store.setCalls != 0 {
		t.Fatalf("keychain writes = %d, want 0", store.setCalls)
	}
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("config was written or stat failed unexpectedly: %v", err)
	}
}

func TestAuthConfigureLeavesConfigUnchangedWhenKeyStorageFails(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.toml")
	original := []byte("default_profile = 'sandbox'\n\n[profiles.sandbox]\napi_user = 'old'\nclient_ip = '8.8.8.8'\nenvironment = 'sandbox'\n")
	if err := os.WriteFile(configPath, original, 0o600); err != nil {
		t.Fatalf("write original config: %v", err)
	}
	store := &transactionalSecretStore{values: make(map[string]string), setErr: errors.New("keychain unavailable")}
	status := executeAuthConfigure(configPath, store, "1.1.1.1", "new-key")
	if status != exitcode.Authentication {
		t.Fatalf("status = %d, want %d", status, exitcode.Authentication)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(content) != string(original) {
		t.Fatalf("config changed after key storage failure:\n%s", content)
	}
}

func TestAuthConfigureRestoresPreviousSecretWhenConfigSaveFails(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	blocker := filepath.Join(directory, "blocker")
	configPath := filepath.Join(blocker, "config.toml")
	store := &transactionalSecretStore{
		values: map[string]string{"sandbox": "old-key"},
		afterFirstSet: func() {
			if err := os.WriteFile(blocker, []byte("blocks config directory"), 0o600); err != nil {
				t.Errorf("create config blocker: %v", err)
			}
		},
	}
	status := executeAuthConfigure(configPath, store, "1.1.1.1", "new-key")
	if status != exitcode.Usage {
		t.Fatalf("status = %d, want %d", status, exitcode.Usage)
	}
	if store.values["sandbox"] != "old-key" {
		t.Fatalf("stored key = %q, want restored old key", store.values["sandbox"])
	}
	if store.setCalls != 2 {
		t.Fatalf("keychain writes = %d, want new value plus rollback", store.setCalls)
	}
}

func executeAuthConfigure(configPath string, store secrets.Store, clientIP string, key string) int {
	return Execute([]string{
		"--environment", "sandbox",
		"auth", "configure", "sandbox",
		"--api-user", "maker",
		"--client-ip", clientIP,
		"--api-key-stdin",
	}, Options{
		Stdin:      strings.NewReader(key + "\n"),
		Build:      buildinfo.Info{Version: "test"},
		Getenv:     func(string) string { return "" },
		ConfigPath: configPath,
		Secrets:    store,
	})
}
