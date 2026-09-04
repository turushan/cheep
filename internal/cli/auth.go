package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/turushan/cheep/internal/config"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/failure"
	"github.com/turushan/cheep/internal/secrets"
	"golang.org/x/term"
)

const maxAPIKeyBytes = 4096

type configureResult struct {
	Profile     string             `json:"profile"`
	Environment config.Environment `json:"environment"`
	ConfigPath  string             `json:"config_path"`
	KeyStored   bool               `json:"key_stored"`
	Default     bool               `json:"default"`
}

func newAuthCommand(state *state) *cobra.Command {
	command := &cobra.Command{
		Use:   "auth",
		Short: "Configure and inspect authentication",
		Args:  noArgs,
	}
	command.AddCommand(newAuthConfigureCommand(state))
	command.AddCommand(newAuthSetKeyCommand(state))
	command.AddCommand(newAuthStatusCommand(state))
	return command
}

func newAuthSetKeyCommand(state *state) *cobra.Command {
	var fromStdin bool
	command := &cobra.Command{
		Use:   "set-key",
		Short: "Store an API key in the operating system keychain",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			profileName, err := state.resolver.SelectedProfileName(state.profile)
			if err != nil {
				return failure.Wrap("config_error", exitcode.Authentication, err.Error(), err)
			}
			var key string
			if fromStdin {
				key, err = readSecret(state.stdin)
			} else {
				if state.noInput {
					return failure.New("input_disabled", exitcode.Safety, "--no-input requires --stdin for auth set-key")
				}
				key, err = readSecretFromTerminal(state.stdin, state.stderr)
			}
			if err != nil {
				return err
			}
			if state.resolver.Secrets == nil {
				return failure.New("keychain_unavailable", exitcode.Authentication, "operating system keychain is unavailable")
			}
			if err := state.resolver.Secrets.Set(profileName, key); err != nil {
				return failure.Wrap("keychain_error", exitcode.Authentication, "could not store API key in the operating system keychain", err)
			}
			result := struct {
				Profile string `json:"profile"`
				Stored  bool   `json:"stored"`
			}{Profile: profileName, Stored: true}
			return state.printer().Data("auth.set-key", result, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Stored the API key for profile %q in the operating system keychain.\n", profileName)
				return err
			})
		},
	}
	command.Flags().BoolVar(&fromStdin, "stdin", false, "read the API key from stdin instead of a terminal prompt")
	return command
}

func newAuthConfigureCommand(state *state) *cobra.Command {
	var apiUser string
	var username string
	var clientIP string
	var apiKeyStdin bool
	var makeDefault bool

	command := &cobra.Command{
		Use:   "configure <profile>",
		Short: "Save a profile and optionally store its API key",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			environment := config.Environment(strings.ToLower(state.environment))
			if environment == "" {
				environment = config.Sandbox
			}
			profileName := args[0]
			record := config.ProfileRecord{
				APIUser:     strings.TrimSpace(apiUser),
				Username:    strings.TrimSpace(username),
				ClientIP:    strings.TrimSpace(clientIP),
				Environment: environment,
			}
			if err := config.ValidateProfile(profileName, record); err != nil {
				return failure.Wrap("invalid_profile", exitcode.Usage, err.Error(), err)
			}

			keyStored := false
			var key string
			var previousKey string
			previousKeyExists := false
			if apiKeyStdin {
				var err error
				key, err = readSecret(state.stdin)
				if err != nil {
					return err
				}
				if state.resolver.Secrets == nil {
					return failure.New("keychain_unavailable", exitcode.Authentication, "operating system keychain is unavailable")
				}
				if _, err := state.resolver.Load(); err != nil {
					return failure.Wrap("invalid_profile", exitcode.Usage, err.Error(), err)
				}
				previousKey, err = state.resolver.Secrets.Get(profileName)
				if errors.Is(err, secrets.ErrNotFound) {
					previousKey = ""
				} else if err != nil {
					return failure.Wrap("keychain_error", exitcode.Authentication, "could not read the existing API key from the operating system keychain", err)
				} else {
					previousKeyExists = true
				}
				if err := state.resolver.Secrets.Set(profileName, key); err != nil {
					return failure.Wrap("keychain_error", exitcode.Authentication, "could not store API key in the operating system keychain", err)
				}
				keyStored = true
			}

			if err := state.resolver.SaveProfile(profileName, record, makeDefault); err != nil {
				if keyStored {
					rollbackErr := restoreAPIKey(state.resolver.Secrets, profileName, previousKey, previousKeyExists)
					if rollbackErr != nil {
						return failure.Wrap(
							"auth_configure_rollback_failed",
							exitcode.Unexpected,
							"could not save the profile or restore the previous API key",
							errors.Join(err, rollbackErr),
						)
					}
				}
				return failure.Wrap("invalid_profile", exitcode.Usage, err.Error(), err)
			}
			savedConfig, err := state.resolver.Load()
			if err != nil {
				return failure.Wrap("config_error", exitcode.Unexpected, err.Error(), err)
			}

			result := configureResult{
				Profile:     profileName,
				Environment: environment,
				ConfigPath:  state.resolver.Path,
				KeyStored:   keyStored,
				Default:     savedConfig.DefaultProfile == profileName,
			}
			return state.printer().Data("auth.configure", result, func(w io.Writer) error {
				if _, err := fmt.Fprintf(w, "Saved %s profile %q to %s.\n", strings.ToUpper(string(environment)), profileName, state.resolver.Path); err != nil {
					return err
				}
				if keyStored {
					_, err := fmt.Fprintln(w, "Stored the API key in the operating system keychain.")
					return err
				}
				_, err := fmt.Fprintln(w, "No API key was stored. Use --api-key-stdin or a CHEEP_API_KEY environment variable.")
				return err
			})
		},
	}
	flags := command.Flags()
	flags.StringVar(&apiUser, "api-user", "", "Namecheap API user")
	flags.StringVar(&username, "username", "", "Namecheap username; defaults to API user")
	flags.StringVar(&clientIP, "client-ip", "", "public IPv4 address whitelisted by Namecheap")
	flags.BoolVar(&apiKeyStdin, "api-key-stdin", false, "read the API key from stdin and store it in the keychain")
	flags.BoolVar(&makeDefault, "default", false, "make this the default profile")
	return command
}

