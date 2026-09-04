package zonefile

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/turushan/cheep/internal/provider"
)

func TestLoadNormalizesAndSortsZone(t *testing.T) {
	t.Parallel()

	input := "version: 1\ndomain: Example.COM.\nemail_type: none\nrecords:\n" +
		"  - host: www\n    type: cname\n    value: example.com.\n    ttl: 300\n" +
		"  - host: \"\"\n    type: a\n    value: 192.0.2.1\n"
	zone, err := Load(strings.NewReader(input))
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if zone.Domain != "example.com" || zone.EmailType != "NONE" {
		t.Fatalf("unexpected zone: %+v", zone)
	}
	if len(zone.Records) != 2 || zone.Records[0].Host != "@" || zone.Records[0].TTL != defaultTTL {
		t.Fatalf("unexpected records: %+v", zone.Records)
	}
}

func TestLoadRejectsUnknownFieldsAndDuplicates(t *testing.T) {
	t.Parallel()

	unknown := "version: 1\ndomain: example.com\nemail_type: NONE\nsurprise: true\nrecords: []\n"
	_, err := Load(strings.NewReader(unknown))
	if err == nil || !strings.Contains(err.Error(), "field surprise not found") {
		t.Fatalf("unexpected unknown-field error: %v", err)
	}

	duplicate := "version: 1\ndomain: example.com\nemail_type: NONE\nrecords:\n" +
		"  - {host: \"@\", type: A, value: 192.0.2.1, ttl: 300}\n" +
		"  - {host: \"@\", type: A, value: 192.0.2.1, ttl: 300}\n"
	_, err = Load(strings.NewReader(duplicate))
	if err == nil || !strings.Contains(err.Error(), "duplicates") {
		t.Fatalf("unexpected duplicate error: %v", err)
	}
}

func TestSnapshotUsesPrivateFileAndDeterministicName(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path, err := Snapshot(directory, provider.Zone{
		Version:   provider.ZoneFileVersion,
		Domain:    "example.com",
		EmailType: "NONE",
		Records:   []provider.DNSRecord{},
	}, time.Date(2026, 9, 4, 12, 34, 56, 123, time.UTC))
	if err != nil {
		t.Fatalf("Snapshot returned an error: %v", err)
	}
	want := filepath.Join(directory, "example.com", "20260904T123456.000000123Z.yaml")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		got := info.Mode().Perm()
		t.Fatalf("snapshot mode = %o, want 600", got)
	}
}

func TestWriteReplacesExistingZone(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "zone.yaml")
	first := provider.Zone{Version: provider.ZoneFileVersion, Domain: "example.com", EmailType: "NONE"}
	second := provider.Zone{
		Version:   provider.ZoneFileVersion,
		Domain:    "example.com",
		EmailType: "NONE",
		Records:   []provider.DNSRecord{{Host: "@", Type: "A", Value: "192.0.2.2", TTL: 300}},
	}
	if err := Write(path, first); err != nil {
		t.Fatalf("write first zone: %v", err)
	}
	if err := Write(path, second); err != nil {
		t.Fatalf("replace zone: %v", err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open replaced zone: %v", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close replaced zone: %v", err)
		}
	}()
	loaded, err := Load(file)
	if err != nil {
		t.Fatalf("load replaced zone: %v", err)
	}
	if len(loaded.Records) != 1 || loaded.Records[0].Value != "192.0.2.2" {
		t.Fatalf("unexpected replaced zone: %+v", loaded)
	}
}

func TestValidateApplyRefusesUnmodeledZones(t *testing.T) {
	t.Parallel()

	zone := provider.Zone{
		Version:   provider.ZoneFileVersion,
		Domain:    "example.com",
		EmailType: "GMAIL",
		Records:   []provider.DNSRecord{},
	}
	if err := ValidateApply(zone); err == nil || !strings.Contains(err.Error(), "GMAIL") {
		t.Fatalf("unexpected Gmail validation error: %v", err)
	}
	zone.EmailType = "NONE"
	zone.Records = []provider.DNSRecord{{Host: "@", Type: "CAA", Value: "0 issue letsencrypt.org", TTL: 300}}
	if err := ValidateApply(zone); err == nil || !strings.Contains(err.Error(), "CAA") {
		t.Fatalf("unexpected CAA validation error: %v", err)
	}
}

func TestValidateApplyRequiresMatchingEmailRecords(t *testing.T) {
	t.Parallel()

	zone := provider.Zone{
		Version:   provider.ZoneFileVersion,
		Domain:    "example.com",
		EmailType: "NONE",
		Records: []provider.DNSRecord{{
			Host:   "@",
			Type:   "MX",
			Value:  "mail.example.com",
			TTL:    300,
			MXPref: uint8Pointer(10),
		}},
	}
	if err := ValidateApply(zone); err == nil || !strings.Contains(err.Error(), "NONE") {
		t.Fatalf("unexpected NONE validation error: %v", err)
	}
	zone.EmailType = "MX"
	if err := ValidateApply(zone); err != nil {
		t.Fatalf("valid MX zone was rejected: %v", err)
	}
}

func uint8Pointer(value uint8) *uint8 {
	return &value
}
