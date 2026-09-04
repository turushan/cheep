package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/failure"
	"github.com/turushan/cheep/internal/provider"
	"github.com/turushan/cheep/internal/zonefile"
)

type exportResult struct {
	File    string `json:"file"`
	Domain  string `json:"domain"`
	Records int    `json:"records"`
}

type applyResult struct {
	Applied  bool                 `json:"applied"`
	DryRun   bool                 `json:"dry_run"`
	Snapshot string               `json:"snapshot,omitempty"`
	Plan     provider.ZonePlan    `json:"plan"`
	Change   *provider.ZoneChange `json:"change,omitempty"`
}

func newDNSCommand(state *state) *cobra.Command {
	command := &cobra.Command{
		Use:   "dns",
		Short: "Inspect and safely change Namecheap DNS zones",
		Args:  noArgs,
	}
	command.AddCommand(newDNSListCommand(state))
	command.AddCommand(newDNSExportCommand(state))
	command.AddCommand(newDNSPlanCommand(state))
	command.AddCommand(newDNSApplyCommand(state, "apply", "Apply an exact desired DNS zone"))
	command.AddCommand(newDNSApplyCommand(state, "restore", "Restore a DNS zone from a snapshot"))
	return command
}

func newDNSListCommand(state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "list <domain>",
		Short: "List the complete Namecheap DNS zone",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, err := normalizeDomain(args[0])
			if err != nil {
				return err
			}
			service, profile, err := state.service(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			zone, err := service.GetZone(ctx, domain)
			if err != nil {
				return mapProviderError(err)
			}
			return state.printer().DataWithMeta("dns.list", zone, metaFor(profile), func(w io.Writer) error {
				return writeZone(w, zone)
			})
		},
	}
}

func newDNSExportCommand(state *state) *cobra.Command {
	var filePath string
	command := &cobra.Command{
		Use:   "export <domain>",
		Short: "Export a complete zone as versioned YAML",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, err := normalizeDomain(args[0])
			if err != nil {
				return err
			}
			service, profile, err := state.service(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			zone, err := service.GetZone(ctx, domain)
			if err != nil {
				return mapProviderError(err)
			}
			if state.json {
				if filePath != "-" {
					if err := zonefile.Write(filePath, zone); err != nil {
						return failure.Wrap("zone_write_failed", exitcode.Unexpected, err.Error(), err)
					}
				}
				return state.printer().DataWithMeta("dns.export", zone, metaFor(profile), func(io.Writer) error { return nil })
			}
			if filePath == "-" {
				content, err := zonefile.Marshal(zone)
				if err != nil {
					return failure.Wrap("zone_encode_failed", exitcode.Unexpected, err.Error(), err)
				}
				_, err = state.stdout.Write(content)
				return err
			}
			if err := zonefile.Write(filePath, zone); err != nil {
				return failure.Wrap("zone_write_failed", exitcode.Unexpected, err.Error(), err)
			}
			result := exportResult{File: filePath, Domain: zone.Domain, Records: len(zone.Records)}
			return state.printer().Data("dns.export", result, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Exported %d DNS record(s) for %s to %s.\n", result.Records, result.Domain, result.File)
				return err
			})
		},
	}
	command.Flags().StringVarP(&filePath, "file", "f", "-", "output file, or - for stdout")
	return command
}

func newDNSPlanCommand(state *state) *cobra.Command {
	var filePath string
	command := &cobra.Command{
		Use:   "plan <domain>",
		Short: "Compare a desired zone file with Namecheap without writing",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, desired, err := loadDesiredZone(args[0], filePath, state.stdin)
			if err != nil {
				return err
			}
			service, profile, err := state.service(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			plan, err := service.PlanZone(ctx, desired)
			if err != nil {
				return mapProviderError(err)
			}
			plan.Domain = domain
			return state.printer().DataWithMeta("dns.plan", plan, metaFor(profile), func(w io.Writer) error {
				return writeZonePlan(w, plan)
			})
		},
	}
	command.Flags().StringVarP(&filePath, "file", "f", "", "desired YAML zone file, or - for stdin")
	return command
}

