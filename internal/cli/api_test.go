package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/turushan/cheep/internal/apicatalog"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/output"
	"github.com/turushan/cheep/internal/provider"
)

type fakeAPIService struct {
	*fakeReader
	calls    int
	request  provider.APICall
	response provider.APIResponse
	err      error
}

func (f *fakeAPIService) CallAPI(_ context.Context, request provider.APICall) (provider.APIResponse, error) {
	f.calls++
	f.request = request
	return f.response, f.err
}

func TestAPISchemaContainsEveryCatalogLeaf(t *testing.T) {
	t.Parallel()

	root, _ := newRoot(Options{})
	schema := buildSchema(root)
	found := make(map[string]struct{})
	for _, command := range schema.Commands {
		found[command.Name] = struct{}{}
	}
	for _, method := range apicatalog.Methods() {
		name := "api." + strings.ReplaceAll(method.CLIPath(), " ", ".")
		if _, ok := found[name]; !ok {
			t.Fatalf("schema is missing %s", name)
		}
	}
}

func TestAPIReadMethodExecutesWithStableJSON(t *testing.T) {
	t.Parallel()

	service := &fakeAPIService{
		fakeReader: &fakeReader{},
		response: provider.APIResponse{
			Method: "namecheap.domains.getContacts",
			Status: "OK",
			Response: provider.XMLElement{
				Name:       "CommandResponse",
				Attributes: map[string]string{"Type": "namecheap.domains.getContacts"},
			},
		},
	}
	options, stdout, stderr := authenticatedOptions(t, service)
	status := Execute([]string{"api", "domains", "get-contacts", "--param", "DomainName=example.com", "--json"}, options)
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	if service.calls != 1 || service.request.Method != "namecheap.domains.getContacts" || service.request.Params["DomainName"] != "example.com" || service.request.Mutation {
		t.Fatalf("unexpected API call: %+v", service.request)
	}
	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout.String()), &envelope); err != nil || envelope.Command != "api.domains.get-contacts" || !envelope.OK {
		t.Fatalf("unexpected JSON result: %+v, %v", envelope, err)
	}
}

func TestAPIMutationDryRunRedactsSecretsAndNeverCallsProvider(t *testing.T) {
	t.Parallel()

	service := &fakeAPIService{fakeReader: &fakeReader{}}
	options, stdout, stderr := authenticatedOptions(t, service)
	status := Execute([]string{
		"--dry-run", "api", "domains", "transfer", "create",
		"--param", "DomainName=example.com",
		"--secret-param", "EPPCode=CHEEP_API_KEY",
		"--json",
	}, options)
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	if service.calls != 0 {
		t.Fatalf("provider calls = %d, want 0", service.calls)
	}
	if strings.Contains(stdout.String(), "test-key") || !strings.Contains(stdout.String(), "[REDACTED]") {
		t.Fatalf("unexpected dry-run output: %s", stdout.String())
	}
}

func TestAPIChargeMethodRequiresEverySafetyGate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantText   string
	}{
		{name: "approval", args: nil, wantStatus: exitcode.Safety, wantText: "--yes"},
		{name: "production", args: []string{"--yes"}, wantStatus: exitcode.Safety, wantText: "--production"},
		{name: "charge", args: []string{"--yes", "--production"}, wantStatus: exitcode.Price, wantText: "--accept-charge"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			service := &fakeAPIService{fakeReader: &fakeReader{}}
			options, _, stderr := authenticatedOptions(t, service)
			args := []string{"--environment", "production", "api", "domains", "create", "--param", "DomainName=example.com"}
			args = append(args, test.args...)
			status := Execute(args, options)
			if status != test.wantStatus || !strings.Contains(stderr.String(), test.wantText) {
				t.Fatalf("status = %d, stderr = %q", status, stderr.String())
			}
			if service.calls != 0 {
				t.Fatalf("provider calls = %d, want 0", service.calls)
			}
		})
	}
}

