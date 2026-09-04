package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/turushan/cheep/internal/apicatalog"
	"github.com/turushan/cheep/internal/config"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/failure"
	"github.com/turushan/cheep/internal/provider"
)

const maximumAPIParameters = 256

var apiParameterNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

type apiCallFlags struct {
	parameters   []string
	paramsFile   string
	secretParams []string
	yes          bool
	production   bool
	acceptCharge bool
}

type apiCallResult struct {
	Method        string                `json:"method"`
	Path          string                `json:"path"`
	Documentation string                `json:"documentation"`
	Mutation      bool                  `json:"mutation"`
	ChargeBearing bool                  `json:"charge_bearing"`
	DryRun        bool                  `json:"dry_run"`
	Executed      bool                  `json:"executed"`
	Parameters    map[string]string     `json:"parameters,omitempty"`
	Response      *provider.APIResponse `json:"response,omitempty"`
}

func newAPICommand(state *state) *cobra.Command {
	command := &cobra.Command{
		Use:   "api",
		Short: "Call every official Namecheap API method",
		Args:  noArgs,
	}
	command.AddCommand(newAPIMethodsCommand(state))
	command.AddCommand(newAPIDescribeCommand(state))
	command.AddCommand(newAPICallCommand(state))
	for _, method := range apicatalog.Methods() {
		addCatalogCommand(command, method, state)
	}
	return command
}

func newAPIMethodsCommand(state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "methods",
		Short: "List the complete supported API catalog",
		Args:  noArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			methods := apicatalog.Methods()
			return state.printer().Data("api.methods", methods, func(w io.Writer) error {
				table := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
				if _, err := fmt.Fprintln(table, "COMMAND\tNAMECHEAP METHOD\tMODE"); err != nil {
					return err
				}
				for _, method := range methods {
					mode := "read"
					if method.ChargeBearing {
						mode = "charge"
					} else if method.Mutation {
						mode = "write"
					}
					if _, err := fmt.Fprintf(table, "api %s\t%s\t%s\n", method.CLIPath(), method.Name, mode); err != nil {
						return err
					}
				}
				return table.Flush()
			})
		},
	}
}

func newAPIDescribeCommand(state *state) *cobra.Command {
	return &cobra.Command{
		Use:   "describe <method-or-path>",
		Short: "Describe one API method and link its official documentation",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			method, ok := apicatalog.Find(args[0])
			if !ok {
				return unknownAPIMethod(args[0])
			}
			return state.printer().Data("api.describe", method, func(w io.Writer) error {
				mode := "read-only"
				if method.ChargeBearing {
					mode = "charge-bearing mutation"
				} else if method.Mutation {
					mode = "mutation"
				}
				_, err := fmt.Fprintf(w, "Command: cheep api %s\nMethod: %s\nMode: %s\nDocumentation: %s\n", method.CLIPath(), method.Name, mode, method.Documentation)
				return err
			})
		},
	}
}

func newAPICallCommand(state *state) *cobra.Command {
	flags := &apiCallFlags{}
	command := &cobra.Command{
		Use:   "call <method-or-path>",
		Short: "Call a catalog method by its API name or slash-separated path",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			method, ok := apicatalog.Find(args[0])
			if !ok {
				return unknownAPIMethod(args[0])
			}
			return runAPIMethod(cmd, state, method, flags)
		},
	}
	addAPICallFlags(command, flags)
	return command
}

func addCatalogCommand(root *cobra.Command, method apicatalog.Method, state *state) {
	parent := root
	for _, name := range method.Path[:len(method.Path)-1] {
		child := directSubcommand(parent, name)
		if child == nil {
			child = &cobra.Command{Use: name, Short: apiGroupDescription(append(commandPathBelow(root, parent), name)), Args: noArgs}
			parent.AddCommand(child)
		}
		parent = child
	}
	flags := &apiCallFlags{}
	leaf := &cobra.Command{
		Use:   method.Path[len(method.Path)-1],
		Short: method.Description,
		Long:  method.Description + ".\n\nNamecheap method: " + method.Name + "\nDocumentation: " + method.Documentation,
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAPIMethod(cmd, state, method, flags)
		},
	}
	addAPICallFlags(leaf, flags)
	parent.AddCommand(leaf)
}

