package apicatalog

import (
	"strings"
	"testing"
)

func TestCatalogCoversEveryOfficialMethod(t *testing.T) {
	t.Parallel()

	methods := Methods()
	if len(methods) != 59 {
		t.Fatalf("methods = %d, want 59", len(methods))
	}
	paths := make(map[string]struct{}, len(methods))
	names := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		path := method.CLIPath()
		if path == "" || len(method.Path) < 2 {
			t.Fatalf("invalid command path: %+v", method)
		}
		if _, exists := paths[path]; exists {
			t.Fatalf("duplicate command path: %s", path)
		}
		paths[path] = struct{}{}
		name := strings.ToLower(method.Name)
		if _, exists := names[name]; exists {
			t.Fatalf("duplicate API method: %s", method.Name)
		}
		names[name] = struct{}{}
		if !strings.HasPrefix(method.Name, "namecheap.") {
			t.Fatalf("invalid API method: %s", method.Name)
		}
		if !strings.HasPrefix(method.Documentation, documentationBase) {
			t.Fatalf("invalid documentation link: %s", method.Documentation)
		}
	}

	for _, required := range []string{
		"namecheap.domains.create",
		"namecheap.domains.dns.setHosts",
		"namecheap.domains.transfer.create",
		"namecheap.ssl.activate",
		"namecheap.users.create",
		"namecheap.users.address.update",
		"namecheap.whoisguard.renew",
	} {
		if _, exists := names[strings.ToLower(required)]; !exists {
			t.Fatalf("catalog is missing %s", required)
		}
	}
}

func TestFindAcceptsAPINameAndCLIPath(t *testing.T) {
	t.Parallel()

	byName, ok := Find("NAMECHEAP.DOMAINS.CREATE")
	if !ok || byName.CLIPath() != "domains create" {
		t.Fatalf("Find by name = %+v, %v", byName, ok)
	}
	byPath, ok := Find("domains/dns/set-custom")
	if !ok || byPath.Name != "namecheap.domains.dns.setCustom" {
		t.Fatalf("Find by path = %+v, %v", byPath, ok)
	}
}
