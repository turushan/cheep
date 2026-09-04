package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/turushan/cheep/internal/buildinfo"
	"github.com/turushan/cheep/internal/exitcode"
	"github.com/turushan/cheep/internal/output"
)

func TestVersionHumanOutput(t *testing.T) {
	t.Parallel()

	status, stdout, stderr := run(t, "version")
	if status != exitcode.Success {
		t.Fatalf("status = %d, want %d; stderr: %s", status, exitcode.Success, stderr)
	}
	if stdout != "cheep 1.2.3 (abc123)\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestVersionJSONOutput(t *testing.T) {
	t.Parallel()

	status, stdout, stderr := run(t, "--json", "version")
	if status != exitcode.Success {
		t.Fatalf("status = %d, want %d; stderr: %s", status, exitcode.Success, stderr)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}

	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.Command != "version" || !envelope.OK {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestInvalidEnvironmentUsesUsageExitCode(t *testing.T) {
	t.Parallel()

	status, stdout, stderr := run(t, "--environment", "maybe", "version")
	if status != exitcode.Usage {
		t.Fatalf("status = %d, want %d", status, exitcode.Usage)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "environment must be sandbox or production") {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestJSONErrorUsesStderr(t *testing.T) {
	t.Parallel()

	status, stdout, stderr := run(t, "--json", "version", "extra")
	if status != exitcode.Usage {
		t.Fatalf("status = %d, want %d", status, exitcode.Usage)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}

	var envelope output.Envelope
	if err := json.Unmarshal([]byte(stderr), &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.OK || envelope.Error == nil || envelope.Error.Code != "unexpected_arguments" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestSchemaIsExplicitlyExperimentalAndUsesEnvelopeNames(t *testing.T) {
	t.Parallel()

	root, _ := newRoot(Options{})
	schema := buildSchema(root)
	if schema.Version != "experimental-v0" || !schema.Experimental {
		t.Fatalf("unexpected schema metadata: %+v", schema)
	}
	foundDNSApply := false
	for _, command := range schema.Commands {
		if strings.HasPrefix(command.Name, "cheep.") || strings.HasPrefix(command.Name, "completion") {
			t.Fatalf("unexpected schema command name: %q", command.Name)
		}
		if command.Name == "dns.apply" {
			foundDNSApply = true
		}
	}
	if !foundDNSApply {
		t.Fatal("schema did not include dns.apply")
	}
}

func run(t *testing.T, args ...string) (int, string, string) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	status := Execute(args, Options{
		Stdin:  strings.NewReader(""),
		Stdout: &stdout,
		Stderr: &stderr,
		Build: buildinfo.Info{
			Version: "1.2.3",
			Commit:  "abc123",
			BuiltAt: "2026-09-04T12:00:00Z",
		},
	})
	return status, stdout.String(), stderr.String()
}
