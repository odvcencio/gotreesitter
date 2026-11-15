package bindtest

import (
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

// KnownDifference is one line of the known-differences file
// (testdata/ccore_route_known_differences.txt of the repository root).
//
// Three kinds of line exist:
//
//	<language> <blob sha256> <note>            Kind "tree": the core's tree differs from C on a committed input;
//	                                           the route-on test runs do not certify the blob.
//	@query <language> <blob sha256> <pattern>  Kind "query": the query matcher gives other captures than C for the pattern
//	                                           on the language's sample; the blob stays certified.
//	@states <language> key=value ...           Kind "states": exact counts of the documented mapping differences
//	                                           between the public tree and C on the language's sample.
//	@ranges <language> key=value ...           Kind "ranges": exact counts of the changed-range comparisons of the
//	                                           language's edit session (route, present engine, C).
//	@flags <language> key=value ...            Kind "flags": exact counts of the edited-flag differences of the session.
type KnownDifference struct {
	Kind     string // "tree", "query", "states", "ranges" or "flags"
	Language string
	Hash     [32]byte          // "tree" and "query"
	Note     string            // "tree": the note; "query": the pattern
	Counts   map[string]string // "states", "ranges", "flags": the key=value fields
}

// ReadKnownDifferences parses the known-differences file.
func ReadKnownDifferences(path string) ([]KnownDifference, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []KnownDifference
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		kd := KnownDifference{Kind: "tree"}
		switch f[0] {
		case "@states", "@ranges", "@flags":
			if len(f) < 3 {
				return nil, fmt.Errorf("bindtest: bad line %q", line)
			}
			kd.Kind, kd.Language, kd.Counts = f[0][1:], f[1], map[string]string{}
			for _, kv := range f[2:] {
				k, v, ok := strings.Cut(kv, "=")
				if !ok {
					return nil, fmt.Errorf("bindtest: bad field %q in %q", kv, line)
				}
				kd.Counts[k] = v
			}
			out = append(out, kd)
			continue
		case "@query":
			if len(f) < 4 {
				return nil, fmt.Errorf("bindtest: bad line %q", line)
			}
			kd.Kind = "query"
			f = f[1:]
			kd.Note = strings.Join(f[2:], " ")
		default:
			if len(f) < 2 {
				return nil, fmt.Errorf("bindtest: bad line %q", line)
			}
			kd.Note = strings.Join(f[2:], " ")
		}
		kd.Language = f[0]
		b, err := hex.DecodeString(f[1])
		if err != nil || len(b) != 32 {
			return nil, fmt.Errorf("bindtest: bad hash in %q", line)
		}
		copy(kd.Hash[:], b)
		out = append(out, kd)
	}
	return out, nil
}

// KnownDifferenceBlobs returns the blob hashes of the "tree" lines of the
// known-differences file. The route-on test runs do not certify them.
func KnownDifferenceBlobs(path string) ([][32]byte, error) {
	all, err := ReadKnownDifferences(path)
	if err != nil {
		return nil, err
	}
	var out [][32]byte
	for _, kd := range all {
		if kd.Kind == "tree" {
			out = append(out, kd.Hash)
		}
	}
	return out, nil
}
