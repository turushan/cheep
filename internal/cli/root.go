package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/turushan/cheep/internal/buildinfo"
	"github.com/turushan/cheep/internal/config"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/failure"
	namecheapapi "github.com/turushan/cheep/internal/namecheap"
	"github.com/turushan/cheep/internal/output"
	"github.com/turushan/cheep/internal/provider"
	"github.com/turushan/cheep/internal/secrets"
)

// Options provides process dependencies without global state.
type Options struct {
	Context         context.Context
	Stdin           io.Reader
	Stdout          io.Writer
	Stderr          io.Writer
	Build           buildinfo.Info
	Getenv          func(string) string
	ConfigPath      string
	Secrets         secrets.Store
	ProviderFactory provider.Factory
	Now             func() time.Time
}

type state struct {
	context     context.Context
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
	timeout     time.Duration
	resolver    config.Resolver
	factory     provider.Factory
	now         func() time.Time
}

// Execute runs the command and maps failures to stable process exit codes.
func Execute(args []string, options Options) int {
	root, state := newRoot(options)
	if requestsJSON(args) {
		state.json = true
	}
	root.SetArgs(args)

	err := root.ExecuteContext(state.context)
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
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.Stdin == nil {
		options.Stdin = strings.NewReader("")
	}
	if options.Stdout == nil {
		options.Stdout = io.Discard
	}
	if options.Stderr == nil {
		options.Stderr = io.Discard
	}
	if options.Getenv == nil {
		options.Getenv = os.Getenv
	}
	if options.Secrets == nil {
		options.Secrets = secrets.Keyring{}
	}
	if options.ConfigPath == "" {
		path, err := config.DefaultPath(options.Getenv)
		if err == nil {
			options.ConfigPath = path
		}
	}
	if options.ProviderFactory == nil {
		options.ProviderFactory = namecheapapi.Factory{Version: options.Build.Version}
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	state := &state{
		context: options.Context,
		stdin:   options.Stdin,
		stdout:  options.Stdout,
		stderr:  options.Stderr,
		build:   options.Build,
		timeout: 30 * time.Second,
		resolver: config.Resolver{
			Path:    options.ConfigPath,
			Getenv:  options.Getenv,
			Secrets: options.Secrets,
		},
		factory: options.ProviderFactory,
		now:     options.Now,
	}

	root := &cobra.Command{
		Use:           "cheep",
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
			if state.timeout <= 0 || state.timeout > 10*time.Minute {
				return failure.New(
					"invalid_timeout",
					exitcode.Usage,
					"timeout must be greater than zero and no more than 10 minutes",
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
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return failure.Wrap("invalid_flags", exitcode.Usage, err.Error(), err)
	})

	flags := root.PersistentFlags()
	flags.BoolVar(&state.json, "json", false, "write a stable JSON document")
	flags.StringVar(&state.profile, "profile", "", "use a named profile")
	flags.StringVar(&state.environment, "environment", "", "override environment: sandbox or production")
	flags.BoolVar(&state.noInput, "no-input", false, "never prompt for input")
	flags.BoolVar(&state.readOnly, "readonly", false, "refuse every remote mutation")
	flags.BoolVar(&state.dryRun, "dry-run", false, "calculate a mutation without applying it")
	flags.DurationVar(&state.timeout, "timeout", 30*time.Second, "maximum time for each command")

	root.AddCommand(newVersionCommand(state))
	root.AddCommand(newSchemaCommand(root, state))
	root.AddCommand(newAuthCommand(state))
	root.AddCommand(newDoctorCommand(state))
	root.AddCommand(newDomainsCommand(state))
	root.AddCommand(newAccountCommand(state))
	root.AddCommand(newTLDsCommand(state))
	root.AddCommand(newDNSCommand(state))

	return root, state
}

func (s *state) service(ctx context.Context) (provider.Service, config.Profile, error) {
	profile, err := s.resolver.Resolve(s.profile, s.environment)
	if err != nil {
		return nil, config.Profile{}, failure.Wrap(
			"authentication_required",
			exitcode.Authentication,
			err.Error(),
			err,
		)
	}
	return s.factory.New(profile), profile, nil
}

func (s *state) reader(ctx context.Context) (provider.Reader, config.Profile, error) {
	service, profile, err := s.service(ctx)
	return service, profile, err
}

func (s *state) commandContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, s.timeout)
}

func (s *state) printer() output.Printer {
	return output.Printer{Stdout: s.stdout, Stderr: s.stderr, JSON: s.json}
}

type executionMeta struct {
	Profile     string             `json:"profile"`
	Environment config.Environment `json:"environment"`
}

func metaFor(profile config.Profile) executionMeta {
	return executionMeta{Profile: profile.Name, Environment: profile.Environment}
}

func mapProviderError(err error) error {
	var providerError *provider.Error
	if !errors.As(err, &providerError) {
		return failure.Wrap("unexpected_error", exitcode.Unexpected, err.Error(), err)
	}
	switch providerError.Kind {
	case provider.ErrorCanceled:
		return failure.Wrap("interrupted", exitcode.Interrupted, providerError.Message, err)
	case provider.ErrorInvalid:
		return failure.Wrap("invalid_request", exitcode.Usage, providerError.Message, err)
	case provider.ErrorAuth:
		return failure.Wrap("authentication_failed", exitcode.Authentication, providerError.Message, err)
	case provider.ErrorNetwork, provider.ErrorRetryable:
		return failure.Wrap("network_error", exitcode.Network, providerError.Message, err)
	case provider.ErrorConflict:
		return failure.Wrap("state_conflict", exitcode.Conflict, providerError.Message, err)
	case provider.ErrorOutcomeUnknown:
		return failure.Wrap("dns_outcome_unknown", exitcode.Conflict, providerError.Message, err)
	case provider.ErrorNotFound:
		return failure.Wrap("not_found", exitcode.Provider, providerError.Message, err)
	case provider.ErrorPermission:
		return failure.Wrap("permission_denied", exitcode.Provider, providerError.Message, err)
	default:
		return failure.Wrap("provider_error", exitcode.Provider, providerError.Message, err)
	}
}

func requestsJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "--json=true" {
			return true
		}
	}
	return false
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

func noArgs(_ *cobra.Command, args []string) error {
	return requireNoArgs(args)
}

func exactArgs(count int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) == count {
			return nil
		}
		return failure.New(
			"invalid_arguments",
			exitcode.Usage,
			fmt.Sprintf("expected %d argument(s), received %d", count, len(args)),
		)
	}
}

func minimumArgs(count int) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) >= count {
			return nil
		}
		return failure.New(
			"invalid_arguments",
			exitcode.Usage,
			fmt.Sprintf("expected at least %d argument(s), received %d", count, len(args)),
		)
	}
}
