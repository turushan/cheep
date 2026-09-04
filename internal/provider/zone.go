package provider

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

const defaultZoneTTL = 1799

// FingerprintZone returns a stable digest of the complete provider-owned zone state.
func FingerprintZone(zone Zone) string {
	canonical := canonicalZone(zone)
	var builder strings.Builder
	fmt.Fprintf(&builder, "%s\x00%s\n", canonical.Domain, canonical.EmailType)
	for _, record := range canonical.Records {
		fmt.Fprintln(&builder, recordIdentity(record))
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

// BuildZonePlan compares two complete zones without another provider read.
func BuildZonePlan(current Zone, desired Zone) ZonePlan {
	current = canonicalZone(current)
	desired = canonicalZone(desired)

	remaining := make(map[string][]DNSRecord, len(current.Records))
	for _, record := range current.Records {
		key := recordIdentity(record)
		remaining[key] = append(remaining[key], record)
	}

	plan := ZonePlan{
		Domain:             desired.Domain,
		CurrentFingerprint: FingerprintZone(current),
		CurrentEmailType:   current.EmailType,
		DesiredEmailType:   desired.EmailType,
		Add:                make([]DNSRecord, 0),
		Remove:             make([]DNSRecord, 0),
		Keep:               make([]DNSRecord, 0),
	}
	for _, record := range desired.Records {
		key := recordIdentity(record)
		matches := remaining[key]
		if len(matches) == 0 {
			plan.Add = append(plan.Add, record)
			continue
		}
		plan.Keep = append(plan.Keep, record)
		if len(matches) == 1 {
			delete(remaining, key)
		} else {
			remaining[key] = matches[1:]
		}
	}
	for _, records := range remaining {
		plan.Remove = append(plan.Remove, records...)
	}
	sortRecords(plan.Remove)
	plan.Satisfiable, plan.RequiredEmailType = emailTypeCompatibility(desired)
	return plan
}

func canonicalZone(zone Zone) Zone {
	zone.Domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(zone.Domain), "."))
	zone.EmailType = strings.ToUpper(strings.TrimSpace(zone.EmailType))
	if zone.EmailType == "" {
		zone.EmailType = "NONE"
	}
	zone.NamecheapDNS = nil
	zone.Records = append([]DNSRecord(nil), zone.Records...)
	for index := range zone.Records {
		record := &zone.Records[index]
		record.Host = strings.ToLower(strings.TrimSpace(record.Host))
		if record.Host == "" {
			record.Host = "@"
		}
		record.Type = strings.ToUpper(strings.TrimSpace(record.Type))
		record.Value = canonicalRecordValue(record.Type, record.Value)
		if record.TTL == 0 {
			record.TTL = defaultZoneTTL
		}
		record.ManagedBy = strings.TrimSpace(record.ManagedBy)
		if record.DDNSEnabled != nil && !*record.DDNSEnabled {
			record.DDNSEnabled = nil
		}
	}
	sortRecords(zone.Records)
	return zone
}

func canonicalRecordValue(recordType string, value string) string {
	value = strings.TrimSpace(value)
	switch recordType {
	case "ALIAS", "CNAME", "MX", "NS":
		return strings.ToLower(strings.TrimSuffix(value, "."))
	default:
		return value
	}
}

func sortRecords(records []DNSRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		return recordIdentity(records[i]) < recordIdentity(records[j])
	})
}

func recordIdentity(record DNSRecord) string {
	mx := ""
	if record.MXPref != nil {
		mx = fmt.Sprintf("%d", *record.MXPref)
	}
	ddns := "false"
	if record.DDNSEnabled != nil && *record.DDNSEnabled {
		ddns = "true"
	}
	return strings.Join([]string{
		record.Host,
		record.Type,
		record.Value,
		fmt.Sprintf("%d", record.TTL),
		mx,
		record.ManagedBy,
		ddns,
	}, "\x00")
}

func emailTypeCompatibility(zone Zone) (bool, string) {
	mxCount := 0
	mxeCount := 0
	for _, record := range zone.Records {
		switch record.Type {
		case "MX":
			mxCount++
		case "MXE":
			mxeCount++
		}
	}
	if mxCount > 0 && mxeCount > 0 {
		return false, ""
	}
	switch zone.EmailType {
	case "MX":
		return mxCount > 0 && mxeCount == 0, "MX"
	case "MXE":
		return mxeCount == 1 && mxCount == 0, "MXE"
	default:
		return mxCount == 0 && mxeCount == 0, ""
	}
}
