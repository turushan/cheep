package namecheap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"

	sdk "github.com/namecheap/go-namecheap-sdk/v2/namecheap"
	"github.com/turushan/cheep/internal/config"
	"github.com/turushan/cheep/internal/provider"
)

const checkBatchSize = 50

var authenticationErrorCodes = map[int]struct{}{
	1010101: {},
	1010102: {},
	1010105: {},
	1011102: {},
	1011105: {},
	1011150: {},
	1016103: {},
	1017101: {},
	1017103: {},
	1017105: {},
	1017150: {},
	1017410: {},
	1017411: {},
	1019103: {},
	1030408: {},
	1050900: {},
}

// Factory constructs official Namecheap SDK clients behind the provider interface.
type Factory struct {
	Version    string
	HTTPClient *http.Client
	Transport  http.RoundTripper
}

func (f Factory) New(profile config.Profile) provider.Service {
	options := sdk.ClientOptions{
		UserName:   profile.Username,
		ApiUser:    profile.APIUser,
		ApiKey:     profile.APIKey,
		ClientIp:   profile.ClientIP,
		UseSandbox: profile.Environment == config.Sandbox,
		UserAgent:  "cheep/" + f.Version,
		HTTPClient: f.HTTPClient,
		Transport:  f.Transport,
	}
	client := sdk.NewClient(&options)
	writeOptions := options
	writeOptions.Retry = &sdk.RetryOptions{MaxAttempts: 1}
	writer := sdk.NewClient(&writeOptions)
	return &Client{sdk: client, writer: writer, apiKey: profile.APIKey}
}

// Client translates SDK models into Cheep's stable internal models.
type Client struct {
	sdk    *sdk.Client
	writer *sdk.Client
	apiKey string
}

func (c *Client) Probe(ctx context.Context) (provider.Probe, error) {
	response, err := c.sdk.Domains.GetListWithContext(ctx, &sdk.DomainsGetListArgs{
		Page:     sdk.Int(1),
		PageSize: sdk.Int(10),
	})
	if err != nil {
		return provider.Probe{}, c.convertError(err)
	}
	if response == nil {
		return provider.Probe{}, responseError("domains list probe returned no response")
	}
	total := 0
	if response.Paging != nil && response.Paging.TotalItems != nil {
		total = *response.Paging.TotalItems
	} else if response.Domains != nil {
		total = len(*response.Domains)
	}
	return provider.Probe{DomainCount: total}, nil
}

func (c *Client) ListDomains(ctx context.Context, filter provider.DomainListFilter) ([]provider.Domain, error) {
	args := &sdk.DomainsGetListArgs{}
	if filter.ListType != "" {
		args.ListType = sdk.String(strings.ToUpper(filter.ListType))
	}
	if filter.Search != "" {
		args.SearchTerm = sdk.String(filter.Search)
	}
	if filter.Sort != "" {
		args.SortBy = sdk.String(strings.ToUpper(filter.Sort))
	}
	domains, err := c.sdk.Domains.ListAllSlice(ctx, args)
	if err != nil {
		return nil, c.convertError(err)
	}
	result := make([]provider.Domain, 0, len(domains))
	for _, domain := range domains {
		if domain == nil {
			continue
		}
		result = append(result, convertDomain(domain))
	}
	return result, nil
}

func (c *Client) DomainInfo(ctx context.Context, domain string) (provider.DomainInfo, error) {
	response, err := c.sdk.Domains.GetInfoWithContext(ctx, domain)
	if err != nil {
		return provider.DomainInfo{}, c.convertError(err)
	}
	if response == nil || response.Result() == nil {
		return provider.DomainInfo{}, responseError("domain info returned no result")
	}
	return convertDomainInfo(response.Result()), nil
}

func (c *Client) CheckDomains(ctx context.Context, domains []string) ([]provider.DomainCheck, error) {
	if len(domains) == 0 {
		return nil, &provider.Error{Kind: provider.ErrorInvalid, Message: "at least one domain is required"}
	}
	result := make([]provider.DomainCheck, 0, len(domains))
	for start := 0; start < len(domains); start += checkBatchSize {
		end := start + checkBatchSize
		if end > len(domains) {
			end = len(domains)
		}
		response, err := c.sdk.Domains.CheckWithContext(ctx, domains[start:end]...)
		if err != nil {
			return nil, c.convertError(err)
		}
		if response == nil || response.DomainCheckResults == nil {
			return nil, responseError("domain check returned no results")
		}
		for i := range *response.DomainCheckResults {
			result = append(result, convertDomainCheck((*response.DomainCheckResults)[i]))
		}
	}
	return result, nil
}