func addAPICallFlags(command *cobra.Command, flags *apiCallFlags) {
	command.Flags().StringArrayVarP(&flags.parameters, "param", "p", nil, "Namecheap parameter as NAME=VALUE; repeat as needed")
	command.Flags().StringVar(&flags.paramsFile, "params-file", "", "JSON object of Namecheap parameters, or - for stdin")
	command.Flags().StringArrayVar(&flags.secretParams, "secret-param", nil, "secret parameter as NAME=ENV_VAR; repeat as needed")
	command.Flags().BoolVar(&flags.yes, "yes", false, "approve this remote mutation")
	command.Flags().BoolVar(&flags.production, "production", false, "confirm this mutation targets production")
	command.Flags().BoolVar(&flags.acceptCharge, "accept-charge", false, "acknowledge that this method can charge the account")
}

func runAPIMethod(command *cobra.Command, state *state, method apicatalog.Method, flags *apiCallFlags) error {
	params, secretNames, err := loadAPIParameters(flags, state)
	if err != nil {
		return err
	}
	service, profile, err := state.service(command.Context())
	if err != nil {
		return err
	}
	api, ok := service.(provider.API)
	if !ok {
		return failure.New("api_unavailable", exitcode.Unexpected, "the configured provider does not expose generic API calls")
	}
	result := apiCallResult{
		Method:        method.Name,
		Path:          method.CLIPath(),
		Documentation: method.Documentation,
		Mutation:      method.Mutation,
		ChargeBearing: method.ChargeBearing,
		DryRun:        state.dryRun,
	}

	if method.Mutation {
		if state.readOnly {
			return failure.New("readonly", exitcode.Safety, "--readonly refuses API mutations")
		}
		if state.dryRun {
			result.Parameters = redactedAPIParameters(params, secretNames)
			return writeAPICallResult(state, profile, result)
		}
		if !flags.yes {
			return failure.New("confirmation_required", exitcode.Safety, "API mutations require --yes after reviewing a --dry-run")
		}
		if profile.Environment == "production" && !flags.production {
			return failure.New("production_confirmation_required", exitcode.Safety, "production API mutations require --production")
		}
		if method.ChargeBearing && !flags.acceptCharge {
			return failure.New("charge_confirmation_required", exitcode.Price, "this method can charge the account; pass --accept-charge after checking current pricing")
		}
	}

	ctx, cancel := state.commandContext(command.Context())
	defer cancel()
	response, err := api.CallAPI(ctx, provider.APICall{Method: method.Name, Params: params, Mutation: method.Mutation})
	if err != nil {
		var providerError *provider.Error
		if errors.As(err, &providerError) && providerError.Kind == provider.ErrorOutcomeUnknown {
			return failure.Wrap("api_outcome_unknown", exitcode.Conflict, providerError.Message, err)
		}
		return mapProviderError(err)
	}
	result.Executed = true
	result.Response = &response
	return writeAPICallResult(state, profile, result)
}

