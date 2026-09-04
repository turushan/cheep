package namecheap

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/turushan/cheep/internal/config"
	"github.com/turushan/cheep/internal/provider"
)

func TestCallAPIUsesAuthenticatedSandboxPOSTAndPreservesXML(t *testing.T) {
	t.Parallel()

	client := testClient(roundTripFunc(func(request *http.Request) (*http.Response, error) {
		values := readForm(t, request)
		if request.URL.Host != "api.sandbox.namecheap.com" {
			t.Fatalf("host = %s", request.URL.Host)
		}
		if values.Get("Command") != "namecheap.domains.getContacts" || values.Get("DomainName") != "example.com" {
			t.Fatalf("unexpected request: %#v", values)
		}
		return xmlResponse(`<RequestedCommand>namecheap.domains.getContacts</RequestedCommand><Warnings><Warning Number="12">Notice</Warning></Warnings><CommandResponse Type="namecheap.domains.getContacts"><DomainContactsResult Domain="example.com"><Registrant FirstName="Ada"/><Tech FirstName="Grace"/></DomainContactsResult></CommandResponse><ExecutionTime>0.123</ExecutionTime>`), nil
	}), config.Sandbox, "test-key")

	response, err := client.CallAPI(context.Background(), provider.APICall{
		Method: "namecheap.domains.getContacts",
		Params: map[string]string{"DomainName": "example.com"},
	})
	if err != nil {
		t.Fatalf("CallAPI returned an error: %v", err)
	}
	if response.Status != "OK" || response.RequestedCommand != "namecheap.domains.getContacts" || response.ExecutionTime != "0.123" {
		t.Fatalf("unexpected response metadata: %+v", response)
	}
	if len(response.Warnings) != 1 || response.Warnings[0].Number != "12" {
		t.Fatalf("unexpected warnings: %+v", response.Warnings)
	}
	if response.Response.Name != "CommandResponse" || response.Response.Attributes["Type"] == "" || len(response.Response.Children) != 1 {
		t.Fatalf("unexpected response tree: %+v", response.Response)
	}
}

func TestCallAPIMutationDoesNotRetryAmbiguousOutcome(t *testing.T) {
	t.Parallel()

	calls := 0
	client := testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return nil, errors.New("connection ended before a response arrived")
	}), config.Sandbox, "test-key")

	_, err := client.CallAPI(context.Background(), provider.APICall{
		Method:   "namecheap.domains.create",
		Params:   map[string]string{"DomainName": "example.com"},
		Mutation: true,
	})
	var providerError *provider.Error
	if !errors.As(err, &providerError) || providerError.Kind != provider.ErrorOutcomeUnknown {
		t.Fatalf("unexpected error: %#v", err)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
}

func TestCallAPIRedactsSecretFromProviderError(t *testing.T) {
	t.Parallel()

	const password = "private-password"
	client := testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return xmlResponse(`<Errors><Error Number="2010404">Invalid password ` + password + `</Error></Errors><CommandResponse/>`), nil
	}), config.Sandbox, "test-key")

	_, err := client.CallAPI(context.Background(), provider.APICall{
		Method: "namecheap.users.login",
		Params: map[string]string{"Password": password},
	})
	if err == nil || strings.Contains(err.Error(), password) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("secret was not redacted: %v", err)
	}
}

func TestCallAPIMissingResponseClassifiesMutationAsUnknown(t *testing.T) {
	t.Parallel()
	for _, mutation := range []bool{false, true} {
		for _, body := range []string{`<ExecutionTime>0.123</ExecutionTime>`, `<CommandResponse Type="namecheap.domains.renew"/>`} {
			calls := 0
			client := testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return xmlResponse(body), nil
			}), config.Sandbox, "test-key")
			_, err := client.CallAPI(context.Background(), provider.APICall{Method: "namecheap.domains.renew", Mutation: mutation})
			want := provider.ErrorProvider
			if mutation {
				want = provider.ErrorOutcomeUnknown
			}
			var problem *provider.Error
			if !errors.As(err, &problem) || problem.Kind != want || calls != 1 {
				t.Fatalf("mutation=%t: %v, calls=%d", mutation, err, calls)
			}
		}
	}
}

func TestCallAPIRedactsExplicitSecretWithUnrecognizedName(t *testing.T) {
	t.Parallel()
	const secret = "private-customer-token"
	client := testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return xmlResponse(`<Errors><Error Number="2010404">Invalid token ` + secret + `</Error></Errors><CommandResponse/>`), nil
	}), config.Sandbox, "test-key")
	_, err := client.CallAPI(context.Background(), provider.APICall{
		Method: "namecheap.users.login", Params: map[string]string{"Token": secret}, SecretParams: []string{"token"},
	})
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("explicit secret exposed: %v", err)
	}
}
