// Package output renders command results, enforcing the project's agent
// contract: machine-readable data goes to stdout, while human-oriented text,
// progress and diagnostics go to stderr. JSON/NDJSON/YAML output never contains
// log lines, so an agent can parse stdout unconditionally.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// Format is a machine- or human-facing rendering of command output.
type Format string

const (
	// FormatTable is the default human-friendly rendering (aligned columns).
	FormatTable Format = "table"
	// FormatJSON renders a single pretty-printed JSON document.
	FormatJSON Format = "json"
	// FormatNDJSON renders newline-delimited JSON: one compact object per line,
	// ideal for streaming lists into agents and tools like jq.
	FormatNDJSON Format = "ndjson"
	// FormatYAML renders a single YAML document.
	FormatYAML Format = "yaml"
)

// ParseFormat normalises a user-supplied format string. An empty string maps to
// the table default.
func ParseFormat(s string) (Format, error) {
	switch f := Format(strings.ToLower(strings.TrimSpace(s))); f {
	case "":
		return FormatTable, nil
	case FormatTable, FormatJSON, FormatNDJSON, FormatYAML:
		return f, nil
	default:
		return "", fmt.Errorf("unknown output format %q (want one of: table, json, ndjson, yaml)", s)
	}
}

// Machine reports whether the format is intended for programmatic consumption.
func (f Format) Machine() bool { return f != FormatTable }

// HumanFunc renders a value for human consumption (table mode). It receives the
// stdout writer. Commands typically build aligned tables via NewTabWriter.
type HumanFunc func(w io.Writer) error

// Printer is the single sink for command output. Construct it once per command
// invocation from the resolved global flags.
type Printer struct {
	Out    io.Writer
	Err    io.Writer
	Format Format
	// Quiet suppresses incidental human messages (Info/Hintf). It never affects
	// machine data on stdout.
	Quiet bool
	// Color enables ANSI styling in human output. Callers should set this from a
	// TTY check plus NO_COLOR/--no-color handling.
	Color bool
}

// Emit renders v in the configured format. In machine formats v is marshalled
// directly; in table mode the human callback is used (falling back to JSON if
// nil so no command is ever silent).
func (p *Printer) Emit(v any, human HumanFunc) error {
	switch p.Format {
	case FormatJSON:
		return p.writeJSON(v)
	case FormatNDJSON:
		return p.writeNDJSON(v)
	case FormatYAML:
		return p.writeYAML(v)
	default:
		if human != nil {
			return human(p.Out)
		}
		return p.writeJSON(v)
	}
}

func (p *Printer) writeJSON(v any) error {
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// writeNDJSON streams one compact JSON object per line. If v is a slice or
// array, each element is emitted on its own line; otherwise v is emitted as a
// single line. This keeps `--output ndjson` consistent for both lists and
// scalars.
func (p *Printer) writeNDJSON(v any) error {
	enc := json.NewEncoder(p.Out)
	enc.SetEscapeHTML(false)

	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Pointer && !rv.IsNil() {
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		for i := 0; i < rv.Len(); i++ {
			if err := enc.Encode(rv.Index(i).Interface()); err != nil {
				return err
			}
		}
		return nil
	}
	return enc.Encode(v)
}

func (p *Printer) writeYAML(v any) error {
	enc := yaml.NewEncoder(p.Out)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return err
	}
	return enc.Close()
}

// Info writes a human-oriented message to stderr unless quiet. Use for progress
// and confirmations — never for data an agent needs to parse.
func (p *Printer) Info(format string, args ...any) {
	if p.Quiet {
		return
	}
	fmt.Fprintf(p.Err, format+"\n", args...)
}

// NewTabWriter returns a tabwriter configured for the project's table style.
// Callers write tab-separated rows and must Flush.
func NewTabWriter(w io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(w, 0, 4, 3, ' ', 0)
}