func (c *Client) Balance(ctx context.Context) (provider.Balance, error) {
	response, err := c.sdk.Users.GetBalancesWithContext(ctx)
	if err != nil {
		return provider.Balance{}, c.convertError(err)
	}
	if response == nil || response.UserGetBalancesResult == nil {
		return provider.Balance{}, responseError("account balance returned no result")
	}
	value := response.UserGetBalancesResult
	return provider.Balance{
		Currency:             value.Currency,
		Available:            value.AvailableBalance.String(),
		Account:              value.AccountBalance.String(),
		Earned:               value.EarnedAmount.String(),
		Withdrawable:         value.WithdrawableAmount.String(),
		RequiredForAutoRenew: value.FundsRequiredForAutoRenew.String(),
	}, nil
}

func (c *Client) Price(ctx context.Context, request provider.PriceRequest) (provider.Price, error) {
	action := strings.ToUpper(request.Action)
	tld := strings.TrimPrefix(strings.ToLower(request.TLD), ".")
	response, err := c.sdk.Users.GetPricingWithContext(ctx, &sdk.UsersGetPricingArgs{
		ProductType: sdk.String("DOMAIN"),
		ActionName:  sdk.String(action),
		ProductName: sdk.String(tld),
	})
	if err != nil {
		return provider.Price{}, c.convertError(err)
	}
	if response == nil || response.UserGetPricingResult == nil {
		return provider.Price{}, responseError("domain pricing returned no result")
	}
	price, ok := response.UserGetPricingResult.PriceFor(action, tld, request.Years)
	if !ok {
		return provider.Price{}, &provider.Error{
			Kind:    provider.ErrorNotFound,
			Message: fmt.Sprintf("no %s price found for .%s at %d year(s)", strings.ToLower(action), tld, request.Years),
		}
	}
	return provider.Price{
		TLD:          tld,
		Action:       strings.ToLower(action),
		Years:        price.Duration,
		DurationType: strings.ToLower(price.DurationType),
		Currency:     price.Currency,
		Effective:    price.EffectivePrice().String(),
		Regular:      price.RegularPrice.String(),
		Account:      price.YourPrice.String(),
		Promotion:    price.PromotionPrice.String(),
	}, nil
}

