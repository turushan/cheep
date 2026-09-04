package provider

import "testing"

func TestFingerprintZoneNormalizesProviderFormatting(t *testing.T) {
	t.Parallel()
	falseValue := false
	left := Zone{
		Domain:    "EXAMPLE.COM.",
		EmailType: "none",
		Records: []DNSRecord{
			{Host: "WWW", Type: "cname", Value: "target.example.com.", TTL: 0, DDNSEnabled: &falseValue},
			{Host: "@", Type: "A", Value: "192.0.2.1", TTL: 300},
		},
	}
	right := Zone{
		Domain:    "example.com",
		EmailType: "NONE",
		Records: []DNSRecord{
			{Host: "@", Type: "A", Value: "192.0.2.1", TTL: 300},
			{Host: "www", Type: "CNAME", Value: "target.example.com", TTL: 1799},
		},
	}
	if FingerprintZone(left) != FingerprintZone(right) {
		t.Fatal("equivalent zones produced different fingerprints")
	}
}

func TestBuildZonePlanUsesOneCurrentState(t *testing.T) {
	t.Parallel()
	currentRecord := DNSRecord{Host: "@", Type: "A", Value: "192.0.2.1", TTL: 300}
	desiredRecord := DNSRecord{Host: "@", Type: "A", Value: "192.0.2.2", TTL: 300}
	current := Zone{Domain: "example.com", EmailType: "NONE", Records: []DNSRecord{currentRecord}}
	desired := Zone{Domain: "example.com", EmailType: "NONE", Records: []DNSRecord{desiredRecord}}
	plan := BuildZonePlan(current, desired)
	if plan.CurrentFingerprint != FingerprintZone(current) || !plan.Satisfiable {
		t.Fatalf("unexpected plan metadata: %+v", plan)
	}
	if len(plan.Add) != 1 || len(plan.Remove) != 1 || len(plan.Keep) != 0 {
		t.Fatalf("unexpected plan diff: %+v", plan)
	}
}
