package ref

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Digest is the first 8 bytes of the SHA-256 of a record line, in hex.
func Digest(line string) string {
	sum := sha256.Sum256([]byte(line))
	return hex.EncodeToString(sum[:8])
}

// SectionOf returns the section of a record line: its layer and kind.
func SectionOf(line string) string {
	f := strings.SplitN(line, "\t", 3)
	if len(f) < 2 {
		return line
	}
	return f[0] + " " + f[1]
}

// Sections groups record lines by section, keeping their order.
func Sections(lines []string) map[string][]string {
	out := map[string][]string{}
	for _, l := range lines {
		s := SectionOf(l)
		out[s] = append(out[s], l)
	}
	return out
}

// SectionDigests is Sections with each line replaced by its digest.
func SectionDigests(lines []string) map[string][]string {
	out := map[string][]string{}
	for _, l := range lines {
		s := SectionOf(l)
		out[s] = append(out[s], Digest(l))
	}
	return out
}

// DigestFile is the committed form of a reference (pure-Go tier). Sections
// holds the generator's output: L0 and L1 node types from the pinned inputs,
// L3 from parser-noopt.c, L1, L2, L4 and L5 from parser.c, and L6 from the
// pinned parser.c. Eligible reports whether the generator rewrote the pinned
// parser.c byte for byte apart from the version comment. Pinned holds the L1,
// L2, L4 and L5 sections of the pinned parser.c when its records differ from
// the generator's; it is empty otherwise.
type DigestFile struct {
	Grammar   string              `json:"grammar"`
	Commit    string              `json:"commit"`
	Generator string              `json:"generator"`
	Eligible  bool                `json:"eligible"`
	Sources   map[string]string   `json:"sources"`
	Sections  map[string][]string `json:"sections"`
	Pinned    map[string][]string `json:"pinned,omitempty"`
}

// Marshal writes the file with sorted keys and one line per section.
func (f *DigestFile) Marshal() []byte {
	var b bytes.Buffer
	q := func(s string) string { x, _ := json.Marshal(s); return string(x) }
	writeMap := func(name string, m map[string][]string, last bool) {
		fmt.Fprintf(&b, "  %s: {", q(name))
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for i, k := range keys {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, "\n    %s: [", q(k))
			for j, d := range m[k] {
				if j > 0 {
					b.WriteString(",")
				}
				b.WriteString(q(d))
			}
			b.WriteString("]")
		}
		if len(keys) > 0 {
			b.WriteString("\n  ")
		}
		b.WriteString("}")
		if !last {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  \"grammar\": %s,\n  \"commit\": %s,\n  \"generator\": %s,\n  \"eligible\": %v,\n", q(f.Grammar), q(f.Commit), q(f.Generator), f.Eligible)
	srcKeys := make([]string, 0, len(f.Sources))
	for k := range f.Sources {
		srcKeys = append(srcKeys, k)
	}
	sort.Strings(srcKeys)
	b.WriteString("  \"sources\": {")
	for i, k := range srcKeys {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "\n    %s: %s", q(k), q(f.Sources[k]))
	}
	if len(srcKeys) > 0 {
		b.WriteString("\n  ")
	}
	b.WriteString("},\n")
	writeMap("sections", f.Sections, len(f.Pinned) == 0)
	if len(f.Pinned) > 0 {
		writeMap("pinned", f.Pinned, true)
	}
	b.WriteString("}\n")
	return b.Bytes()
}

// ReadDigestFile reads a committed digest file.
func ReadDigestFile(path string) (*DigestFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f DigestFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &f, nil
}
