package namecheap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"

	sdk "github.com/namecheap/go-namecheap-sdk/v2/namecheap"
	"github.com/turushan/nccli/internal/config"
	"github.com/turushan/nccli/internal/provider"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func TestCheckDomainsUsesSandboxPOSTAndChunksRequests(t *testing.T) {
	t.Parallel()

	const secret = "must-not-appear-in-url"
	var mu sync.Mutex
	calls := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", request.Method)
		}
		if request.URL.Host != "api.sandbox.namecheap.com" {
			t.Fatalf("host = %s, want sandbox", request.URL.Host)
		}
		if strings.Contains(request.URL.String(), secret) || request.URL.RawQuery != "" {
			t.Fatalf("request URL exposed credentials: %s", request.URL.Redacted())
		}
		values := readForm(t, request)
		if values.Get("ApiKey") != secret {
			t.Fatal("request body did not contain the expected credential")
		}
		domains := strings.Split(values.Get("DomainList"), ",")
		if len(domains) > checkBatchSize {
			t.Fatalf("batch size = %d, want no more than %d", len(domains), checkBatchSize)
		}
		var results strings.Builder
		for _, domain := range domains {
			fmt.Fprintf(&results, `<DomainCheckResult Domain="%s" Available="true" IsPremiumName="false"/>`, domain)
		}
		mu.Lock()
		calls++
		mu.Unlock()
		return xmlResponse(`<CommandResponse>` + results.String() + `</CommandResponse>`), nil
	})

	client := testClient(transport, config.Sandbox, secret)
	domains := make([]string, 51)
	for i := range domains {
		domains[i] = fmt.Sprintf("example-%d.com", i)
	}
	checks, err := client.CheckDomains(context.Background(), domains)
	if err != nil {
		t.Fatalf("CheckDomains returned an error: %v", err)
	}
	if len(checks) != len(domains) {
		t.Fatalf("results = %d, want %d", len(checks), len(domains))
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestReadOnlyModelConversions(t *testing.T) {
	t.Parallel()

	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		values := readForm(t, request)
		switch values.Get("Command") {
		case "namecheap.domains.getList":
			return xmlResponse(`<CommandResponse><DomainGetListResult><Domain ID="42" Name="example.com" User="maker" Created="1/2/2025" Expires="1/2/2027" IsExpired="false" IsLocked="true" AutoRenew="true" WhoisGuard="ENABLED" IsPremium="false" IsOurDNS="true"/></DomainGetListResult><Paging><TotalItems>1</TotalItems><CurrentPage>1</CurrentPage><PageSize>100</PageSize></Paging></CommandResponse>`), nil
		case "namecheap.domains.getInfo":
			return xmlResponse(`<CommandResponse><DomainGetInfoResult Status="Locked" ID="42" DomainName="example.com" OwnerName="maker" IsOwner="true" IsPremium="false"><DomainDetails><CreatedDate>1/2/2025</CreatedDate><ExpiredDate>1/2/2027</ExpiredDate><NumYears>2</NumYears></DomainDetails><Whoisguard Enabled="True"><ExpiredDate>1/2/2027</ExpiredDate></Whoisguard><DnsDetails ProviderType="FREE" IsUsingOurDNS="true" HostCount="2" EmailType="NONE" DynamicDNSStatus="false"><Nameserver>dns1.registrar-servers.com</Nameserver></DnsDetails><Modificationrights><Rights Type="dns">OK</Rights></Modificationrights></DomainGetInfoResult></CommandResponse>`), nil
		case "namecheap.users.getBalances":
			return xmlResponse(`<CommandResponse><UserGetBalancesResult Currency="USD" AvailableBalance="50.25" AccountBalance="50.25" EarnedAmount="1.00" WithdrawableAmount="0.50" FundsRequiredForAutoRenew="9.99"/></CommandResponse>`), nil
		case "namecheap.users.getPricing":
			return xmlResponse(`<CommandResponse><UserGetPricingResult><ProductType Name="domains"><ProductCategory Name="register"><Product Name="com"><Price Duration="1" DurationType="YEAR" Price="8.88" RegularPrice="12.98" YourPrice="9.48" Currency="USD" PromotionPrice="8.88"/></Product></ProductCategory></ProductType></UserGetPricingResult></CommandResponse>`), nil
		case "namecheap.domains.getTldList":
			return xmlResponse(`<CommandResponse><Tlds><Tld Name="com" NonRealTimeDomain="false" MinRegisterYears="1" MaxRegisterYears="10" MinRenewYears="1" MaxRenewYears="10" MinTransferYears="1" MaxTransferYears="1" IsApiRegisterable="true" IsApiRenewable="true" IsApiTransferable="true" IsEppRequired="true"/></Tlds></CommandResponse>`), nil
		default:
			t.Fatalf("unexpected Namecheap command: %s", values.Get("Command"))
			return nil, nil
		}
	})
	client := testClient(transport, config.Sandbox, "test-key")
	ctx := context.Background()

	probe, err := client.Probe(ctx)
	if err != nil || probe.DomainCount != 1 {
		t.Fatalf("Probe = %+v, %v", probe, err)
	}
	domains, err := client.ListDomains(ctx, provider.DomainListFilter{ListType: "all", Sort: "NAME"})
	if err != nil || len(domains) != 1 || domains[0].Name != "example.com" || domains[0].Expires == nil {
		t.Fatalf("ListDomains = %+v, %v", domains, err)
	}
	info, err := client.DomainInfo(ctx, "example.com")
	if err != nil || info.ID != 42 || len(info.DNS.Nameservers) != 1 || info.Rights["dns"] != "OK" {
		t.Fatalf("DomainInfo = %+v, %v", info, err)
	}
	balance, err := client.Balance(ctx)
	if err != nil || balance.Available != "50.25" || balance.Currency != "USD" {
		t.Fatalf("Balance = %+v, %v", balance, err)
	}
	price, err := client.Price(ctx, provider.PriceRequest{TLD: ".com", Action: "register", Years: 1})
	if err != nil || price.Effective != "8.88" || price.Regular != "12.98" {
		t.Fatalf("Price = %+v, %v", price, err)
	}
	tlds, err := client.ListTLDs(ctx)
	if err != nil || len(tlds) != 1 || tlds[0].Name != "com" || !isTrue(tlds[0].APIRegisterable) {
		t.Fatalf("ListTLDs = %+v, %v", tlds, err)
	}
}

