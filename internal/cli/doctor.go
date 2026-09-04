package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/turushan/nccli/internal/config"
)

type doctorResult struct {
	Profile config.PublicProfile `json:"profile"`
	Remote  bool                 `json:"remote_checked"`
	Domains *int                 `json:"domain_count,omitempty"`
}

func newDoctorCommand(state *state) *cobra.Command {
	var localOnly bool
	command := &cobra.Command{
		Use:   "doctor",
		Short: "Validate configuration and read-only API access",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reader, profile, err := state.reader(cmd.Context())
			if err != nil {
				return err
			}
			result := doctorResult{Profile: profile.Public()}
			if !localOnly {
				ctx, cancel := state.commandContext(cmd.Context())
				defer cancel()
				probe, err := reader.Probe(ctx)
				if err != nil {
					return mapProviderError(err)
				}
				result.Remote = true
				result.Domains = &probe.DomainCount
			}
			return state.printer().DataWithMeta("doctor", result, metaFor(profile), func(w io.Writer) error {
				if _, err := fmt.Fprintf(w, "Profile %q is valid for %s.\n", profile.Name, strings.ToUpper(string(profile.Environment))); err != nil {
					return err
				}
				if localOnly {
					_, err := fmt.Fprintln(w, "Remote API access was not checked.")
					return err
				}
				_, err := fmt.Fprintf(w, "Read-only API access succeeded. Account domains: %d.\n", *result.Domains)
				return err
			})
		},
	}
	command.Flags().BoolVar(&localOnly, "local", false, "validate local configuration without contacting Namecheap")
	return command
}
