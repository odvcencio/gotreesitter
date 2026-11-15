// Command ccorecert writes internal/ccore/certified/gen.go from
// testdata/ccore_certificates.json. Run it from the repository root.
package main

import (
	"fmt"
	"os"

	"github.com/odvcencio/gotreesitter/internal/ccore/certgen"
	"github.com/odvcencio/gotreesitter/internal/ccore/certgen/certlang"
)

func main() {
	raw, err := os.ReadFile("testdata/ccore_certificates.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "ccorecert:", err)
		os.Exit(1)
	}
	// The configuration hash comes from the loaded blob, not from the file.
	recs, err := certlang.Load(raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ccorecert:", err)
		os.Exit(1)
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