func TestAPIErrorIsTypedWithoutCredentialExposure(t *testing.T) {
	t.Parallel()

	const secret = "super-secret-test-key"
	client := testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`<?xml version="1.0"?><ApiResponse Status="ERROR"><Errors><Error Number="4011103">Access denied</Error></Errors><CommandResponse/></ApiResponse>`)),
		}, nil
	}), config.Production, secret)

	_, err := client.CheckDomains(context.Background(), []string{"example.com"})
	if err == nil {
		t.Fatal("expected an API error")
	}
	providerError, ok := err.(*provider.Error)
	if !ok || providerError.Kind != provider.ErrorAuth || providerError.ProviderCode != 4011103 {
		t.Fatalf("unexpected error: %#v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("error exposed API key")
	}
}

func TestRequestIPErrorIsAuthenticationFailure(t *testing.T) {
	t.Parallel()

	client := testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`<?xml version="1.0"?><ApiResponse Status="ERROR"><Errors><Error Number="1011150">Invalid request IP: 203.0.113.10</Error></Errors><CommandResponse/></ApiResponse>`)),
		}, nil
	}), config.Sandbox, "test-key")

	_, err := client.Probe(context.Background())
	providerError, ok := err.(*provider.Error)
	if !ok || providerError.Kind != provider.ErrorAuth {
		t.Fatalf("unexpected error: %#v", err)
	}
	if strings.Contains(providerError.Message, "203.0.113.10") {
		t.Fatalf("message should not echo the request IP: %s", providerError.Message)
	}
}

