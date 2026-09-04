package output

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

func TestJSONDataUsesStdoutOnly(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	printer := Printer{Stdout: &stdout, Stderr: &stderr, JSON: true}

	err := printer.Data("test.command", map[string]string{"answer": "yes"}, func(io.Writer) error {
		return errors.New("human renderer must not run")
	})
	if err != nil {
		t.Fatalf("Data returned an error: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	var envelope Envelope
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.SchemaVersion != SchemaVersion || envelope.Command != "test.command" || !envelope.OK {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
}

func TestJSONFailureUsesStderrOnly(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	printer := Printer{Stdout: &stdout, Stderr: &stderr, JSON: true}
	printer.Failure("test.command", "test_error", "it failed")

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	var envelope Envelope
	if err := json.Unmarshal(stderr.Bytes(), &envelope); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if envelope.OK || envelope.Error == nil || envelope.Error.Code != "test_error" {
		t.Fatalf("unexpected envelope: %+v", envelope)
	}
	if !bytes.Contains(stderr.Bytes(), []byte(`"data":null`)) {
		t.Fatalf("failure envelope omitted stable data field: %s", stderr.String())
	}
}
