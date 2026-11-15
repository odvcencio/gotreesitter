// Command ccorecert writes internal/ccore/certified/gen.go from
// testdata/ccore_certificates.json. Run it from the repository root.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certgen"
)

func main() {
	raw, err := os.ReadFile("testdata/ccore_certificates.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "ccorecert:", err)
		os.Exit(1)
	}
	var recs []certgen.Record
	if err := json.Unmarshal(raw, &recs); err != nil {
		fmt.Fprintln(os.Stderr, "ccorecert:", err)
		os.Exit(1)
	}
	// The configuration hash comes from the loaded blob, not from the file.
	for i := range recs {
		found := false
		for _, e := range grammars.AllLanguages() {
			l := e.Language()
			if l == nil {
				continue
			}
			if sha, ok := l.GrammarBlobSHA256(); ok && fmt.Sprintf("%x", sha) == strings.ToLower(recs[i].BlobSHA256) {
				h, err := bind.ConfigHash(l)
				if err != nil {
					fmt.Fprintf(os.Stderr, "ccorecert: blob %s: %v\n", recs[i].BlobSHA256, err)
					os.Exit(1)
				}
				recs[i].ConfigSHA256 = fmt.Sprintf("%x", h)
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(os.Stderr, "ccorecert: no loaded language has blob %s\n", recs[i].BlobSHA256)
			os.Exit(1)
		}
	}
	out, err := certgen.Render(recs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ccorecert:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("internal/ccore/certified/gen.go", out, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "ccorecert:", err)
		os.Exit(1)
	}
}