func restoreAPIKey(store secrets.Store, profileName string, previousKey string, previousKeyExists bool) error {
	if previousKeyExists {
		return store.Set(profileName, previousKey)
	}
	err := store.Delete(profileName)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil
	}
	return err
}

func newAuthStatusCommand(state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the resolved profile without exposing its API key",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			profile, err := state.resolver.Resolve(state.profile, state.environment)
			if err != nil {
				return failure.Wrap("authentication_required", exitcode.Authentication, err.Error(), err)
			}
			public := profile.Public()
			return state.printer().Data("auth.status", public, func(w io.Writer) error {
				_, err := fmt.Fprintf(
					w,
					"Profile: %s\nEnvironment: %s\nAPI user: %s\nUsername: %s\nClient IP: %s\nAPI key: %s\nConfig: %s\n",
					public.Name,
					strings.ToUpper(string(public.Environment)),
					public.APIUser,
					public.Username,
					public.ClientIP,
					public.APIKeySource,
					public.ConfigPath,
				)
				return err
			})
		},
	}
}

func readSecret(reader io.Reader) (string, error) {
	content, err := io.ReadAll(io.LimitReader(reader, maxAPIKeyBytes+1))
	if err != nil {
		return "", failure.Wrap("secret_read_failed", exitcode.Authentication, "could not read API key from stdin", err)
	}
	if len(content) > maxAPIKeyBytes {
		return "", failure.New("secret_too_long", exitcode.Usage, "API key input is too long")
	}
	value := strings.TrimSpace(string(content))
	if value == "" {
		return "", failure.New("secret_empty", exitcode.Usage, "API key input is empty")
	}
	return value, nil
}

func readSecretFromTerminal(reader io.Reader, stderr io.Writer) (string, error) {
	terminal, ok := reader.(interface{ Fd() uintptr })
	if !ok || !term.IsTerminal(int(terminal.Fd())) {
		return "", failure.New("terminal_required", exitcode.Usage, "a terminal is required; use auth set-key --stdin for piped input")
	}
	if _, err := fmt.Fprint(stderr, "Namecheap API key: "); err != nil {
		return "", failure.Wrap("prompt_failed", exitcode.Unexpected, "could not write API key prompt", err)
	}
	content, err := term.ReadPassword(int(terminal.Fd()))
	_, _ = fmt.Fprintln(stderr)
	if err != nil {
		return "", failure.Wrap("secret_read_failed", exitcode.Authentication, "could not read API key from terminal", err)
	}
	value := strings.TrimSpace(string(content))
	for i := range content {
		content[i] = 0
	}
	if value == "" {
		return "", failure.New("secret_empty", exitcode.Usage, "API key input is empty")
	}
	if len(value) > maxAPIKeyBytes {
		return "", failure.New("secret_too_long", exitcode.Usage, "API key input is too long")
	}
	return value, nil
}