func writeAPICallResult(state *state, profile config.Profile, result apiCallResult) error {
	return state.printer().DataWithMeta("api."+strings.ReplaceAll(result.Path, " ", "."), result, metaFor(profile), func(w io.Writer) error {
		if result.DryRun {
			if _, err := fmt.Fprintf(w, "DRY RUN: %s was not sent.\n", result.Method); err != nil {
				return err
			}
			return writeParameterTable(w, result.Parameters)
		}
		if result.Response == nil {
			return nil
		}
		content, err := json.MarshalIndent(result.Response, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(w, "%s\n", content)
		return err
	})
}

func loadAPIParameters(flags *apiCallFlags, state *state) (map[string]string, map[string]struct{}, error) {
	params := make(map[string]string)
	secretNames := make(map[string]struct{})
	if flags.paramsFile != "" {
		fileParams, err := readAPIParamsFile(flags.paramsFile, state.stdin)
		if err != nil {
			return nil, nil, err
		}
		for key, value := range fileParams {
			if err := addAPIParameter(params, key, value); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, assignment := range flags.parameters {
		key, value, err := splitAPIAssignment(assignment, "--param")
		if err != nil {
			return nil, nil, err
		}
		if err := addAPIParameter(params, key, value); err != nil {
			return nil, nil, err
		}
	}
	for _, assignment := range flags.secretParams {
		key, environmentName, err := splitAPIAssignment(assignment, "--secret-param")
		if err != nil {
			return nil, nil, err
		}
		value := state.resolver.Getenv(environmentName)
		if value == "" {
			return nil, nil, failure.New("missing_secret_parameter", exitcode.Authentication, fmt.Sprintf("environment variable %s is empty", environmentName))
		}
		if err := addAPIParameter(params, key, value); err != nil {
			return nil, nil, err
		}
		secretNames[strings.ToLower(key)] = struct{}{}
	}
	if len(params) > maximumAPIParameters {
		return nil, nil, failure.New("too_many_parameters", exitcode.Usage, fmt.Sprintf("API calls support at most %d parameters", maximumAPIParameters))
	}
	return params, secretNames, nil
}

func readAPIParamsFile(path string, stdin io.Reader) (map[string]string, error) {
	var reader io.Reader
	var file *os.File
	var err error
	if path == "-" {
		reader = stdin
	} else {
		file, err = os.Open(path)
		if err != nil {
			return nil, failure.Wrap("params_file_read_failed", exitcode.Usage, fmt.Sprintf("open parameters file: %v", err), err)
		}
		defer func() {
			_ = file.Close()
		}()
		reader = file
	}
	decoder := json.NewDecoder(bufio.NewReader(reader))
	decoder.UseNumber()
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, failure.Wrap("invalid_params_file", exitcode.Usage, "parameters file must contain one JSON object", err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return nil, failure.New("invalid_params_file", exitcode.Usage, "parameters file must contain one JSON object")
	}
	params := make(map[string]string, len(raw))
	for key, value := range raw {
		switch typed := value.(type) {
		case string:
			params[key] = typed
		case json.Number:
			params[key] = typed.String()
		case bool:
			params[key] = fmt.Sprint(typed)
		case nil:
			params[key] = ""
		default:
			return nil, failure.New("invalid_params_file", exitcode.Usage, fmt.Sprintf("parameter %s must be a string, number, boolean, or null", key))
		}
	}
	return params, nil
}

func splitAPIAssignment(assignment, flag string) (string, string, error) {
	key, value, ok := strings.Cut(assignment, "=")
	if !ok || strings.TrimSpace(key) == "" {
		return "", "", failure.New("invalid_parameter", exitcode.Usage, flag+" requires NAME=VALUE")
	}
	return strings.TrimSpace(key), value, nil
}

func addAPIParameter(params map[string]string, key, value string) error {
	if !apiParameterNamePattern.MatchString(key) {
		return failure.New("invalid_parameter", exitcode.Usage, fmt.Sprintf("invalid Namecheap parameter name: %q", key))
	}
	for _, reserved := range []string{"ApiUser", "ApiKey", "UserName", "Username", "ClientIp", "Command"} {
		if strings.EqualFold(key, reserved) {
			return failure.New("reserved_parameter", exitcode.Safety, fmt.Sprintf("%s is supplied by the selected Cheep profile", key))
		}
	}
	for existing := range params {
		if strings.EqualFold(existing, key) {
			return failure.New("duplicate_parameter", exitcode.Usage, fmt.Sprintf("parameter %s was provided more than once", key))
		}
	}
	params[key] = value
	return nil
}

func redactedAPIParameters(params map[string]string, explicitSecrets map[string]struct{}) map[string]string {
	result := make(map[string]string, len(params))
	for key, value := range params {
		_, explicit := explicitSecrets[strings.ToLower(key)]
		if explicit || sensitiveAPIParameterName(key) {
			result[key] = "[REDACTED]"
		} else {
			result[key] = value
		}
	}
	return result
}

func sensitiveAPIParameterName(name string) bool {
	normalized := strings.ToLower(name)
	for _, part := range []string{"password", "eppcode", "authorizationcode", "csr", "cardnumber", "cvv", "securitycode"} {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

func writeParameterTable(w io.Writer, params map[string]string) error {
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if _, err := fmt.Fprintf(w, "%s=%s\n", key, params[key]); err != nil {
			return err
		}
	}
	return nil
}

func directSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func commandPathBelow(root, command *cobra.Command) []string {
	if command == root {
		return nil
	}
	path := strings.TrimSpace(strings.TrimPrefix(command.CommandPath(), root.CommandPath()))
	return strings.Fields(path)
}

func apiGroupDescription(path []string) string {
	return "Access Namecheap " + strings.Join(path, " ") + " methods"
}

func unknownAPIMethod(value string) error {
	return failure.New("unknown_api_method", exitcode.Usage, fmt.Sprintf("unknown Namecheap API method or path: %s", value))
}