func newDNSApplyCommand(state *state, name string, description string) *cobra.Command {
	var filePath string
	var confirmDomain string
	var confirmEmailType string
	var confirmProduction bool
	var allowEmpty bool
	var snapshotDirectory string
	command := &cobra.Command{
		Use:   name + " <domain>",
		Short: description,
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, desired, err := loadDesiredZone(args[0], filePath, state.stdin)
			if err != nil {
				return err
			}
			if err := zonefile.ValidateApply(desired); err != nil {
				return failure.Wrap("unsafe_zone", exitcode.Safety, err.Error(), err)
			}
			service, profile, err := state.service(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			plan, err := service.PlanZone(ctx, desired)
			if err != nil {
				return mapProviderError(err)
			}
			plan.Domain = domain
			result := applyResult{Plan: plan, DryRun: state.dryRun}
			if !plan.Satisfiable {
				return failure.New("unsatisfiable_zone", exitcode.Safety, "desired records do not match the requested email_type")
			}
			if state.dryRun {
				return state.printer().DataWithMeta("dns."+name, result, metaFor(profile), func(w io.Writer) error {
					if _, err := fmt.Fprintln(w, "DRY RUN"); err != nil {
						return err
					}
					return writeZonePlan(w, plan)
				})
			}
			if state.readOnly {
				return failure.New("readonly", exitcode.Safety, "--readonly refuses DNS mutations")
			}
			if len(plan.Add) == 0 && len(plan.Remove) == 0 && strings.EqualFold(plan.CurrentEmailType, plan.DesiredEmailType) {
				return state.printer().DataWithMeta("dns."+name, result, metaFor(profile), func(w io.Writer) error {
					_, err := fmt.Fprintln(w, "DNS zone already matches the desired state. No write was made.")
					return err
				})
			}
			confirmed, err := normalizeDomain(confirmDomain)
			if err != nil || confirmed != domain {
				return failure.New("confirmation_required", exitcode.Safety, "DNS apply requires --confirm-domain with the exact target domain")
			}
			if len(desired.Records) == 0 && !allowEmpty {
				return failure.New("empty_zone_refused", exitcode.Safety, "an empty DNS zone requires --allow-empty-zone")
			}
			if profile.Environment == "production" && !confirmProduction {
				return failure.New("production_confirmation_required", exitcode.Safety, "production DNS changes require --production")
			}
			if !strings.EqualFold(plan.CurrentEmailType, plan.DesiredEmailType) && !strings.EqualFold(strings.TrimSpace(confirmEmailType), plan.DesiredEmailType) {
				return failure.New("email_confirmation_required", exitcode.Safety, "changing email_type requires --confirm-email-type with the exact desired value")
			}
			current, err := service.GetZone(ctx, domain)
			if err != nil {
				return mapProviderError(err)
			}
			if plan.CurrentFingerprint == "" || provider.FingerprintZone(current) != plan.CurrentFingerprint {
				return failure.New("state_conflict", exitcode.Conflict, "DNS zone changed after planning; make a new plan and review it before retrying")
			}
			if current.NamecheapDNS == nil || !*current.NamecheapDNS {
				return failure.New("external_dns_refused", exitcode.Safety, "the domain is not confirmed to use Namecheap DNS")
			}
			if err := zonefile.ValidateApply(current); err != nil {
				return failure.Wrap("unsafe_current_zone", exitcode.Safety, "current DNS zone cannot be safely replaced: "+err.Error(), err)
			}
			if snapshotDirectory == "" {
				snapshotDirectory = filepath.Join(filepath.Dir(state.resolver.Path), "snapshots")
			}
			snapshotPath, err := zonefile.Snapshot(snapshotDirectory, current, state.now())
			if err != nil {
				return failure.Wrap("snapshot_failed", exitcode.Safety, err.Error(), err)
			}
			result.Snapshot = snapshotPath
			if !state.json {
				if _, err := fmt.Fprintf(state.stderr, "%s DNS apply for %s. Snapshot: %s\n", strings.ToUpper(string(profile.Environment)), domain, snapshotPath); err != nil {
					return failure.Wrap("diagnostic_failed", exitcode.Unexpected, "could not write the DNS safety diagnostic", err)
				}
			}
			change, err := service.ApplyZone(ctx, desired, plan.CurrentFingerprint)
			if err != nil {
				var providerError *provider.Error
				if errors.As(err, &providerError) && providerError.Kind == provider.ErrorOutcomeUnknown {
					message := fmt.Sprintf("DNS update outcome is unknown. Run cheep dns list %s and inspect snapshot %s before retrying: %s", domain, snapshotPath, providerError.Message)
					return failure.Wrap("dns_outcome_unknown", exitcode.Conflict, message, err)
				}
				return mapProviderError(err)
			}
			result.Applied = true
			result.Change = &change
			return state.printer().DataWithMeta("dns."+name, result, metaFor(profile), func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Applied DNS zone for %s: +%d -%d =%d.\n", domain, change.Added, change.Removed, change.Kept)
				return err
			})
		},
	}
	flags := command.Flags()
	flags.StringVarP(&filePath, "file", "f", "", "desired YAML zone file, or - for stdin")
	flags.StringVar(&confirmDomain, "confirm-domain", "", "exact domain confirmation required for a write")
	flags.StringVar(&confirmEmailType, "confirm-email-type", "", "exact desired email type confirmation required when mail mode changes")
	flags.BoolVar(&confirmProduction, "production", false, "confirm this DNS write targets production")
	flags.BoolVar(&allowEmpty, "allow-empty-zone", false, "allow intentional removal of every DNS record")
	flags.StringVar(&snapshotDirectory, "snapshot-dir", "", "directory for automatic pre-change snapshots")
	return command
}