func (c *Client) ListTLDs(ctx context.Context) ([]provider.TLD, error) {
	response, err := c.sdk.Domains.GetTldListWithContext(ctx)
	if err != nil {
		return nil, c.convertError(err)
	}
	if response == nil || response.Tlds == nil {
		return nil, responseError("TLD list returned no results")
	}
	result := make([]provider.TLD, 0, len(*response.Tlds))
	for i := range *response.Tlds {
		value := (*response.Tlds)[i]
		result = append(result, provider.TLD{
			Name:             stringValue(value.Name),
			NonRealTime:      value.NonRealTimeDomain,
			MinRegisterYears: value.MinRegisterYears,
			MaxRegisterYears: value.MaxRegisterYears,
			MinRenewYears:    value.MinRenewYears,
			MaxRenewYears:    value.MaxRenewYears,
			MinTransferYears: value.MinTransferYears,
			MaxTransferYears: value.MaxTransferYears,
			APIRegisterable:  value.IsAPIRegisterable,
			APIRenewable:     value.IsAPIRenewable,
			APITransferable:  value.IsAPITransferable,
			EPPRequired:      value.IsEppRequired,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (c *Client) GetZone(ctx context.Context, domain string) (provider.Zone, error) {
	response, err := c.sdk.DomainsDNS.GetHostsWithContext(ctx, domain)
	if err != nil {
		return provider.Zone{}, c.convertError(err)
	}
	if response == nil || response.DomainDNSGetHostsResult == nil {
		return provider.Zone{}, responseError("DNS host list returned no result")
	}
	value := response.DomainDNSGetHostsResult
	zone := provider.Zone{
		Version:      provider.ZoneFileVersion,
		Domain:       stringValue(value.Domain),
		EmailType:    stringValue(value.EmailType),
		NamecheapDNS: value.IsUsingOurDNS,
		Records:      make([]provider.DNSRecord, 0),
	}
	if zone.EmailType == "" {
		zone.EmailType = sdk.EmailTypeNone
	}
	if zone.Domain == "" {
		zone.Domain = domain
	}
	if value.Hosts != nil {
		zone.Records = make([]provider.DNSRecord, 0, len(*value.Hosts))
		for _, record := range *value.Hosts {
			converted, err := detailedDNSRecord(record)
			if err != nil {
				return provider.Zone{}, err
			}
			zone.Records = append(zone.Records, converted)
		}
	}
	sortDNSRecords(zone.Records)
	return zone, nil
}

func (c *Client) PlanZone(ctx context.Context, zone provider.Zone) (provider.ZonePlan, error) {
	current, err := c.GetZone(ctx, zone.Domain)
	if err != nil {
		return provider.ZonePlan{}, err
	}
	plan := provider.BuildZonePlan(current, zone)
	plan.Satisfiable = plan.Satisfiable && emailTypeAccepts(zone.Records, zone.EmailType)
	return plan, nil
}

func (c *Client) ApplyZone(ctx context.Context, zone provider.Zone, expectedFingerprint string) (provider.ZoneChange, error) {
	current, err := c.GetZone(ctx, zone.Domain)
	if err != nil {
		return provider.ZoneChange{}, err
	}
	if current.NamecheapDNS == nil || !*current.NamecheapDNS {
		return provider.ZoneChange{}, &provider.Error{
			Kind:    provider.ErrorConflict,
			Message: "the domain is no longer confirmed to use Namecheap DNS",
		}
	}
	if provider.FingerprintZone(current) != expectedFingerprint {
		return provider.ZoneChange{}, &provider.Error{
			Kind:    provider.ErrorConflict,
			Message: "DNS zone changed after planning; make a new plan and review it before retrying",
		}
	}

	plan := provider.BuildZonePlan(current, zone)
	records := sdkDNSRecords(zone.Records)
	emailType := strings.ToUpper(zone.EmailType)
	response, err := c.writer.DomainsDNS.SetHostsWithContext(ctx, &sdk.DomainsDNSSetHostsArgs{
		Domain:    sdk.String(zone.Domain),
		Records:   &records,
		EmailType: &emailType,
	})
	if err != nil {
		converted := c.convertError(err)
		if isDefiniteMutationFailure(err) {
			return provider.ZoneChange{}, converted
		}
		return provider.ZoneChange{}, c.outcomeUnknown("Namecheap did not return a definite DNS update result", err)
	}
	if response == nil || response.DomainDNSSetHostsResult == nil || response.DomainDNSSetHostsResult.IsSuccess == nil || !*response.DomainDNSSetHostsResult.IsSuccess {
		return provider.ZoneChange{}, c.outcomeUnknown("Namecheap returned an incomplete DNS update result", nil)
	}
	verified, err := c.GetZone(ctx, zone.Domain)
	if err != nil {
		return provider.ZoneChange{}, c.outcomeUnknown("DNS update verification failed", err)
	}
	if provider.FingerprintZone(verified) != provider.FingerprintZone(zone) {
		return provider.ZoneChange{}, c.outcomeUnknown("DNS update verification returned a different zone", nil)
	}
	return provider.ZoneChange{
		Domain:    zone.Domain,
		Added:     len(plan.Add),
		Removed:   len(plan.Remove),
		Kept:      len(plan.Keep),
		EmailType: emailType,
		Records:   verified.Records,
	}, nil
}

func convertDomain(value *sdk.Domain) provider.Domain {
	result := provider.Domain{
		ID:           stringValue(value.ID),
		Name:         stringValue(value.Name),
		User:         stringValue(value.User),
		Expired:      value.IsExpired,
		Locked:       value.IsLocked,
		AutoRenew:    value.AutoRenew,
		Privacy:      stringValue(value.WhoisGuard),
		Premium:      value.IsPremium,
		NamecheapDNS: value.IsOurDNS,
	}
	if value.Created != nil && !value.Created.IsZero() {
		created := value.Created.Time
		result.Created = &created
	}
	if value.Expires != nil && !value.Expires.IsZero() {
		expires := value.Expires.Time
		result.Expires = &expires
	}
	return result
}

func convertDomainInfo(value *sdk.DomainsGetInfoResult) provider.DomainInfo {
	result := provider.DomainInfo{
		Name:    stringValue(value.DomainName),
		Status:  stringValue(value.Status),
		Owner:   stringValue(value.OwnerName),
		IsOwner: value.IsOwner,
		Premium: value.IsPremium,
		Rights:  make(map[string]string),
	}
	if value.ID != nil {
		result.ID = *value.ID
	}
	if value.DomainDetails != nil {
		result.Years = value.DomainDetails.NumYears
		if value.DomainDetails.CreatedDate != nil && !value.DomainDetails.CreatedDate.IsZero() {
			created := value.DomainDetails.CreatedDate.Time
			result.Created = &created
		}
		if value.DomainDetails.ExpiredDate != nil && !value.DomainDetails.ExpiredDate.IsZero() {
			expires := value.DomainDetails.ExpiredDate.Time
			result.Expires = &expires
		}
	}
	if value.WhoisGuard != nil {
		result.Privacy.Status = stringValue(value.WhoisGuard.Enabled)
		if value.WhoisGuard.ExpiredDate != nil && !value.WhoisGuard.ExpiredDate.IsZero() {
			expires := value.WhoisGuard.ExpiredDate.Time
			result.Privacy.Expires = &expires
		}
	}
	if value.DnsDetails != nil {
		result.DNS = provider.DNSInfo{
			Provider:       stringValue(value.DnsDetails.ProviderType),
			UsingNamecheap: value.DnsDetails.IsUsingOurDNS,
			HostCount:      value.DnsDetails.HostCount,
			EmailType:      stringValue(value.DnsDetails.EmailType),
			Dynamic:        value.DnsDetails.DynamicDNSStatus,
		}
		if value.DnsDetails.Nameservers != nil {
			result.DNS.Nameservers = append([]string(nil), (*value.DnsDetails.Nameservers)...)
		}
	}
	if value.ModificationRights != nil && value.ModificationRights.Rights != nil {
		for _, right := range *value.ModificationRights.Rights {
			result.Rights[stringValue(right.Type)] = stringValue(right.Value)
		}
	}
	if len(result.Rights) == 0 {
		result.Rights = nil
	}
	return result
}

func convertDomainCheck(value sdk.DomainCheckResult) provider.DomainCheck {
	return provider.DomainCheck{
		Domain:                   stringValue(value.Domain),
		Available:                value.IsAvailable,
		Premium:                  value.IsPremiumName,
		PremiumRegistrationPrice: value.PremiumRegistrationPrice,
		PremiumRenewalPrice:      value.PremiumRenewalPrice,
		PremiumRestorePrice:      value.PremiumRestorePrice,
		PremiumTransferPrice:     value.PremiumTransferPrice,
		ICANNFee:                 value.IcannFee,
		EAPFee:                   value.EapFee,
	}
}

func detailedDNSRecord(value sdk.DomainsDNSHostRecordDetailed) (provider.DNSRecord, error) {
	record := provider.DNSRecord{
		Host:      stringValue(value.Name),
		Type:      strings.ToUpper(stringValue(value.Type)),
		Value:     stringValue(value.Address),
		ManagedBy: firstNonEmptyString(stringValue(value.AssociatedAppTitle), stringValue(value.FriendlyName)),
	}
	if value.TTL != nil {
		record.TTL = *value.TTL
	}
	if record.Type == sdk.RecordTypeMX && value.MXPref != nil {
		if *value.MXPref < 0 || *value.MXPref > 255 {
			return provider.DNSRecord{}, &provider.Error{
				Kind:    provider.ErrorProvider,
				Message: fmt.Sprintf("Namecheap returned an out-of-range MX preference for %s", record.Host),
			}
		}
		preference := uint8(*value.MXPref)
		record.MXPref = &preference
	}
	if value.IsDDNSEnabled != nil && *value.IsDDNSEnabled {
		enabled := true
		record.DDNSEnabled = &enabled
	}
	return record, nil
}

func sdkDNSRecords(records []provider.DNSRecord) []sdk.DomainsDNSHostRecord {
	result := make([]sdk.DomainsDNSHostRecord, 0, len(records))
	for _, record := range records {
		host := record.Host
		recordType := strings.ToUpper(record.Type)
		value := record.Value
		ttl := record.TTL
		converted := sdk.DomainsDNSHostRecord{
			HostName:   &host,
			RecordType: &recordType,
			Address:    &value,
			TTL:        &ttl,
		}
		if record.MXPref != nil {
			preference := *record.MXPref
			converted.MXPref = &preference
		}
		result = append(result, converted)
	}
	return result
}

func sortDNSRecords(records []provider.DNSRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		left := fmt.Sprintf("%s\x00%s\x00%s\x00%03d\x00%05d", strings.ToLower(records[i].Host), records[i].Type, records[i].Value, mxValue(records[i].MXPref), records[i].TTL)
		right := fmt.Sprintf("%s\x00%s\x00%s\x00%03d\x00%05d", strings.ToLower(records[j].Host), records[j].Type, records[j].Value, mxValue(records[j].MXPref), records[j].TTL)
		return left < right
	})
}

func mxValue(value *uint8) int {
	if value == nil {
		return -1
	}
	return int(*value)
}

func emailTypeAccepts(records []provider.DNSRecord, emailType string) bool {
	mxCount := 0
	mxeCount := 0
	for _, record := range records {
		switch strings.ToUpper(record.Type) {
		case "MX":
			mxCount++
		case "MXE":
			mxeCount++
		}
	}
	if mxCount > 0 && mxeCount > 0 {
		return false
	}
	switch strings.ToUpper(emailType) {
	case "MX":
		return mxCount > 0 && mxeCount == 0
	case "MXE":
		return mxeCount == 1 && mxCount == 0
	default:
		return mxCount == 0 && mxeCount == 0
	}
}

func (c *Client) convertError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return &provider.Error{Kind: provider.ErrorCanceled, Message: "operation canceled", Cause: err}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &provider.Error{Kind: provider.ErrorNetwork, Message: redact(err.Error(), c.apiKey), Cause: err}
	}
	if errors.Is(err, sdk.ErrConcurrentModification) {
		return &provider.Error{Kind: provider.ErrorConflict, Message: err.Error(), Cause: err}
	}
	var invalid *sdk.InvalidArgumentsError
	if errors.As(err, &invalid) {
		return &provider.Error{Kind: provider.ErrorInvalid, Message: redact(invalid.Error(), c.apiKey), Cause: err}
	}
	apiErrors := collectAPIErrors(err)
	if len(apiErrors) > 0 {
		apiError := preferredAPIError(apiErrors)
		kind := provider.ErrorProvider
		switch {
		case hasAuthenticationError(apiErrors), errors.Is(err, sdk.ErrAccessDenied):
			kind = provider.ErrorAuth
		case errors.Is(err, sdk.ErrDomainNotFound), errors.Is(err, sdk.ErrDomainNotAssociated):
			kind = provider.ErrorNotFound
		case sdk.IsRetryable(err):
			kind = provider.ErrorRetryable
		}
		return &provider.Error{
			Kind:         kind,
			ProviderCode: apiError.Number,
			Message:      authenticationMessage(apiError, redact(apiError.Error(), c.apiKey)),
			Cause:        err,
		}
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		kind := provider.ErrorNetwork
		if sdk.IsRetryable(err) {
			kind = provider.ErrorRetryable
		}
		return &provider.Error{Kind: kind, Message: redact(networkError.Error(), c.apiKey), Cause: err}
	}
	return &provider.Error{Kind: provider.ErrorProvider, Message: redact(err.Error(), c.apiKey), Cause: err}
}

