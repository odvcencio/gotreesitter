package sched

import (
	"flag"
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite docs/v1-capability-flags.md from the capability table")

// wantOpenFlags pins the scoreboard metric "capability flags open". Change it
// only in the same change that closes or opens a flag, and regenerate
// docs/v1-capability-flags.md with -update.
const wantOpenFlags = 8

func TestCapabilityTableMatchesModes(t *testing.T) {
	var all, open Mode
	names := map[string]bool{}
	for i, row := range capabilities {
		if bits.OnesCount32(uint32(row.Mode)) != 1 {
			t.Fatalf("row %d (%s) has mode %#x; want one bit", i, row.Name, row.Mode)
		}
		if want := Mode(1) << i; row.Mode != want {
			t.Fatalf("row %d (%s) has mode %#x; want bit order %#x", i, row.Name, row.Mode, want)
		}
		if row.Name == "" || row.Closes == "" || row.Detail == "" {
			t.Fatalf("row %d has an empty field: %+v", i, row)
		}
		if names[row.Name] {
			t.Fatalf("duplicate capability name %q", row.Name)
		}
		names[row.Name] = true
		all |= row.Mode
		if !row.Compact {
			open |= row.Mode
		}
	}
	if open != openModes {
		t.Fatalf("openModes = %#x; the table's open rows are %#x", openModes, open)
	}
	if last := OldTreeReuse; all != last<<1-1 {
		t.Fatalf("table covers modes %#x; want every mode through %#x", all, last)
	}
}

// TestOpenFlagCount prints the scoreboard metric and pins it.
func TestOpenFlagCount(t *testing.T) {
	got := OpenCount()
	t.Logf("capability flags open: %d", got)
	for _, row := range Capabilities() {
		if !row.Compact {
			t.Logf("open flag %-20s closes with %s", row.Name, row.Closes)
		}
	}
	if got != wantOpenFlags {
		t.Fatalf("capability flags open = %d; want %d. Update wantOpenFlags and regenerate the document", got, wantOpenFlags)
	}
	if bits.OnesCount32(uint32(Open())) != got {
		t.Fatalf("Open() has %d bits; OpenCount() = %d", bits.OnesCount32(uint32(Open())), got)
	}
}

func TestSupports(t *testing.T) {
	served := Incremental | UTF16 | Profiling | Strict | Deadline
	if !Supports(Request{}) || !Supports(Request{Modes: served}) {
		t.Fatal("Supports rejected a request with only served modes")
	}
	for _, row := range capabilities {
		req := Request{Modes: served | row.Mode}
		if Supports(req) != row.Compact {
			t.Fatalf("Supports(%s) = %v; want %v", row.Name, Supports(req), row.Compact)
		}
		if got := req.OpenModes(); (got != 0) == row.Compact {
			t.Fatalf("OpenModes(%s) = %#x", row.Name, got)
		}
		if !req.Has(row.Mode) || !req.Has(served) {
			t.Fatalf("Has failed for %s", row.Name)
		}
	}
	if (Request{Modes: Incremental}).Has(Incremental | UTF16) {
		t.Fatal("Has reported a mode the request does not need")
	}
}

func TestCapabilitiesReturnsCopy(t *testing.T) {
	rows := Capabilities()
	rows[0].Compact = !rows[0].Compact
	if Capabilities()[0].Compact == rows[0].Compact {
		t.Fatal("Capabilities exposed the package table")
	}
}

func TestParseRunsRequestOnce(t *testing.T) {
	want := Request{Modes: Incremental | TokenSource}
	calls := 0
	got, err := Parse(want, func(req Request) (Mode, error) {
		calls++
		return req.All(), nil
	})
	if err != nil || calls != 1 || got != want.Modes {
		t.Fatalf("Parse ran %d times and returned %#x, %v; want one run returning %#x", calls, got, err, want.Modes)
	}
}

func TestImpliedModes(t *testing.T) {
	calls := 0
	req := Request{Modes: Incremental, Implied: func() Mode {
		calls++
		return IncludedRanges
	}}
	if got := req.All(); got != Incremental|IncludedRanges {
		t.Fatalf("All() = %#x; want %#x", got, Incremental|IncludedRanges)
	}
	if !req.Has(IncludedRanges) || req.OpenModes() != IncludedRanges || Supports(req) {
		t.Fatal("request methods ignored the implied modes")
	}
	calls = 0
	if Supports(Request{Modes: TokenSource, Implied: req.Implied}) || calls != 0 {
		t.Fatalf("Supports called Implied %d times for a request with an open method mode", calls)
	}
	if !Supports(Request{Modes: Incremental, Implied: func() Mode { return Deadline }}) {
		t.Fatal("Supports rejected a request whose implied modes are all served")
	}
}

func TestParseDoesNotAllocate(t *testing.T) {
	var sink int
	extra := Deadline
	allocs := testing.AllocsPerRun(100, func() {
		req := Request{Modes: Incremental, Implied: func() Mode { return extra }}
		n, _ := Parse(req, func(Request) (int, error) {
			sink++
			return sink, nil
		})
		sink = n
	})
	if allocs != 0 {
		t.Fatalf("Parse allocated %.1f times per call; want 0", allocs)
	}
}

// TestCapabilityFlagsDoc keeps docs/v1-capability-flags.md equal to the
// capability table. Run with -update to rewrite it.
func TestCapabilityFlagsDoc(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "v1-capability-flags.md")
	want := capabilityFlagsDoc()
	if *update {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("%s is stale; run go test ./internal/sched -run '^TestCapabilityFlagsDoc$' -update", path)
	}
}

func capabilityFlagsDoc() string {
	var b strings.Builder
	b.WriteString("# Capability flags\n\n")
	b.WriteString("<!-- Generated by go test ./internal/sched -run '^TestCapabilityFlagsDoc$' -update. Do not edit. -->\n\n")
	fmt.Fprintf(&b, "Capability flags open: %d.\n\n", OpenCount())
	b.WriteString("Every public parse method builds one `internal/sched.Request` and calls `sched.Parse`.\n")
	b.WriteString("The request names the modes that the call needs.\n")
	b.WriteString("This table says whether the compact engine serves each mode.\n")
	b.WriteString("A mode that compact does not serve is an open flag: only the legacy engine serves a request that needs it.\n")
	b.WriteString("The table does not decide policy. The admission switch, the allowlist, per-parser overrides, sub-parser pins, and nested-parse suppression stay in the root package.\n\n")
	b.WriteString("| Flag | Compact serves it | Closes with | Detail |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, row := range capabilities {
		serves := "no (open)"
		if row.Compact {
			serves = "yes"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", row.Name, serves, row.Closes, row.Detail)
	}
	return b.String()
}