func loadDesiredZone(argument string, filePath string, stdin io.Reader) (string, provider.Zone, error) {
	domain, err := normalizeDomain(argument)
	if err != nil {
		return "", provider.Zone{}, err
	}
	if filePath == "" {
		return "", provider.Zone{}, failure.New("zone_file_required", exitcode.Usage, "--file is required")
	}
	var reader io.Reader
	var file *os.File
	if filePath == "-" {
		reader = stdin
	} else {
		file, err = os.Open(filePath)
		if err != nil {
			return "", provider.Zone{}, failure.Wrap("zone_read_failed", exitcode.Usage, fmt.Sprintf("open zone file: %v", err), err)
		}
		reader = file
	}
	zone, err := zonefile.Load(reader)
	if file != nil {
		closeErr := file.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
	}
	if err != nil {
		return "", provider.Zone{}, failure.Wrap("invalid_zone_file", exitcode.Usage, err.Error(), err)
	}
	zoneDomain, err := normalizeDomain(zone.Domain)
	if err != nil {
		return "", provider.Zone{}, err
	}
	if zoneDomain != domain {
		return "", provider.Zone{}, failure.New("zone_domain_mismatch", exitcode.Safety, fmt.Sprintf("zone file targets %s, not %s", zoneDomain, domain))
	}
	zone.Domain = domain
	return domain, zone, nil
}

func writeZone(w io.Writer, zone provider.Zone) error {
	if _, err := fmt.Fprintf(w, "Domain: %s\nEmail type: %s\n", zone.Domain, zone.EmailType); err != nil {
		return err
	}
	return writeDNSRecords(w, zone.Records, "")
}

func writeZonePlan(w io.Writer, plan provider.ZonePlan) error {
	if _, err := fmt.Fprintf(w, "Domain: %s\nEmail type: %s -> %s\nChanges: +%d -%d =%d\n",
		plan.Domain,
		plan.CurrentEmailType,
		plan.DesiredEmailType,
		len(plan.Add),
		len(plan.Remove),
		len(plan.Keep),
	); err != nil {
		return err
	}
	if !plan.Satisfiable {
		if _, err := fmt.Fprintln(w, "This plan cannot be applied with the requested email type."); err != nil {
			return err
		}
	}
	if err := writeDNSRecords(w, plan.Add, "+"); err != nil {
		return err
	}
	return writeDNSRecords(w, plan.Remove, "-")
}

func writeDNSRecords(w io.Writer, records []provider.DNSRecord, prefix string) error {
	if len(records) == 0 {
		return nil
	}
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "\tHOST\tTYPE\tVALUE\tTTL\tMX"); err != nil {
		return err
	}
	for _, record := range records {
		mx := ""
		if record.MXPref != nil {
			mx = fmt.Sprintf("%d", *record.MXPref)
		}
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%d\t%s\n", prefix, record.Host, record.Type, record.Value, record.TTL, mx); err != nil {
			return err
		}
	}
	return table.Flush()
}