func (c *Client) outcomeUnknown(message string, cause error) error {
	if cause != nil {
		message += ": " + redact(cause.Error(), c.apiKey)
	}
	return &provider.Error{Kind: provider.ErrorOutcomeUnknown, Message: message, Cause: cause}
}

func isDefiniteMutationFailure(err error) bool {
	var invalid *sdk.InvalidArgumentsError
	if errors.As(err, &invalid) {
		return true
	}
	return len(collectAPIErrors(err)) > 0
}

func collectAPIErrors(err error) []*sdk.APIError {
	result := make([]*sdk.APIError, 0)
	var walk func(error)
	walk = func(current error) {
		if current == nil {
			return
		}
		if apiError, ok := current.(*sdk.APIError); ok {
			result = append(result, apiError)
		}
		switch value := current.(type) {
		case interface{ Unwrap() []error }:
			for _, nested := range value.Unwrap() {
				walk(nested)
			}
		case interface{ Unwrap() error }:
			walk(value.Unwrap())
		}
	}
	walk(err)
	return result
}

func preferredAPIError(apiErrors []*sdk.APIError) *sdk.APIError {
	for _, apiError := range apiErrors {
		if isAuthenticationErrorCode(apiError.Number) || errors.Is(apiError, sdk.ErrAccessDenied) {
			return apiError
		}
	}
	return apiErrors[0]
}

func hasAuthenticationError(apiErrors []*sdk.APIError) bool {
	for _, apiError := range apiErrors {
		if isAuthenticationErrorCode(apiError.Number) || errors.Is(apiError, sdk.ErrAccessDenied) {
			return true
		}
	}
	return false
}

func isAuthenticationErrorCode(code int) bool {
	_, ok := authenticationErrorCodes[code]
	return ok
}

func authenticationMessage(apiError *sdk.APIError, fallback string) string {
	switch apiError.Number {
	case 1011150:
		return "request IPv4 is not whitelisted for the selected Namecheap environment (1011150)"
	case 1017150, 1017105:
		return fmt.Sprintf("Namecheap reports that an IP authentication parameter is disabled or locked (%d)", apiError.Number)
	default:
		return fallback
	}
}

func redact(message string, values ...string) string {
	for _, value := range values {
		if value != "" {
			message = strings.ReplaceAll(message, value, "[REDACTED]")
		}
	}
	return message
}

func responseError(message string) error {
	return &provider.Error{Kind: provider.ErrorProvider, Message: message}
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
