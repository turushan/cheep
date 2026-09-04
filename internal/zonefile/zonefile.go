package zonefile

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/turushan/cheep/internal/fsutil"
	"github.com/turushan/cheep/internal/provider"
	"go.yaml.in/yaml/v3"
)

const (
	maxFileBytes = 1024 * 1024
	defaultTTL   = 1799
)

var allowedRecordTypes = map[string]struct{}{
	"A":      {},
	"AAAA":   {},
	"ALIAS":  {},
	"CAA":    {},
	"CNAME":  {},
	"FRAME":  {},
	"MX":     {},
	"MXE":    {},
	"NS":     {},
	"TXT":    {},
	"URL":    {},
	"URL301": {},
}

var allowedEmailTypes = map[string]struct{}{
	"NONE":  {},
	"MXE":   {},
	"MX":    {},
	"FWD":   {},
	"OX":    {},
	"GMAIL": {},
}

// Load decodes one strict, versioned zone document.
func Load(reader io.Reader) (provider.Zone, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxFileBytes+1))
	if err != nil {
		return provider.Zone{}, fmt.Errorf("read zone file: %w", err)
	}
	if len(content) > maxFileBytes {
		return provider.Zone{}, fmt.Errorf("zone file exceeds %d bytes", maxFileBytes)
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(content)))
	decoder.KnownFields(true)
	var zone provider.Zone
	if err := decoder.Decode(&zone); err != nil {
		return provider.Zone{}, fmt.Errorf("decode zone file: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return provider.Zone{}, errors.New("zone file must contain exactly one document")
		}
		return provider.Zone{}, fmt.Errorf("decode trailing zone file content: %w", err)
	}
	if err := NormalizeAndValidate(&zone); err != nil {
		return provider.Zone{}, err
	}
	return zone, nil
}

// Marshal produces a deterministic YAML zone document.
func Marshal(zone provider.Zone) ([]byte, error) {
	if err := NormalizeAndValidate(&zone); err != nil {
		return nil, err
	}
	content, err := yaml.Marshal(zone)
	if err != nil {
		return nil, fmt.Errorf("encode zone file: %w", err)
	}
	return content, nil
}

// Write atomically stores a zone document with private permissions.
func Write(path string, zone provider.Zone) error {
	content, err := Marshal(zone)
	if err != nil {
		return err
	}
	return atomicWrite(path, content)
}

// Snapshot stores a timestamped pre-change zone and returns its path.
func Snapshot(directory string, zone provider.Zone, now time.Time) (string, error) {
	if directory == "" {
		return "", errors.New("snapshot directory is empty")
	}
	domainDirectory := filepath.Join(directory, zone.Domain)
	filename := now.UTC().Format("20060102T150405.000000000Z") + ".yaml"
	path := filepath.Join(domainDirectory, filename)
	if err := Write(path, zone); err != nil {
		return "", fmt.Errorf("write DNS snapshot: %w", err)
	}
	return path, nil
}

// NormalizeAndValidate canonicalizes a zone and rejects unsafe or malformed records.
func NormalizeAndValidate(zone *provider.Zone) error {
	if zone.Version != provider.ZoneFileVersion {
		return fmt.Errorf("zone version must be %d", provider.ZoneFileVersion)
	}
	zone.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone.Domain), "."))
	if zone.Domain == "" || !strings.Contains(zone.Domain, ".") {
		return errors.New("zone domain is invalid")
	}
	zone.EmailType = strings.ToUpper(strings.TrimSpace(zone.EmailType))
	if _, ok := allowedEmailTypes[zone.EmailType]; !ok {
		return errors.New("email_type must be NONE, MXE, MX, FWD, OX, or GMAIL")
	}
	if zone.Records == nil {
		zone.Records = make([]provider.DNSRecord, 0)
	}

	seen := make(map[string]struct{}, len(zone.Records))
	for index := range zone.Records {
		record := &zone.Records[index]
		record.Host = strings.ToLower(strings.TrimSpace(record.Host))
		if record.Host == "" {
			record.Host = "@"
		}
		record.Type = strings.ToUpper(strings.TrimSpace(record.Type))
		if _, ok := allowedRecordTypes[record.Type]; !ok {
			return fmt.Errorf("records[%d].type is unsupported: %s", index, record.Type)
		}
		if strings.TrimSpace(record.Value) == "" {
			return fmt.Errorf("records[%d].value is required", index)
		}
		if record.TTL == 0 {
			record.TTL = defaultTTL
		}
		if record.TTL < 60 || record.TTL > 60000 {
			return fmt.Errorf("records[%d].ttl must be between 60 and 60000", index)
		}
		if record.Type == "MX" && record.MXPref == nil {
			return fmt.Errorf("records[%d].mx_pref is required for MX", index)
		}
		if record.Type != "MX" && record.MXPref != nil {
			return fmt.Errorf("records[%d].mx_pref is valid only for MX", index)
		}
		key := recordKey(*record)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("records[%d] duplicates another record", index)
		}
		seen[key] = struct{}{}
	}
	sort.SliceStable(zone.Records, func(i, j int) bool {
		return recordKey(zone.Records[i]) < recordKey(zone.Records[j])
	})
	return nil
}

// ValidateApply rejects provider combinations that cannot round-trip safely yet.
func ValidateApply(zone provider.Zone) error {
	if err := NormalizeAndValidate(&zone); err != nil {
		return err
	}
	if zone.EmailType == "OX" || zone.EmailType == "GMAIL" {
		return fmt.Errorf("applying records to %s mail zones is disabled until Namecheap behavior is verified", zone.EmailType)
	}
	mxCount := 0
	mxeCount := 0
	for _, record := range zone.Records {
		if record.ManagedBy != "" {
			return fmt.Errorf("record %s %s is managed by %s and cannot be safely replaced", record.Host, record.Type, record.ManagedBy)
		}
		if record.DDNSEnabled != nil && *record.DDNSEnabled {
			return fmt.Errorf("record %s %s has dynamic DNS enabled and cannot be safely replaced", record.Host, record.Type)
		}
		if record.Type == "CAA" {
			return errors.New("applying CAA records is disabled until per-record flag and tag behavior is modeled")
		}
		if record.Type == "MX" {
			mxCount++
		}
		if record.Type == "MXE" {
			mxeCount++
		}
	}
	if mxCount > 0 && mxeCount > 0 {
		return errors.New("a zone cannot contain both MX and MXE records")
	}
	switch zone.EmailType {
	case "MX":
		if mxCount == 0 {
			return errors.New("email_type MX requires at least one MX record")
		}
	case "MXE":
		if mxeCount != 1 {
			return errors.New("email_type MXE requires exactly one MXE record")
		}
	default:
		if mxCount > 0 || mxeCount > 0 {
			return fmt.Errorf("email_type %s cannot contain MX or MXE records", zone.EmailType)
		}
	}
	return nil
}

func recordKey(record provider.DNSRecord) string {
	mx := ""
	if record.MXPref != nil {
		mx = fmt.Sprintf("%d", *record.MXPref)
	}
	return strings.Join([]string{
		strings.ToLower(record.Host),
		strings.ToUpper(record.Type),
		record.Value,
		mx,
		fmt.Sprintf("%d", record.TTL),
	}, "\x00")
}

func atomicWrite(path string, content []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create zone directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".zone-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary zone file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("secure temporary zone file: %w", err)
	}
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary zone file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary zone file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary zone file: %w", err)
	}
	if err := fsutil.Replace(temporaryPath, path); err != nil {
		return fmt.Errorf("replace zone file: %w", err)
	}
	return nil
}
