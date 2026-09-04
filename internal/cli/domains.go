package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/failure"
	"github.com/turushan/cheep/internal/provider"
	"golang.org/x/net/idna"
)

func newDomainsCommand(state *state) *cobra.Command {
	command := &cobra.Command{
		Use:   "domains",
		Short: "Inspect and manage domains",
		Args:  noArgs,
	}
	command.AddCommand(newDomainsListCommand(state))
	command.AddCommand(newDomainsInfoCommand(state))
	command.AddCommand(newDomainsCheckCommand(state))
	return command
}

func newDomainsListCommand(state *state) *cobra.Command {
	var listType string
	var search string
	var sortBy string
	command := &cobra.Command{
		Use:   "list",
		Short: "List every domain in the selected account",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			listType = strings.ToLower(listType)
			if !oneOf(listType, "all", "expiring", "expired") {
				return failure.New("invalid_list_type", exitcode.Usage, "type must be all, expiring, or expired")
			}
			sortValue, ok := domainSortValue(sortBy)
			if !ok {
				return failure.New("invalid_sort", exitcode.Usage, "sort must be name, name-desc, expiry, expiry-desc, created, or created-desc")
			}
			reader, profile, err := state.reader(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			domains, err := reader.ListDomains(ctx, provider.DomainListFilter{
				ListType: listType,
				Search:   strings.TrimSpace(search),
				Sort:     sortValue,
			})
			if err != nil {
				return mapProviderError(err)
			}
			return state.printer().DataWithMeta("domains.list", domains, metaFor(profile), func(w io.Writer) error {
				return writeDomainTable(w, domains)
			})
		},
	}
	flags := command.Flags()
	flags.StringVar(&listType, "type", "all", "domain set: all, expiring, or expired")
	flags.StringVar(&search, "search", "", "filter by a domain name fragment")
	flags.StringVar(&sortBy, "sort", "name", "sort order")
	return command
}

func newDomainsInfoCommand(state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "info <domain>",
		Short: "Show detailed information about one domain",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domain, err := normalizeDomain(args[0])
			if err != nil {
				return err
			}
			reader, profile, err := state.reader(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			info, err := reader.DomainInfo(ctx, domain)
			if err != nil {
				return mapProviderError(err)
			}
			return state.printer().DataWithMeta("domains.info", info, metaFor(profile), func(w io.Writer) error {
				return writeDomainInfo(w, info)
			})
		},
	}
}

func newDomainsCheckCommand(state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "check <domain>...",
		Short: "Check availability for one or more domains",
		Args:  minimumArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			domains := make([]string, 0, len(args))
			seen := make(map[string]struct{}, len(args))
			for _, argument := range args {
				domain, err := normalizeDomain(argument)
				if err != nil {
					return err
				}
				if _, exists := seen[domain]; exists {
					continue
				}
				seen[domain] = struct{}{}
				domains = append(domains, domain)
			}
			reader, profile, err := state.reader(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			checks, err := reader.CheckDomains(ctx, domains)
			if err != nil {
				return mapProviderError(err)
			}
			return state.printer().DataWithMeta("domains.check", checks, metaFor(profile), func(w io.Writer) error {
				return writeDomainChecks(w, checks)
			})
		},
	}
}

func normalizeDomain(value string) (string, error) {
	value = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if value == "" || strings.ContainsAny(value, "/:@") || !strings.Contains(value, ".") {
		return "", failure.New("invalid_domain", exitcode.Usage, fmt.Sprintf("invalid domain: %q", value))
	}
	ascii, err := idna.Lookup.ToASCII(value)
	if err != nil {
		return "", failure.Wrap("invalid_domain", exitcode.Usage, fmt.Sprintf("invalid domain: %q", value), err)
	}
	if len(ascii) > 253 {
		return "", failure.New("invalid_domain", exitcode.Usage, fmt.Sprintf("invalid domain: %q", value))
	}
	for _, label := range strings.Split(ascii, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", failure.New("invalid_domain", exitcode.Usage, fmt.Sprintf("invalid domain: %q", value))
		}
	}
	return ascii, nil
}

func domainSortValue(value string) (string, bool) {
	switch strings.ToLower(value) {
	case "name":
		return "NAME", true
	case "name-desc":
		return "NAME_DESC", true
	case "expiry":
		return "EXPIREDATE", true
	case "expiry-desc":
		return "EXPIREDATE_DESC", true
	case "created":
		return "CREATEDATE", true
	case "created-desc":
		return "CREATEDATE_DESC", true
	default:
		return "", false
	}
}

func writeDomainTable(w io.Writer, domains []provider.Domain) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "DOMAIN\tEXPIRES\tLOCKED\tAUTO RENEW\tNAMECHEAP DNS"); err != nil {
		return err
	}
	for _, domain := range domains {
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\t%s\n",
			domain.Name,
			dateValue(domain.Expires),
			boolValue(domain.Locked),
			boolValue(domain.AutoRenew),
			boolValue(domain.NamecheapDNS),
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

func writeDomainChecks(w io.Writer, checks []provider.DomainCheck) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "DOMAIN\tAVAILABLE\tPREMIUM\tPREMIUM REGISTRATION"); err != nil {
		return err
	}
	for _, check := range checks {
		price := ""
		if check.PremiumRegistrationPrice != nil {
			price = fmt.Sprintf("%.2f", *check.PremiumRegistrationPrice)
		}
		if _, err := fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", check.Domain, boolValue(check.Available), boolValue(check.Premium), price); err != nil {
			return err
		}
	}
	return table.Flush()
}

func writeDomainInfo(w io.Writer, info provider.DomainInfo) error {
	if _, err := fmt.Fprintf(w, "Domain: %s\nStatus: %s\nOwner: %s\nCreated: %s\nExpires: %s\nPrivacy: %s\nDNS provider: %s\n",
		info.Name,
		info.Status,
		info.Owner,
		dateValue(info.Created),
		dateValue(info.Expires),
		info.Privacy.Status,
		info.DNS.Provider,
	); err != nil {
		return err
	}
	if len(info.DNS.Nameservers) > 0 {
		if _, err := fmt.Fprintln(w, "Nameservers:"); err != nil {
			return err
		}
		for _, nameserver := range info.DNS.Nameservers {
			if _, err := fmt.Fprintf(w, "  %s\n", nameserver); err != nil {
				return err
			}
		}
	}
	if len(info.Rights) > 0 {
		keys := make([]string, 0, len(info.Rights))
		for key := range info.Rights {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if _, err := fmt.Fprintln(w, "Rights:"); err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := fmt.Fprintf(w, "  %s: %s\n", key, info.Rights[key]); err != nil {
				return err
			}
		}
	}
	return nil
}

func boolValue(value *bool) string {
	if value == nil {
		return "unknown"
	}
	if *value {
		return "yes"
	}
	return "no"
}

func dateValue(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "unknown"
	}
	return value.Format("2006-01-02")
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
