package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/turushan/cheep/internal/output"
)

func newVersionCommand(state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build information",
		Args: func(_ *cobra.Command, args []string) error {
			return requireNoArgs(args)
		},
		RunE: func(_ *cobra.Command, _ []string) error {
			return output.Printer{Stdout: state.stdout, Stderr: state.stderr, JSON: state.json}.
				Data("version", state.build, func(w io.Writer) error {
					_, err := fmt.Fprintf(w, "cheep %s (%s)\n", state.build.Version, state.build.Commit)
					return err
				})
		},
	}
}
