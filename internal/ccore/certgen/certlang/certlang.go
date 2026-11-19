// Package certlang completes certificate records from the grammars the module
// registers: it finds the loaded language of a blob and takes its configuration
// hash. The generator (cmd/ccorecert) and the test that checks the generated
// list both use it, so the two cannot compute the hash in different ways.
package certlang

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certgen"
)

// Load parses the certificate records and sets ConfigSHA256 of each from the
// registered language that has the record's blob. A value of ConfigSHA256 in
// the input is ignored: the hash comes from the loaded blob, never from the
// file. A blob that no registered language has is an error.
func Load(raw []byte) ([]certgen.Record, error) {
	recs, err := certgen.Parse(raw)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return recs, nil
	}
	hashes := map[string]string{}
	for _, e := range grammars.AllLanguages() {
		l := e.Language()
		if l == nil {
			continue
		}
		sha, ok := l.GrammarBlobSHA256()
		if !ok {
			continue
		}
		blob := fmt.Sprintf("%x", sha)
		wanted := false
		for i := range recs {
			if strings.ToLower(recs[i].BlobSHA256) == blob {
				wanted = true
				break
			}
		}
		if !wanted {
			continue
		}
		h, err := bind.ConfigHash(l)
		if err != nil {
			return nil, fmt.Errorf("blob %s: %w", blob, err)
		}
		hashes[blob] = fmt.Sprintf("%x", h)
	}
	for i := range recs {
		h, ok := hashes[strings.ToLower(recs[i].BlobSHA256)]
		if !ok {
			return nil, fmt.Errorf("no registered language has blob %s", recs[i].BlobSHA256)
		}
		recs[i].ConfigSHA256 = h
	}
	return recs, nil
}
