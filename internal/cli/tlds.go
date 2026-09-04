package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/turushan/nccli/internal/provider"
)

func newTLDsCommand(state *state) *cobra.Command {
	command := &cobra.Command{
		Use:   "tlds",
		Short: "Inspect Namecheap TLD capabilities",
		Args:  noArgs,
	}
	command.AddCommand(newTLDsListCommand(state))
	return command
}

func newTLDsListCommand(state *state) *cobra.Command {
	var registerable bool
	var renewable bool
	var transferable bool
	command := &cobra.Command{
		Use:   "list",
		Short: "List TLDs and API capabilities",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, profile, err := state.reader(cmd.Context())
			if err != nil {
				return err
			}
			ctx, cancel := state.commandContext(cmd.Context())
			defer cancel()
			values, err := reader.ListTLDs(ctx)
			if err != nil {
				return mapProviderError(err)
			}
			filtered := values[:0]
			for _, value := range values {
				if registerable && !isTrue(value.APIRegisterable) {
					continue
				}
				if renewable && !isTrue(value.APIRenewable) {
					continue
				}
				if transferable && !isTrue(value.APITransferable) {
					continue
				}
				filtered = append(filtered, value)
			}
			return state.printer().DataWithMeta("tlds.list", filtered, metaFor(profile), func(w io.Writer) error {
				return writeTLDs(w, filtered)
			})
		},
	}
	flags := command.Flags()
	flags.BoolVar(&registerable, "registerable", false, "include only TLDs registerable through the API")
	flags.BoolVar(&renewable, "renewable", false, "include only TLDs renewable through the API")
	flags.BoolVar(&transferable, "transferable", false, "include only TLDs transferable through the API")
	return command
}

func writeTLDs(w io.Writer, values []provider.TLD) error {
	table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "TLD\tREGISTER\tRENEW\tTRANSFER\tEPP"); err != nil {
		return err
	}
	for _, value := range values {
		if _, err := fmt.Fprintf(table, ".%s\t%s\t%s\t%s\t%s\n",
			value.Name,
			boolValue(value.APIRegisterable),
			boolValue(value.APIRenewable),
			boolValue(value.APITransferable),
			boolValue(value.EPPRequired),
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

func isTrue(value *bool) bool {
	return value != nil && *value
}
