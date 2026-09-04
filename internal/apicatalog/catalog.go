// Package apicatalog defines the official Namecheap API methods exposed by
// Cheep's complete, generic command surface.
package apicatalog

import (
	"sort"
	"strings"
)

const documentationBase = "https://www.namecheap.com/support/api/methods/"

// Method describes one official Namecheap API operation.
type Method struct {
	Path          []string `json:"path"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Documentation string   `json:"documentation"`
	Mutation      bool     `json:"mutation"`
	ChargeBearing bool     `json:"charge_bearing"`
}

// CLIPath returns the space-separated path below `cheep api`.
func (m Method) CLIPath() string {
	return strings.Join(m.Path, " ")
}

// Methods returns the current official method catalog in CLI path order.
func Methods() []Method {
	methods := []Method{
		method("domains/get-list", "namecheap.domains.getList", "List domains in the account", "domains/get-list/", false, false),
		method("domains/get-contacts", "namecheap.domains.getContacts", "Get contacts for a domain", "domains/get-contacts/", false, false),
		method("domains/create", "namecheap.domains.create", "Register a domain", "domains/create/", true, true),
		method("domains/get-tld-list", "namecheap.domains.getTldList", "List supported TLDs and capabilities", "domains/get-tld-list/", false, false),
		method("domains/set-contacts", "namecheap.domains.setContacts", "Set contacts for a domain", "domains/set-contacts/", true, false),
		method("domains/check", "namecheap.domains.check", "Check domain availability", "domains/check/", false, false),
		method("domains/reactivate", "namecheap.domains.reactivate", "Reactivate an expired domain", "domains/reactivate/", true, true),
		method("domains/renew", "namecheap.domains.renew", "Renew a domain", "domains/renew/", true, true),
		method("domains/get-registrar-lock", "namecheap.domains.getRegistrarLock", "Get registrar lock status", "domains/get-registrar-lock/", false, false),
		method("domains/set-registrar-lock", "namecheap.domains.setRegistrarLock", "Set registrar lock status", "domains/set-registrar-lock/", true, false),
		method("domains/get-info", "namecheap.domains.getInfo", "Get detailed domain information", "domains/get-info/", false, false),

		method("domains/dns/set-default", "namecheap.domains.dns.setDefault", "Use Namecheap default DNS servers", "domains-dns/set-default/", true, false),
		method("domains/dns/set-custom", "namecheap.domains.dns.setCustom", "Use custom DNS servers", "domains-dns/set-custom/", true, false),
		method("domains/dns/get-list", "namecheap.domains.dns.getList", "List assigned DNS servers", "domains-dns/get-list/", false, false),
		method("domains/dns/get-hosts", "namecheap.domains.dns.getHosts", "Get DNS host records", "domains-dns/get-hosts/", false, false),
		method("domains/dns/get-email-forwarding", "namecheap.domains.dns.getEmailForwarding", "Get email forwarding settings", "domains-dns/get-email-forwarding/", false, false),
		method("domains/dns/set-email-forwarding", "namecheap.domains.dns.setEmailForwarding", "Set email forwarding settings", "domains-dns/set-email-forwarding/", true, false),
		method("domains/dns/set-hosts", "namecheap.domains.dns.setHosts", "Replace DNS host records", "domains-dns/set-hosts/", true, false),

		method("domains/ns/create", "namecheap.domains.ns.create", "Create a registered nameserver", "domains-ns/create/", true, false),
		method("domains/ns/delete", "namecheap.domains.ns.delete", "Delete a registered nameserver", "domains-ns/delete/", true, false),
		method("domains/ns/get-info", "namecheap.domains.ns.getInfo", "Get registered nameserver information", "domains-ns/get-info/", false, false),
		method("domains/ns/update", "namecheap.domains.ns.update", "Update a registered nameserver address", "domains-ns/update/", true, false),

		method("domains/transfer/create", "namecheap.domains.transfer.create", "Transfer a domain to Namecheap", "domains-transfer/create/", true, true),
		method("domains/transfer/get-status", "namecheap.domains.transfer.getStatus", "Get a domain transfer status", "domains-transfer/get-status/", false, false),
		method("domains/transfer/update-status", "namecheap.domains.transfer.updateStatus", "Resubmit a domain transfer", "domains-transfer/update-status/", true, false),
		method("domains/transfer/get-list", "namecheap.domains.transfer.getList", "List domain transfers", "domains-transfer/get-list/", false, false),

		method("ssl/create", "namecheap.ssl.create", "Purchase an SSL certificate", "ssl/create/", true, true),
		method("ssl/get-list", "namecheap.ssl.getList", "List SSL certificates", "ssl/get-list/", false, false),
		method("ssl/parse-csr", "namecheap.ssl.parseCSR", "Parse a certificate signing request", "ssl/parse-csr/", false, false),
		method("ssl/get-approver-email-list", "namecheap.ssl.getApproverEmailList", "List certificate approver emails", "ssl/get-approver-email-list/", false, false),
		method("ssl/activate", "namecheap.ssl.activate", "Activate a purchased SSL certificate", "ssl/activate/", true, false),
		method("ssl/resend-approver-email", "namecheap.ssl.resendApproverEmail", "Resend an SSL approver email", "ssl/resend-approver-email/", true, false),
		method("ssl/get-info", "namecheap.ssl.getInfo", "Get SSL certificate information", "ssl/get-info/", false, false),
		method("ssl/renew", "namecheap.ssl.renew", "Renew an SSL certificate", "ssl/renew/", true, true),
		method("ssl/reissue", "namecheap.ssl.reissue", "Reissue an SSL certificate", "ssl/reissue/", true, false),
		method("ssl/resend-fulfillment-email", "namecheap.ssl.resendfulfillmentemail", "Resend an SSL fulfillment email", "ssl/resend-fulfillment-email/", true, false),
		method("ssl/purchase-more-sans", "namecheap.ssl.purchasemoresans", "Purchase additional certificate SANs", "ssl/purchase-more-sans/", true, true),
		method("ssl/revoke-certificate", "namecheap.ssl.revokecertificate", "Revoke a reissued SSL certificate", "ssl/revoke-certificate/", true, false),
		method("ssl/edit-dcv-method", "namecheap.ssl.editDCVMethod", "Change or retry certificate validation", "ssl/edit-dcv-method/", true, false),

		method("users/get-pricing", "namecheap.users.getPricing", "Get account pricing", "users/get-pricing/", false, false),
		method("users/get-balances", "namecheap.users.getBalances", "Get account balances", "users/get-balances/", false, false),
		method("users/change-password", "namecheap.users.changePassword", "Change a user password", "users/change-password/", true, false),
		method("users/update", "namecheap.users.update", "Update a user account", "users/update/", true, false),
		method("users/create-add-funds-request", "namecheap.users.createaddfundsrequest", "Create an add-funds request", "users/create-add-funds-request/", true, true),
		method("users/get-add-funds-status", "namecheap.users.getAddFundsStatus", "Get add-funds request status", "users/get-add-funds-status/", false, false),
		method("users/create", "namecheap.users.create", "Create a user under the API account", "users/create/", true, false),
		method("users/login", "namecheap.users.login", "Validate a created user's credentials", "users/login/", false, false),
		method("users/reset-password", "namecheap.users.resetPassword", "Email a password reset link", "users/reset-password/", true, false),

		method("users/address/create", "namecheap.users.address.create", "Create a user address", "users-address/create/", true, false),
		method("users/address/delete", "namecheap.users.address.delete", "Delete a user address", "users-address/delete/", true, false),
		method("users/address/get-info", "namecheap.users.address.getInfo", "Get a user address", "users-address/get-info/", false, false),
		method("users/address/get-list", "namecheap.users.address.getList", "List user addresses", "users-address/get-list/", false, false),
		method("users/address/set-default", "namecheap.users.address.setDefault", "Set the default user address", "users-address/set-default/", true, false),
		method("users/address/update", "namecheap.users.address.update", "Update a user address", "users-address/update/", true, false),

		method("domainprivacy/change-email-address", "namecheap.whoisguard.changeemailaddress", "Change a domain privacy email address", "domainprivacy/change-email-address/", true, false),
		method("domainprivacy/enable", "namecheap.whoisguard.enable", "Enable domain privacy", "domainprivacy/enable/", true, false),
		method("domainprivacy/disable", "namecheap.whoisguard.disable", "Disable domain privacy", "domainprivacy/disable/", true, false),
		method("domainprivacy/get-list", "namecheap.whoisguard.getlist", "List domain privacy subscriptions", "domainprivacy/get-list/", false, false),
		method("domainprivacy/renew", "namecheap.whoisguard.renew", "Renew domain privacy", "domainprivacy/renew/", true, true),
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i].CLIPath() < methods[j].CLIPath() })
	return methods
}

// Find resolves either an exact API method name or a slash/space-separated CLI
// path. Name matching is case-insensitive because Namecheap examples vary in
// capitalization.
func Find(value string) (Method, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalizedPath := strings.ReplaceAll(normalized, "/", " ")
	normalizedPath = strings.Join(strings.Fields(normalizedPath), " ")
	for _, candidate := range Methods() {
		if strings.ToLower(candidate.Name) == normalized || candidate.CLIPath() == normalizedPath {
			return candidate, true
		}
	}
	return Method{}, false
}

func method(path, name, description, documentation string, mutation, chargeBearing bool) Method {
	return Method{
		Path:          strings.Split(path, "/"),
		Name:          name,
		Description:   description,
		Documentation: documentationBase + documentation,
		Mutation:      mutation,
		ChargeBearing: chargeBearing,
	}
}
