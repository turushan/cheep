package provider

import (
	"context"
	"time"

	"github.com/turushan/nccli/internal/config"
)

type ErrorKind string

const (
	ErrorInvalid        ErrorKind = "invalid"
	ErrorCanceled       ErrorKind = "canceled"
	ErrorAuth           ErrorKind = "authentication"
	ErrorPermission     ErrorKind = "permission"
	ErrorNotFound       ErrorKind = "not_found"
	ErrorNetwork        ErrorKind = "network"
	ErrorRetryable      ErrorKind = "retryable"
	ErrorConflict       ErrorKind = "conflict"
	ErrorOutcomeUnknown ErrorKind = "outcome_unknown"
	ErrorProvider       ErrorKind = "provider"
)

// Error keeps Namecheap-specific failures behind the provider boundary.
type Error struct {
	Kind         ErrorKind
	ProviderCode int
	Message      string
	Cause        error
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.Cause
}

// Factory creates a provider service for one resolved profile.
type Factory interface {
	New(config.Profile) Service
}

// Reader is the complete read-only v0.1 provider surface.
type Reader interface {
	Probe(context.Context) (Probe, error)
	ListDomains(context.Context, DomainListFilter) ([]Domain, error)
	DomainInfo(context.Context, string) (DomainInfo, error)
	CheckDomains(context.Context, []string) ([]DomainCheck, error)
	Balance(context.Context) (Balance, error)
	Price(context.Context, PriceRequest) (Price, error)
	ListTLDs(context.Context) ([]TLD, error)
}

// DNS manages whole-zone operations through an explicit plan and verified apply flow.
type DNS interface {
	GetZone(context.Context, string) (Zone, error)
	PlanZone(context.Context, Zone) (ZonePlan, error)
	ApplyZone(context.Context, Zone, string) (ZoneChange, error)
}

// Service is the provider surface available to the command layer.
type Service interface {
	Reader
	DNS
}

type Probe struct {
	DomainCount int `json:"domain_count"`
}

type DomainListFilter struct {
	ListType string
	Search   string
	Sort     string
}

type Domain struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	User         string     `json:"user"`
	Created      *time.Time `json:"created,omitempty"`
	Expires      *time.Time `json:"expires,omitempty"`
	Expired      *bool      `json:"expired,omitempty"`
	Locked       *bool      `json:"locked,omitempty"`
	AutoRenew    *bool      `json:"auto_renew,omitempty"`
	Privacy      string     `json:"privacy,omitempty"`
	Premium      *bool      `json:"premium,omitempty"`
	NamecheapDNS *bool      `json:"namecheap_dns,omitempty"`
}

type DomainInfo struct {
	ID      int               `json:"id"`
	Name    string            `json:"name"`
	Status  string            `json:"status"`
	Owner   string            `json:"owner"`
	IsOwner *bool             `json:"is_owner,omitempty"`
	Premium *bool             `json:"premium,omitempty"`
	Created *time.Time        `json:"created,omitempty"`
	Expires *time.Time        `json:"expires,omitempty"`
	Years   *int              `json:"years,omitempty"`
	Privacy PrivacyInfo       `json:"privacy"`
	DNS     DNSInfo           `json:"dns"`
	Rights  map[string]string `json:"rights,omitempty"`
}

type PrivacyInfo struct {
	Status  string     `json:"status,omitempty"`
	Expires *time.Time `json:"expires,omitempty"`
}

type DNSInfo struct {
	Provider       string   `json:"provider,omitempty"`
	UsingNamecheap *bool    `json:"using_namecheap,omitempty"`
	HostCount      *int     `json:"host_count,omitempty"`
	EmailType      string   `json:"email_type,omitempty"`
	Dynamic        *bool    `json:"dynamic,omitempty"`
	Nameservers    []string `json:"nameservers,omitempty"`
}

