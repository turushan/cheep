package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/turushan/nccli/internal/buildinfo"
	"github.com/turushan/nccli/internal/exitcode"
	"github.com/turushan/nccli/internal/failure"
	"github.com/turushan/nccli/internal/output"
)

// Options provides process dependencies without global state.
type Options struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Build  buildinfo.Info
}

type state struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	build       buildinfo.Info
	json        bool
	profile     string
	environment string
	noInput     bool
	readOnly    bool
	dryRun      bool
}

// Execute runs the command and maps failures to stable process exit codes.
func Execute(args []string, options Options) int {
	root, state := newRoot(options)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		return exitcode.Success
	}

	code, status, message := failure.Details(err)
	command := commandName(root, args)
	output.Printer{Stdout: state.stdout, Stderr: state.stderr, JSON: state.json}.
		Failure(command, code, message)
	return status
}

func newRoot(options Options) (*cobra.Command, *state) {
	state := &state{
		stdin:  options.Stdin,
		stdout: options.Stdout,
		stderr: options.Stderr,
		build:  options.Build,
	}

	root := &cobra.Command{
		Use:           "nccli",
		Short:         "The safe, unofficial Namecheap CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if state.environment != "" && state.environment != "sandbox" && state.environment != "production" {
				return failure.New(
					"invalid_environment",
					exitcode.Usage,
					"environment must be sandbox or production",
				)
			}
			return nil
		},
	}
	root.SetIn(state.stdin)
	root.SetOut(state.stdout)
	root.SetErr(state.stderr)
	root.CompletionOptions.DisableDefaultCmd = false
	root.SetHelpCommand(&cobra.Command{Hidden: true})

	flags := root.PersistentFlags()
	flags.BoolVar(&state.json, "json", false, "write a stable JSON document")
	flags.StringVar(&state.profile, "profile", "", "use a named profile")
	flags.StringVar(&state.environment, "environment", "", "override environment: sandbox or production")
	flags.BoolVar(&state.noInput, "no-input", false, "never prompt for input")
	flags.BoolVar(&state.readOnly, "readonly", false, "refuse every remote mutation")
	flags.BoolVar(&state.dryRun, "dry-run", false, "calculate a mutation without applying it")

	root.AddCommand(newVersionCommand(state))
	root.AddCommand(newSchemaCommand(root, state))

	return root, state
}

func commandName(root *cobra.Command, args []string) string {
	command, _, err := root.Find(args)
	if err == nil && command != nil {
		name := strings.TrimSpace(strings.TrimPrefix(command.CommandPath(), root.Name()))
		if name != "" {
			return strings.ReplaceAll(name, " ", ".")
		}
	}
	return root.Name()
}

func requireNoArgs(args []string) error {
	if len(args) == 0 {
		return nil
	}
	return failure.New("unexpected_arguments", exitcode.Usage, fmt.Sprintf("unexpected arguments: %s", strings.Join(args, " ")))
}
