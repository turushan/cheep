package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/turushan/cheep/internal/buildinfo"
	"github.com/turushan/cheep/internal/config"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/output"
	"github.com/turushan/cheep/internal/provider"
	"github.com/turushan/cheep/internal/secrets"
)

type fakeFactory struct {
	reader  provider.Service
	profile *config.Profile
}

func (f *fakeFactory) New(profile config.Profile) provider.Service {
	*f.profile = profile
	return f.reader
}

type fakeReader struct {
	probe            provider.Probe
	checks           []provider.DomainCheck
	checkDomains     []string
	checkError       error
	zone             provider.Zone
	plan             provider.ZonePlan
	change           provider.ZoneChange
	applyError       error
	applyFingerprint string
	getZoneCalls     int
	planCalls        int
	applyCalls       int
}

func (f *fakeReader) Probe(context.Context) (provider.Probe, error) {
	return f.probe, nil
}

func (f *fakeReader) ListDomains(context.Context, provider.DomainListFilter) ([]provider.Domain, error) {
	return nil, nil
}

func (f *fakeReader) DomainInfo(context.Context, string) (provider.DomainInfo, error) {
	return provider.DomainInfo{}, nil
}

func (f *fakeReader) CheckDomains(_ context.Context, domains []string) ([]provider.DomainCheck, error) {
	f.checkDomains = append([]string(nil), domains...)
	return f.checks, f.checkError
}

func (f *fakeReader) Balance(context.Context) (provider.Balance, error) {
	return provider.Balance{}, nil
}

func (f *fakeReader) Price(context.Context, provider.PriceRequest) (provider.Price, error) {
	return provider.Price{}, nil
}

func (f *fakeReader) ListTLDs(context.Context) ([]provider.TLD, error) {
	return nil, nil
}

func (f *fakeReader) GetZone(context.Context, string) (provider.Zone, error) {
	f.getZoneCalls++
	return f.zone, nil
}

func (f *fakeReader) PlanZone(context.Context, provider.Zone) (provider.ZonePlan, error) {
	f.planCalls++
	return f.plan, nil
}

func (f *fakeReader) ApplyZone(_ context.Context, _ provider.Zone, fingerprint string) (provider.ZoneChange, error) {
	f.applyCalls++
	f.applyFingerprint = fingerprint
	return f.change, f.applyError
}

