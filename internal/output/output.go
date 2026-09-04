package output

import (
	"encoding/json"
	"fmt"
	"io"
)

const SchemaVersion = "v1"

// Problem is the stable machine representation of a command failure.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Envelope is the stable top-level JSON contract.
type Envelope struct {
	SchemaVersion string   `json:"schema_version"`
	Command       string   `json:"command"`
	OK            bool     `json:"ok"`
	Data          any      `json:"data"`
	Meta          any      `json:"meta,omitempty"`
	Error         *Problem `json:"error,omitempty"`
}

// Printer keeps command data and diagnostics on separate streams.
type Printer struct {
	Stdout io.Writer
	Stderr io.Writer
	JSON   bool
}

// Data renders a successful command result.
func (p Printer) Data(command string, data any, human func(io.Writer) error) error {
	return p.DataWithMeta(command, data, nil, human)
}

// DataWithMeta renders a successful result with non-secret execution context.
func (p Printer) DataWithMeta(command string, data any, meta any, human func(io.Writer) error) error {
	if p.JSON {
		return writeJSON(p.Stdout, Envelope{
			SchemaVersion: SchemaVersion,
			Command:       command,
			OK:            true,
			Data:          data,
			Meta:          meta,
		})
	}
	return human(p.Stdout)
}

// Failure renders a command failure without placing diagnostics on stdout.
func (p Printer) Failure(command, code, message string) {
	if p.JSON {
		_ = writeJSON(p.Stderr, Envelope{
			SchemaVersion: SchemaVersion,
			Command:       command,
			OK:            false,
			Error: &Problem{
				Code:    code,
				Message: message,
			},
		})
		return
	}
	_, _ = fmt.Fprintf(p.Stderr, "Error: %s\n", message)
}

// Diagnostic writes human context to stderr and stays silent in JSON mode.
func (p Printer) Diagnostic(format string, args ...any) {
	if p.JSON {
		return
	}
	_, _ = fmt.Fprintf(p.Stderr, format, args...)
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	return encoder.Encode(value)
}
