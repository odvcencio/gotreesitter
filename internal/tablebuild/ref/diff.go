package ref

import (
	"fmt"
	"sort"
	"strings"
)

// Divergence is the first record where a port and a reference differ.
type Divergence struct {
	Section   string
	Index     int    // index of the record within its section
	PortCount int    // records of the section in the port
	RefCount  int    // records of the section in the reference
	Port      string // the port's line; "" when the port has no record here
	Ref       string // the reference's line; "" when only digests are known or it has none
	RefDigest string // the reference's digest when only digests are known
	Field     int    // first differing field, counted from 0; -1 when unknown
	PortField string
	RefField  string
}

func clip(s string) string {
	const max = 600
	if len(s) > max {
		return s[:max] + fmt.Sprintf("... (%d bytes)", len(s))
	}
	return s
}

// String names the record and prints both sides.
func (d *Divergence) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "first difference: section %q, record %d (port %d records, reference %d)", d.Section, d.Index, d.PortCount, d.RefCount)
	if d.Field >= 0 {
		fmt.Fprintf(&b, "\n  field %d: port %q, reference %q", d.Field, d.PortField, d.RefField)
	}
	port := d.Port
	if port == "" {
		port = "(none)"
	}
	fmt.Fprintf(&b, "\n  port:      %s", clip(port))
	switch {
	case d.Ref != "":
		fmt.Fprintf(&b, "\n  reference: %s", clip(d.Ref))
	case d.RefDigest != "":
		fmt.Fprintf(&b, "\n  reference: digest %s (the reference is stored as digests)", d.RefDigest)
	default:
		b.WriteString("\n  reference: (none)")
	}
	return b.String()
}

func sortedKeys(maps ...map[string][]string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range maps {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

// firstField returns the first field where two lines differ.
func firstField(a, b string) (int, string, string) {
	fa, fb := strings.Split(a, "\t"), strings.Split(b, "\t")
	for i := 0; i < len(fa) || i < len(fb); i++ {
		var x, y string
		if i < len(fa) {
			x = fa[i]
		}
		if i < len(fb) {
			y = fb[i]
		}
		if x != y {
			return i, x, y
		}
	}
	return -1, "", ""
}

// Compare returns the first record where two record sets differ, or nil.
// Sections are compared in sorted order, records in order within a section.
func Compare(port, ref []string) *Divergence {
	ps, rs := Sections(port), Sections(ref)
	for _, s := range sortedKeys(ps, rs) {
		p, r := ps[s], rs[s]
		for i := 0; i < len(p) || i < len(r); i++ {
			d := &Divergence{Section: s, Index: i, PortCount: len(p), RefCount: len(r), Field: -1}
			if i < len(p) {
				d.Port = p[i]
			}
			if i < len(r) {
				d.Ref = r[i]
			}
			if d.Port == d.Ref {
				continue
			}
			if d.Port != "" && d.Ref != "" {
				d.Field, d.PortField, d.RefField = firstField(d.Port, d.Ref)
			}
			return d
		}
	}
	return nil
}

// CompareDigests returns the first record where the port differs from a
// reference stored as digests, or nil. Every section of either side is
// compared: a caller that leaves a section out must remove it from both.
func CompareDigests(port []string, ref map[string][]string) *Divergence {
	ps := Sections(port)
	for _, s := range sortedKeys(ps, ref) {
		p, r := ps[s], ref[s]
		for i := 0; i < len(p) || i < len(r); i++ {
			d := &Divergence{Section: s, Index: i, PortCount: len(p), RefCount: len(r), Field: -1}
			if i < len(p) {
				d.Port = p[i]
			}
			if i < len(r) {
				d.RefDigest = r[i]
			}
			if d.Port != "" && Digest(d.Port) == d.RefDigest {
				continue
			}
			return d
		}
	}
	return nil
}