func TestDomainsCheckNormalizesAndDeduplicatesNames(t *testing.T) {
	t.Parallel()

	available := true
	reader := &fakeReader{checks: []provider.DomainCheck{{Domain: "xn--bcher-kva.de", Available: &available}}}
	options, stdout, stderr := authenticatedOptions(t, reader)
	status := Execute([]string{"domains", "check", "BÜCHER.de.", "xn--bcher-kva.de", "--json"}, options)
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	if len(reader.checkDomains) != 1 || reader.checkDomains[0] != "xn--bcher-kva.de" {
		t.Fatalf("domains = %#v", reader.checkDomains)
	}

	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.Command != "domains.check" || !envelope.OK || envelope.Meta == nil {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestDoctorUsesReadOnlyProbe(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{probe: provider.Probe{DomainCount: 12}}
	options, stdout, stderr := authenticatedOptions(t, reader)
	status := Execute([]string{"doctor"}, options)
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Account domains: 12") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestProviderFailureUsesStableJSONError(t *testing.T) {
	t.Parallel()

	reader := &fakeReader{checkError: &provider.Error{
		Kind:    provider.ErrorAuth,
		Message: "access denied",
	}}
	options, stdout, stderr := authenticatedOptions(t, reader)
	status := Execute([]string{"domains", "check", "example.com", "--json"}, options)
	if status != exitcode.Authentication {
		t.Fatalf("status = %d, want %d", status, exitcode.Authentication)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stderr.String()), &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.Error == nil || envelope.Error.Code != "authentication_failed" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestAuthConfigureStoresSecretOutsideConfig(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.toml")
	store := secrets.NewMemory()
	var stdout strings.Builder
	var stderr strings.Builder
	status := Execute([]string{
		"--environment", "sandbox",
		"auth", "configure", "sandbox",
		"--api-user", "maker",
		"--client-ip", "8.8.8.8",
		"--api-key-stdin",
		"--default",
		"--json",
	}, Options{
		Stdin:      strings.NewReader("private-test-key\n"),
		Stdout:     &stdout,
		Stderr:     &stderr,
		Build:      buildinfo.Info{Version: "test"},
		Getenv:     func(string) string { return "" },
		ConfigPath: configPath,
		Secrets:    store,
	})
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(content), "private-test-key") {
		t.Fatal("configuration file exposed API key")
	}
	stored, err := store.Get("sandbox")
	if err != nil || stored != "private-test-key" {
		t.Fatalf("stored key = %q, %v", stored, err)
	}
}

func TestJSONIsDetectedAfterSubcommandOnUsageFailure(t *testing.T) {
	t.Parallel()

	status, stdout, stderr := run(t, "domains", "check", "--json")
	if status != exitcode.Usage {
		t.Fatalf("status = %d, want %d", status, exitcode.Usage)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.Error == nil {
		t.Fatalf("expected JSON error, got %+v", envelope)
	}
}

func TestDNSApplyDryRunNeverWritesOrSnapshots(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	options, stdout, stderr := authenticatedOptions(t, reader)
	zonePath := writeZoneInput(t, false)
	status := Execute([]string{"--dry-run", "dns", "apply", "example.com", "--file", zonePath, "--json"}, options)
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	if reader.planCalls != 1 || reader.getZoneCalls != 0 || reader.applyCalls != 0 {
		t.Fatalf("calls: plan=%d get=%d apply=%d", reader.planCalls, reader.getZoneCalls, reader.applyCalls)
	}
	if !strings.Contains(stdout.String(), `"dry_run":true`) {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestDNSApplyRequiresExactConfirmationBeforeSnapshot(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	options, _, stderr := authenticatedOptions(t, reader)
	zonePath := writeZoneInput(t, false)
	status := Execute([]string{"dns", "apply", "example.com", "--file", zonePath}, options)
	if status != exitcode.Safety {
		t.Fatalf("status = %d, want %d; stderr: %s", status, exitcode.Safety, stderr.String())
	}
	if reader.getZoneCalls != 0 || reader.applyCalls != 0 {
		t.Fatalf("calls: get=%d apply=%d", reader.getZoneCalls, reader.applyCalls)
	}
}

func TestDNSApplySnapshotsThenUsesVerifiedProviderFlow(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	options, stdout, stderr := authenticatedOptions(t, reader)
	snapshotDirectory := t.TempDir()
	zonePath := writeZoneInput(t, false)
	options.Now = func() time.Time {
		return time.Date(2026, 9, 4, 18, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
	}
	status := Execute([]string{
		"dns", "apply", "example.com",
		"--file", zonePath,
		"--confirm-domain", "example.com",
		"--snapshot-dir", snapshotDirectory,
	}, options)
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	if reader.getZoneCalls != 1 || reader.applyCalls != 1 {
		t.Fatalf("calls: get=%d apply=%d", reader.getZoneCalls, reader.applyCalls)
	}
	if reader.applyFingerprint != reader.plan.CurrentFingerprint {
		t.Fatalf("apply fingerprint = %q, want %q", reader.applyFingerprint, reader.plan.CurrentFingerprint)
	}
	snapshotPath := filepath.Join(snapshotDirectory, "example.com", "20260904T163000.000000000Z.yaml")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
	if !strings.Contains(stderr.String(), "SANDBOX DNS apply") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "Applied DNS zone") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestDNSApplyJSONWritesNoHumanDiagnostic(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	options, stdout, stderr := authenticatedOptions(t, reader)
	status := Execute([]string{
		"dns", "apply", "example.com",
		"--file", writeZoneInput(t, false),
		"--confirm-domain", "example.com",
		"--snapshot-dir", t.TempDir(),
		"--json",
	}, options)
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil || !envelope.OK {
		t.Fatalf("unexpected JSON result: %+v, %v", envelope, err)
	}
}

func TestDNSApplyRefusesChangedStateBeforeSnapshot(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	reader.zone.Records[0].Value = "192.0.2.99"
	options, _, stderr := authenticatedOptions(t, reader)
	snapshotDirectory := t.TempDir()
	status := Execute([]string{
		"dns", "apply", "example.com",
		"--file", writeZoneInput(t, false),
		"--confirm-domain", "example.com",
		"--snapshot-dir", snapshotDirectory,
	}, options)
	if status != exitcode.Conflict {
		t.Fatalf("status = %d, want %d; stderr: %s", status, exitcode.Conflict, stderr.String())
	}
	if reader.applyCalls != 0 {
		t.Fatalf("apply calls = %d, want 0", reader.applyCalls)
	}
	entries, err := os.ReadDir(snapshotDirectory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("snapshot directory entries = %d, %v; want none", len(entries), err)
	}
}

func TestDNSApplyRequiresProductionConfirmation(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	options, _, stderr := authenticatedOptions(t, reader)
	status := Execute([]string{
		"--environment", "production",
		"dns", "apply", "example.com",
		"--file", writeZoneInput(t, false),
		"--confirm-domain", "example.com",
	}, options)
	if status != exitcode.Safety {
		t.Fatalf("status = %d, want %d; stderr: %s", status, exitcode.Safety, stderr.String())
	}
	if reader.getZoneCalls != 0 || reader.applyCalls != 0 {
		t.Fatalf("calls: get=%d apply=%d", reader.getZoneCalls, reader.applyCalls)
	}
}

func TestDNSApplyRequiresSeparateEmailTypeConfirmation(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	preference := uint8(10)
	reader.plan.DesiredEmailType = "MX"
	reader.plan.Add = []provider.DNSRecord{{Host: "@", Type: "MX", Value: "mail.example.com", TTL: 300, MXPref: &preference}}
	zonePath := filepath.Join(t.TempDir(), "zone.yaml")
	content := "version: 1\ndomain: example.com\nemail_type: MX\nrecords:\n  - {host: '@', type: MX, value: mail.example.com, ttl: 300, mx_pref: 10}\n"
	if err := os.WriteFile(zonePath, []byte(content), 0o600); err != nil {
		t.Fatalf("write zone input: %v", err)
	}
	options, _, stderr := authenticatedOptions(t, reader)
	status := Execute([]string{
		"dns", "apply", "example.com",
		"--file", zonePath,
		"--confirm-domain", "example.com",
	}, options)
	if status != exitcode.Safety {
		t.Fatalf("status = %d, want %d; stderr: %s", status, exitcode.Safety, stderr.String())
	}
	if reader.getZoneCalls != 0 || reader.applyCalls != 0 {
		t.Fatalf("calls: get=%d apply=%d", reader.getZoneCalls, reader.applyCalls)
	}
}

func TestDNSApplyUnknownOutcomeNamesSnapshotInJSONError(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	reader.applyError = &provider.Error{Kind: provider.ErrorOutcomeUnknown, Message: "verification timed out"}
	options, stdout, stderr := authenticatedOptions(t, reader)
	snapshotDirectory := t.TempDir()
	status := Execute([]string{
		"dns", "apply", "example.com",
		"--file", writeZoneInput(t, false),
		"--confirm-domain", "example.com",
		"--snapshot-dir", snapshotDirectory,
		"--json",
	}, options)
	if status != exitcode.Conflict {
		t.Fatalf("status = %d, want %d; stderr: %s", status, exitcode.Conflict, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stderr.String()), &envelope); err != nil {
		t.Fatalf("decode JSON error: %v", err)
	}
	if envelope.Error == nil || envelope.Error.Code != "dns_outcome_unknown" || !strings.Contains(envelope.Error.Message, snapshotDirectory) {
		t.Fatalf("unexpected error envelope: %+v", envelope)
	}
}

func TestDNSApplyRefusesEmptyZoneWithoutSeparateFlag(t *testing.T) {
	t.Parallel()

	reader := dnsFakeReader()
	reader.plan = provider.ZonePlan{
		Domain:           "example.com",
		CurrentEmailType: "NONE",
		DesiredEmailType: "NONE",
		Satisfiable:      true,
		Remove:           []provider.DNSRecord{{Host: "@", Type: "A", Value: "192.0.2.1", TTL: 300}},
	}
	options, _, stderr := authenticatedOptions(t, reader)
	zonePath := writeZoneInput(t, true)
	status := Execute([]string{
		"dns", "apply", "example.com",
		"--file", zonePath,
		"--confirm-domain", "example.com",
	}, options)
	if status != exitcode.Safety {
		t.Fatalf("status = %d, want %d; stderr: %s", status, exitcode.Safety, stderr.String())
	}
	if reader.applyCalls != 0 || reader.getZoneCalls != 0 {
		t.Fatalf("calls: get=%d apply=%d", reader.getZoneCalls, reader.applyCalls)
	}
}

func dnsFakeReader() *fakeReader {
	usingNamecheap := true
	current := provider.DNSRecord{Host: "@", Type: "A", Value: "192.0.2.1", TTL: 300}
	desired := provider.DNSRecord{Host: "@", Type: "A", Value: "192.0.2.2", TTL: 300}
	zone := provider.Zone{
		Version:      provider.ZoneFileVersion,
		Domain:       "example.com",
		EmailType:    "NONE",
		NamecheapDNS: &usingNamecheap,
		Records:      []provider.DNSRecord{current},
	}
	return &fakeReader{
		zone: zone,
		plan: provider.ZonePlan{
			Domain:             "example.com",
			CurrentFingerprint: provider.FingerprintZone(zone),
			CurrentEmailType:   "NONE",
			DesiredEmailType:   "NONE",
			Satisfiable:        true,
			Add:                []provider.DNSRecord{desired},
			Remove:             []provider.DNSRecord{current},
		},
		change: provider.ZoneChange{
			Domain:    "example.com",
			Added:     1,
			Removed:   1,
			EmailType: "NONE",
			Records:   []provider.DNSRecord{desired},
		},
	}
}

func writeZoneInput(t *testing.T, empty bool) string {
	t.Helper()
	content := "version: 1\ndomain: example.com\nemail_type: NONE\nrecords: []\n"
	if !empty {
		content = "version: 1\ndomain: example.com\nemail_type: NONE\nrecords:\n" +
			"  - {host: \"@\", type: A, value: 192.0.2.2, ttl: 300}\n"
	}
	path := filepath.Join(t.TempDir(), "zone.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write zone input: %v", err)
	}
	return path
}

func authenticatedOptions(t *testing.T, reader provider.Service) (Options, *strings.Builder, *strings.Builder) {
	t.Helper()
	values := map[string]string{
		"CHEEP_API_USER":    "maker",
		"CHEEP_API_KEY":     "test-key",
		"CHEEP_CLIENT_IP":   "8.8.8.8",
		"CHEEP_ENVIRONMENT": "sandbox",
	}
	var resolved config.Profile
	var stdout strings.Builder
	var stderr strings.Builder
	return Options{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
		Build:  buildinfo.Info{Version: "test"},
		Getenv: func(name string) string {
			return values[name]
		},
		ConfigPath:      filepath.Join(t.TempDir(), "missing.toml"),
		Secrets:         secrets.NewMemory(),
		ProviderFactory: &fakeFactory{reader: reader, profile: &resolved},
	}, &stdout, &stderr
}
