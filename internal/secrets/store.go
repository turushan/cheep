package secrets

import (
	"errors"

	keyring "github.com/zalando/go-keyring"
)

const serviceName = "nccli"

var ErrNotFound = errors.New("secret not found")

// Store persists API keys without exposing them to configuration files.
type Store interface {
	Get(profile string) (string, error)
	Set(profile, value string) error
	Delete(profile string) error
}

// Keyring stores secrets in the operating system credential manager.
type Keyring struct{}

func (Keyring) Get(profile string) (string, error) {
	value, err := keyring.Get(serviceName, account(profile))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return value, err
}

func (Keyring) Set(profile, value string) error {
	return keyring.Set(serviceName, account(profile), value)
}

func (Keyring) Delete(profile string) error {
	err := keyring.Delete(serviceName, account(profile))
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}

func account(profile string) string {
	return "profile:" + profile + ":api-key"
}