func TestDNSPlanAndApplyUseWholeZoneVerifiedFlow(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	currentValue := "192.0.2.1"
	setCalls := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		values := readForm(t, request)
		mu.Lock()
		defer mu.Unlock()
		switch values.Get("Command") {
		case "namecheap.domains.dns.getHosts":
			body := fmt.Sprintf(`<CommandResponse><DomainDNSGetHostsResult Domain="example.com" EmailType="NONE" IsUsingOurDNS="true"><host HostId="1" Name="@" Type="A" Address="%s" MXPref="10" TTL="300" IsActive="true"/></DomainDNSGetHostsResult></CommandResponse>`, currentValue)
			return xmlResponse(body), nil
		case "namecheap.domains.dns.setHosts":
			if values.Get("HostName1") != "@" || values.Get("RecordType1") != "A" {
				t.Fatalf("unexpected setHosts record host=%q type=%q", values.Get("HostName1"), values.Get("RecordType1"))
			}
			currentValue = values.Get("Address1")
			setCalls++
			return xmlResponse(`<CommandResponse><DomainDNSSetHostsResult Domain="example.com" IsSuccess="true"/></CommandResponse>`), nil
		default:
			t.Fatalf("unexpected Namecheap command: %s", values.Get("Command"))
			return nil, nil
		}
	})
	client := testClient(transport, config.Sandbox, "test-key")
	ctx := context.Background()

	zone, err := client.GetZone(ctx, "example.com")
	if err != nil || len(zone.Records) != 1 || zone.Records[0].Value != "192.0.2.1" || zone.Records[0].MXPref != nil {
		t.Fatalf("GetZone = %+v, %v", zone, err)
	}
	zone.Records[0].Value = "192.0.2.2"
	plan, err := client.PlanZone(ctx, zone)
	if err != nil || !plan.Satisfiable || len(plan.Add) != 1 || len(plan.Remove) != 1 {
		t.Fatalf("PlanZone = %+v, %v", plan, err)
	}
	change, err := client.ApplyZone(ctx, zone, plan.CurrentFingerprint)
	if err != nil {
		t.Fatalf("ApplyZone returned an error: %v", err)
	}
	if change.Added != 1 || change.Removed != 1 || len(change.Records) != 1 {
		t.Fatalf("unexpected change: %+v", change)
	}
	mu.Lock()
	defer mu.Unlock()
	if setCalls != 1 || currentValue != "192.0.2.2" {
		t.Fatalf("set calls = %d, current value = %s", setCalls, currentValue)
	}
}

func TestDNSApplyRefusesChangedFingerprintWithoutWriting(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	currentValue := "192.0.2.1"
	setCalls := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		values := readForm(t, request)
		mu.Lock()
		defer mu.Unlock()
		switch values.Get("Command") {
		case "namecheap.domains.dns.getHosts":
			return xmlResponse(fmt.Sprintf(`<CommandResponse><DomainDNSGetHostsResult Domain="example.com" EmailType="NONE" IsUsingOurDNS="true"><host HostId="1" Name="@" Type="A" Address="%s" TTL="300" IsActive="true"/></DomainDNSGetHostsResult></CommandResponse>`, currentValue)), nil
		case "namecheap.domains.dns.setHosts":
			setCalls++
			return xmlResponse(`<CommandResponse><DomainDNSSetHostsResult Domain="example.com" IsSuccess="true"/></CommandResponse>`), nil
		default:
			t.Fatalf("unexpected Namecheap command: %s", values.Get("Command"))
			return nil, nil
		}
	})
	client := testClient(transport, config.Sandbox, "test-key")
	desired := provider.Zone{
		Version:   provider.ZoneFileVersion,
		Domain:    "example.com",
		EmailType: "NONE",
		Records:   []provider.DNSRecord{{Host: "@", Type: "A", Value: "192.0.2.2", TTL: 300}},
	}
	plan, err := client.PlanZone(context.Background(), desired)
	if err != nil {
		t.Fatalf("PlanZone returned an error: %v", err)
	}
	mu.Lock()
	currentValue = "192.0.2.99"
	mu.Unlock()
	_, err = client.ApplyZone(context.Background(), desired, plan.CurrentFingerprint)
	var providerError *provider.Error
	if !errors.As(err, &providerError) || providerError.Kind != provider.ErrorConflict {
		t.Fatalf("unexpected error: %#v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if setCalls != 0 {
		t.Fatalf("setHosts calls = %d, want 0", setCalls)
	}
}

