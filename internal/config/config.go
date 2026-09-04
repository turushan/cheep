package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/turushan/nccli/internal/fsutil"
	"github.com/turushan/nccli/internal/secrets"
)

const DefaultProfile = "sandbox"

type Environment string

const (
	Sandbox    Environment = "sandbox"
	Production Environment = "production"
)

var profileNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

var nonPublicIPv4Prefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
}

// File is the non-secret configuration stored on disk.
type File struct {
	DefaultProfile string                   `toml:"default_profile,omitempty"`
	Profiles       map[string]ProfileRecord `toml:"profiles,omitempty"`
}

// ProfileRecord is safe to store in a configuration file.
type ProfileRecord struct {
	APIUser     string      `toml:"api_user"`
	Username    string      `toml:"username,omitempty"`
	ClientIP    string      `toml:"client_ip"`
	Environment Environment `toml:"environment"`
}

// Profile contains the resolved runtime configuration. APIKey must never be serialized.
type Profile struct {
	Name         string
	APIUser      string
	Username     string
	ClientIP     string
	Environment  Environment
	APIKey       string
	ConfigPath   string
	APIKeySource string
}

// PublicProfile is safe for command output and diagnostics.
type PublicProfile struct {
	Name         string      `json:"name"`
	APIUser      string      `json:"api_user"`
	Username     string      `json:"username"`
	ClientIP     string      `json:"client_ip"`
	Environment  Environment `json:"environment"`
	APIKeySource string      `json:"api_key_source"`
	ConfigPath   string      `json:"config_path"`
}

// Public returns the profile fields that may be printed.
func (p Profile) Public() PublicProfile {
	return PublicProfile{
		Name:         p.Name,
		APIUser:      p.APIUser,
		Username:     p.Username,
		ClientIP:     maskIPv4(p.ClientIP),
		Environment:  p.Environment,
		APIKeySource: p.APIKeySource,
		ConfigPath:   p.ConfigPath,
	}
}

// Resolver loads profiles and applies environment overrides.
type Resolver struct {
	Path    string
	Getenv  func(string) string
	Secrets secrets.Store
}

// DefaultPath returns the platform configuration path.
func DefaultPath(getenv func(string) string) (string, error) {
	if path := strings.TrimSpace(getenv("NCCLI_CONFIG")); path != "" {
		return filepath.Clean(path), nil
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate user configuration directory: %w", err)
	}
	return filepath.Join(directory, "nccli", "config.toml"), nil
}