func TestAPIChargeMethodCanExecuteAfterSafetyGates(t *testing.T) {
	t.Parallel()

	service := &fakeAPIService{
		fakeReader: &fakeReader{},
		response: provider.APIResponse{
			Method:   "namecheap.domains.create",
			Status:   "OK",
			Response: provider.XMLElement{Name: "CommandResponse"},
		},
	}
	options, _, stderr := authenticatedOptions(t, service)
	status := Execute([]string{
		"--environment", "production", "api", "domains", "create",
		"--param", "DomainName=example.com",
		"--yes", "--production", "--accept-charge",
	}, options)
	if status != exitcode.Success {
		t.Fatalf("status = %d; stderr: %s", status, stderr.String())
	}
	if service.calls != 1 || !service.request.Mutation {
		t.Fatalf("unexpected API call: %+v", service.request)
	}
}

func TestAPIRefusesCredentialAndCommandOverrides(t *testing.T) {
	t.Parallel()

	for _, parameter := range []string{"ApiKey=bad", "Command=namecheap.users.getBalances", "clientip=1.2.3.4"} {
		service := &fakeAPIService{fakeReader: &fakeReader{}}
		options, _, stderr := authenticatedOptions(t, service)
		status := Execute([]string{"api", "domains", "get-info", "--param", parameter}, options)
		if status != exitcode.Safety || !strings.Contains(stderr.String(), "selected Cheep profile") {
			t.Fatalf("parameter = %s, status = %d, stderr = %q", parameter, status, stderr.String())
		}
	}
}

func TestAPIReadDryRunReportsExecutedResponse(t *testing.T) {
	t.Parallel()
	for _, jsonMode := range []bool{false, true} {
		service := &fakeAPIService{fakeReader: &fakeReader{}, response: provider.APIResponse{Status: "OK"}}
		options, stdout, stderr := authenticatedOptions(t, service)
		args := []string{"--dry-run", "api", "domains", "get-info", "-p", "DomainName=example.com"}
		if jsonMode {
			args = append(args, "--json")
		}
		if status := Execute(args, options); status != exitcode.Success || service.calls != 1 {
			t.Fatalf("read was not executed: %d, %s", status, stderr.String())
		}
		if strings.Contains(stdout.String(), "was not sent") || !strings.Contains(stdout.String(), `"status":`) {
			t.Fatalf("read output misrepresents execution: %s", stdout.String())
		}
		if jsonMode && (!strings.Contains(stdout.String(), `"dry_run":false`) || !strings.Contains(stdout.String(), `"executed":true`)) {
			t.Fatalf("incorrect execution metadata: %s", stdout.String())
		}
	}
}

func TestAPIExplicitSecretNamesReachProvider(t *testing.T) {
	t.Parallel()
	service := &fakeAPIService{fakeReader: &fakeReader{}}
	options, _, stderr := authenticatedOptions(t, service)
	status := Execute([]string{"api", "domains", "get-info", "--secret-param", "DomainName=CHEEP_API_KEY"}, options)
	if status != exitcode.Success || len(service.request.SecretParams) != 1 || service.request.SecretParams[0] != "domainname" {
		t.Fatalf("secret metadata lost: %+v; %s", service.request, stderr.String())
	}
}

func TestAPIParametersFileRejectsAmbiguousAndUnboundedInput(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		`null`, `[]`, `{"Years":1,"Years":10}`, `{"Years":1,"years":10}`,
		`{"Years":1} {}`, `{"Years":1`, `{"Years":[1]}`, `{"ApiKey":"override"}`,
		`{"Notes":"` + strings.Repeat("x", maximumAPIParamsFileBytes) + `"}`,
	} {
		if _, err := readAPIParamsFile("-", strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid parameter document (%d bytes)", len(input))
		}
	}
	params, err := readAPIParamsFile("-", strings.NewReader(`{"Years":2,"Enabled":true,"Coupon":null,"DomainName":"example.com","Amount":1.25}`))
	if err != nil || params["Years"] != "2" || params["Enabled"] != "true" || params["Coupon"] != "" || params["Amount"] != "1.25" {
		t.Fatalf("scalar conversion failed: %#v, %v", params, err)
	}
}