type DomainCheck struct {
	Domain                   string   `json:"domain"`
	Available                *bool    `json:"available,omitempty"`
	Premium                  *bool    `json:"premium,omitempty"`
	PremiumRegistrationPrice *float64 `json:"premium_registration_price,omitempty"`
	PremiumRenewalPrice      *float64 `json:"premium_renewal_price,omitempty"`
	PremiumRestorePrice      *float64 `json:"premium_restore_price,omitempty"`
	PremiumTransferPrice     *float64 `json:"premium_transfer_price,omitempty"`
	ICANNFee                 *float64 `json:"icann_fee,omitempty"`
	EAPFee                   *float64 `json:"eap_fee,omitempty"`
}

type Balance struct {
	Currency             string `json:"currency"`
	Available            string `json:"available"`
	Account              string `json:"account"`
	Earned               string `json:"earned"`
	Withdrawable         string `json:"withdrawable"`
	RequiredForAutoRenew string `json:"required_for_auto_renew"`
}

type PriceRequest struct {
	TLD    string
	Action string
	Years  int
}

type Price struct {
	TLD          string `json:"tld"`
	Action       string `json:"action"`
	Years        int    `json:"years"`
	DurationType string `json:"duration_type"`
	Currency     string `json:"currency"`
	Effective    string `json:"effective"`
	Regular      string `json:"regular"`
	Account      string `json:"account"`
	Promotion    string `json:"promotion"`
}

type TLD struct {
	Name             string `json:"name"`
	NonRealTime      *bool  `json:"non_real_time,omitempty"`
	MinRegisterYears *int   `json:"min_register_years,omitempty"`
	MaxRegisterYears *int   `json:"max_register_years,omitempty"`
	MinRenewYears    *int   `json:"min_renew_years,omitempty"`
	MaxRenewYears    *int   `json:"max_renew_years,omitempty"`
	MinTransferYears *int   `json:"min_transfer_years,omitempty"`
	MaxTransferYears *int   `json:"max_transfer_years,omitempty"`
	APIRegisterable  *bool  `json:"api_registerable,omitempty"`
	APIRenewable     *bool  `json:"api_renewable,omitempty"`
	APITransferable  *bool  `json:"api_transferable,omitempty"`
	EPPRequired      *bool  `json:"epp_required,omitempty"`
}

const ZoneFileVersion = 1

type Zone struct {
	Version      int         `json:"version" yaml:"version"`
	Domain       string      `json:"domain" yaml:"domain"`
	EmailType    string      `json:"email_type" yaml:"email_type"`
	NamecheapDNS *bool       `json:"namecheap_dns,omitempty" yaml:"namecheap_dns,omitempty"`
	Records      []DNSRecord `json:"records" yaml:"records"`
}

type DNSRecord struct {
	Host        string `json:"host" yaml:"host"`
	Type        string `json:"type" yaml:"type"`
	Value       string `json:"value" yaml:"value"`
	TTL         int    `json:"ttl" yaml:"ttl"`
	MXPref      *uint8 `json:"mx_pref,omitempty" yaml:"mx_pref,omitempty"`
	ManagedBy   string `json:"managed_by,omitempty" yaml:"managed_by,omitempty"`
	DDNSEnabled *bool  `json:"ddns_enabled,omitempty" yaml:"ddns_enabled,omitempty"`
}

type ZonePlan struct {
	Domain             string      `json:"domain"`
	CurrentFingerprint string      `json:"current_fingerprint"`
	CurrentEmailType   string      `json:"current_email_type"`
	DesiredEmailType   string      `json:"desired_email_type"`
	RequiredEmailType  string      `json:"required_email_type,omitempty"`
	Satisfiable        bool        `json:"satisfiable"`
	Add                []DNSRecord `json:"add"`
	Remove             []DNSRecord `json:"remove"`
	Keep               []DNSRecord `json:"keep"`
}

type ZoneChange struct {
	Domain    string      `json:"domain"`
	Added     int         `json:"added"`
	Removed   int         `json:"removed"`
	Kept      int         `json:"kept"`
	EmailType string      `json:"email_type"`
	Records   []DNSRecord `json:"records"`
}