// Resolve returns a validated profile and obtains its API key from the environment or keychain.
func (r Resolver) Resolve(profileOverride string, environmentOverride string) (Profile, error) {
	file, err := r.Load()
	if err != nil {
		return Profile{}, err
	}

	name := r.selectedProfileName(profileOverride, file)
	record := file.Profiles[name]

	environmentText := firstNonEmpty(
		environmentOverride,
		r.getenv("NCCLI_ENVIRONMENT"),
		r.getenv("NAMECHEAP_ENVIRONMENT"),
		string(record.Environment),
		legacyEnvironment(r.getenv("NAMECHEAP_USE_SANDBOX")),
		string(Sandbox),
	)
	profile := Profile{
		Name:        name,
		APIUser:     firstNonEmpty(r.getenv("NCCLI_API_USER"), r.getenv("NAMECHEAP_API_USER"), record.APIUser),
		Username:    firstNonEmpty(r.getenv("NCCLI_USERNAME"), r.getenv("NAMECHEAP_USERNAME"), r.getenv("NAMECHEAP_USER_NAME"), record.Username),
		ClientIP:    firstNonEmpty(r.getenv("NCCLI_CLIENT_IP"), r.getenv("NAMECHEAP_CLIENT_IP"), record.ClientIP),
		Environment: Environment(strings.ToLower(environmentText)),
		ConfigPath:  r.Path,
	}
	if profile.Username == "" {
		profile.Username = profile.APIUser
	}

	profile.APIKey, profile.APIKeySource, err = r.resolveAPIKey(name)
	if err != nil {
		return Profile{}, err
	}
	if err := validateResolved(profile); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// SelectedProfileName resolves the profile selector without requiring credentials.
func (r Resolver) SelectedProfileName(profileOverride string) (string, error) {
	file, err := r.Load()
	if err != nil {
		return "", err
	}
	return r.selectedProfileName(profileOverride, file), nil
}

func (r Resolver) selectedProfileName(profileOverride string, file File) string {
	return firstNonEmpty(
		profileOverride,
		r.getenv("NCCLI_PROFILE"),
		r.getenv("NAMECHEAP_PROFILE"),
		file.DefaultProfile,
		DefaultProfile,
	)
}

// Load reads the non-secret configuration. A missing file is an empty configuration.
func (r Resolver) Load() (File, error) {
	content, err := os.ReadFile(r.Path)
	if errors.Is(err, os.ErrNotExist) {
		return File{Profiles: make(map[string]ProfileRecord)}, nil
	}
	if err != nil {
		return File{}, fmt.Errorf("read config %s: %w", r.Path, err)
	}
	var file File
	if err := toml.Unmarshal(content, &file); err != nil {
		return File{}, fmt.Errorf("parse config %s: %w", r.Path, err)
	}
	if file.Profiles == nil {
		file.Profiles = make(map[string]ProfileRecord)
	}
	return file, nil
}

// SaveProfile validates and atomically stores non-secret profile data.
func (r Resolver) SaveProfile(name string, record ProfileRecord, makeDefault bool) error {
	var err error
	record, err = validatedProfileRecord(name, record)
	if err != nil {
		return err
	}

	file, err := r.Load()
	if err != nil {
		return err
	}
	file.Profiles[name] = record
	if makeDefault || (file.DefaultProfile == "" && record.Environment == Sandbox) {
		file.DefaultProfile = name
	}
	content, err := toml.Marshal(file)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return atomicWrite(r.Path, content)
}

// ValidateProfile validates profile data without writing configuration or secrets.
func ValidateProfile(name string, record ProfileRecord) error {
	_, err := validatedProfileRecord(name, record)
	return err
}

func validatedProfileRecord(name string, record ProfileRecord) (ProfileRecord, error) {
	if !profileNamePattern.MatchString(name) {
		return ProfileRecord{}, fmt.Errorf("profile name must contain only letters, numbers, underscores, and hyphens")
	}
	record.Environment = Environment(strings.ToLower(string(record.Environment)))
	if record.Username == "" {
		record.Username = record.APIUser
	}
	if err := validateRecord(record); err != nil {
		return ProfileRecord{}, err
	}
	return record, nil
}

func (r Resolver) resolveAPIKey(profile string) (string, string, error) {
	if value := strings.TrimSpace(r.getenv("NCCLI_API_KEY")); value != "" {
		return value, "NCCLI_API_KEY", nil
	}
	if value := strings.TrimSpace(r.getenv("NAMECHEAP_API_KEY")); value != "" {
		return value, "NAMECHEAP_API_KEY", nil
	}
	if r.Secrets == nil {
		return "", "", nil
	}
	value, err := r.Secrets.Get(profile)
	if errors.Is(err, secrets.ErrNotFound) {
		return "", "", nil
	}
	if err != nil {
		return "", "", fmt.Errorf("read API key from keychain: %w", err)
	}
	return value, "keychain", nil
}

func (r Resolver) getenv(name string) string {
	if r.Getenv == nil {
		return ""
	}
	return strings.TrimSpace(r.Getenv(name))
}

func validateResolved(profile Profile) error {
	record := ProfileRecord{
		APIUser:     profile.APIUser,
		Username:    profile.Username,
		ClientIP:    profile.ClientIP,
		Environment: profile.Environment,
	}
	if err := validateRecord(record); err != nil {
		return err
	}
	if profile.APIKey == "" {
		return errors.New("API key is missing; set NCCLI_API_KEY or store it with auth configure --api-key-stdin")
	}
	return nil
}

func validateRecord(record ProfileRecord) error {
	missing := make([]string, 0, 3)
	if record.APIUser == "" {
		missing = append(missing, "api_user")
	}
	if record.Username == "" {
		missing = append(missing, "username")
	}
	if record.ClientIP == "" {
		missing = append(missing, "client_ip")
	}
	if len(missing) > 0 {
		return fmt.Errorf("profile is missing %s", strings.Join(missing, ", "))
	}
	if record.Environment != Sandbox && record.Environment != Production {
		return errors.New("environment must be sandbox or production")
	}
	ip := net.ParseIP(record.ClientIP)
	if ip == nil || ip.To4() == nil {
		return errors.New("client_ip must be a public IPv4 address")
	}
	if !isPublicIPv4(ip.To4()) {
		return errors.New("client_ip must be a public IPv4 address")
	}
	return nil
}

func isPublicIPv4(ip net.IP) bool {
	address, err := netip.ParseAddr(ip.String())
	if err != nil || !address.Is4() || !address.IsGlobalUnicast() {
		return false
	}
	for _, prefix := range nonPublicIPv4Prefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func maskIPv4(value string) string {
	ip := net.ParseIP(value)
	if ip == nil || ip.To4() == nil {
		return "invalid"
	}
	parts := strings.Split(ip.To4().String(), ".")
	return strings.Join([]string{parts[0], parts[1], "x", "x"}, ".")
}

func legacyEnvironment(value string) string {
	if value == "" {
		return ""
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return value
	}
	if parsed {
		return string(Sandbox)
	}
	return string(Production)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func atomicWrite(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary config: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary config: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := fsutil.Replace(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