func TestDNSApplyDoesNotRetryUnknownSetHostsOutcome(t *testing.T) {
	t.Parallel()

	setCalls := 0
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		values := readForm(t, request)
		switch values.Get("Command") {
		case "namecheap.domains.dns.getHosts":
			return xmlResponse(`<CommandResponse><DomainDNSGetHostsResult Domain="example.com" EmailType="NONE" IsUsingOurDNS="true"><host HostId="1" Name="@" Type="A" Address="192.0.2.1" TTL="300" IsActive="true"/></DomainDNSGetHostsResult></CommandResponse>`), nil
		case "namecheap.domains.dns.setHosts":
			setCalls++
			return nil, errors.New("connection ended before a response arrived")
		default:
			t.Fatalf("unexpected Namecheap command: %s", values.Get("Command"))
			return nil, nil
		}
	})
	client := testClient(transport, config.Sandbox, "test-key")
	desired := provider.Zone{
		Version:   provider.ZoneFileVersion,
		Domain:    "example.com",
		EmailType: "NONE",
		Records:   []provider.DNSRecord{{Host: "@", Type: "A", Value: "192.0.2.2", TTL: 300}},
	}
	current, err := client.GetZone(context.Background(), desired.Domain)
	if err != nil {
		t.Fatalf("GetZone returned an error: %v", err)
	}
	_, err = client.ApplyZone(context.Background(), desired, provider.FingerprintZone(current))
	var providerError *provider.Error
	if !errors.As(err, &providerError) || providerError.Kind != provider.ErrorOutcomeUnknown {
		t.Fatalf("unexpected error: %#v", err)
	}
	if setCalls != 1 {
		t.Fatalf("setHosts calls = %d, want 1", setCalls)
	}
}

func TestGetZoneRefusesOutOfRangeMXPreference(t *testing.T) {
	t.Parallel()

	client := testClient(roundTripFunc(func(*http.Request) (*http.Response, error) {
		return xmlResponse(`<CommandResponse><DomainDNSGetHostsResult Domain="example.com" EmailType="MX" IsUsingOurDNS="true"><host HostId="1" Name="@" Type="MX" Address="mail.example.com" MXPref="256" TTL="300" IsActive="true"/></DomainDNSGetHostsResult></CommandResponse>`), nil
	}), config.Sandbox, "test-key")
	_, err := client.GetZone(context.Background(), "example.com")
	var providerError *provider.Error
	if !errors.As(err, &providerError) || providerError.Kind != provider.ErrorProvider {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestConvertErrorPrefersAuthenticationInJoinedAPIErrors(t *testing.T) {
	t.Parallel()

	client := &Client{apiKey: "test-key"}
	err := errors.Join(
		&sdk.APIError{Number: 2019166, Message: "domain not found"},
		&sdk.APIError{Number: 1011150, Message: "invalid request IP"},
	)
	converted := client.convertError(err)
	var providerError *provider.Error
	if !errors.As(converted, &providerError) || providerError.Kind != provider.ErrorAuth || providerError.ProviderCode != 1011150 {
		t.Fatalf("unexpected error: %#v", converted)
	}
}

func TestConvertErrorKeepsOperatorCancellationDistinct(t *testing.T) {
	t.Parallel()

	client := &Client{apiKey: "test-key"}
	converted := client.convertError(context.Canceled)
	var providerError *provider.Error
	if !errors.As(converted, &providerError) || providerError.Kind != provider.ErrorCanceled {
		t.Fatalf("unexpected error: %#v", converted)
	}
}

func testClient(transport http.RoundTripper, environment config.Environment, key string) *Client {
	reader := Factory{Version: "test", Transport: transport}.New(config.Profile{
		APIUser:     "maker",
		Username:    "maker",
		APIKey:      key,
		ClientIP:    "8.8.8.8",
		Environment: environment,
	})
	return reader.(*Client)
}

func readForm(t *testing.T, request *http.Request) url.Values {
	t.Helper()
	content, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	if err := request.Body.Close(); err != nil {
		t.Fatalf("close request body: %v", err)
	}
	values, err := url.ParseQuery(string(content))
	if err != nil {
		t.Fatalf("parse request body: %v", err)
	}
	return values
}

func xmlResponse(commandResponse string) *http.Response {
	body := `<?xml version="1.0"?><ApiResponse Status="OK">` + commandResponse + `</ApiResponse>`
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func isTrue(value *bool) bool {
	return value != nil && *value
}
