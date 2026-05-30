package output

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestParseFormat(t *testing.T) {
	for _, in := range []string{"", "table", "json", "JSON", "ndjson", "yaml"} {
		if _, err := ParseFormat(in); err != nil {
			t.Errorf("ParseFormat(%q) unexpected error: %v", in, err)
		}
	}
	if _, err := ParseFormat("xml"); err == nil {
		t.Error("ParseFormat(xml): expected error")
	}
}

func TestEmitJSON(t *testing.T) {
	var out bytes.Buffer
	p := &Printer{Out: &out, Format: FormatJSON}
	if err := p.Emit(map[string]int{"a": 1}, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "\"a\": 1") {
		t.Fatalf("json output = %q", out.String())
	}
}

func TestEmitNDJSONSlice(t *testing.T) {
	var out bytes.Buffer
	p := &Printer{Out: &out, Format: FormatNDJSON}
	if err := p.Emit([]int{1, 2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 ndjson lines, got %d: %q", len(lines), out.String())
	}
}

func TestEmitTableUsesHumanCallback(t *testing.T) {
	var out bytes.Buffer
	p := &Printer{Out: &out, Format: FormatTable}
	called := false
	err := p.Emit(struct{}{}, func(w io.Writer) error {
		called = true
		_, werr := w.Write([]byte("human"))
		return werr
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("human callback was not invoked in table mode")
	}
	if out.String() != "human" {
		t.Fatalf("table output = %q", out.String())
	}
}

func TestEmitErrorEnvelope(t *testing.T) {
	var errBuf bytes.Buffer
	p := &Printer{Err: &errBuf, Format: FormatJSON}
	p.EmitError(ErrorPayload{Code: "not_found", Message: "missing", HTTPStatus: 404})
	s := errBuf.String()
	if !strings.Contains(s, "\"error\"") || !strings.Contains(s, "\"code\": \"not_found\"") {
		t.Fatalf("error envelope = %q", s)
	}
}

func TestEmitErrorHuman(t *testing.T) {
	var errBuf bytes.Buffer
	p := &Printer{Err: &errBuf, Format: FormatTable}
	p.EmitError(ErrorPayload{Code: "auth", Message: "no token", Hint: "run login"})
	s := errBuf.String()
	if !strings.Contains(s, "Error: no token") || !strings.Contains(s, "Hint:") {
		t.Fatalf("human error = %q", s)
	}
}
