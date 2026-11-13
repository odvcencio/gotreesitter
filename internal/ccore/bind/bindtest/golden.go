package bindtest

import (
	"fmt"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest"
)

// RecordGolden parses src with the core and returns a golden without the C
// trace (the harness fills Trace from C's logger).
func RecordGolden(name string, lang any, src []byte) (*ccoretest.Golden, error) {
	cfg, err := bind.Config(lang)
	if err != nil {
		return nil, err
	}
	p, err := ccore.NewParser(cfg)
	if err != nil {
		return nil, fmt.Errorf("bind: %w", err)
	}
	t, stop := p.Parse(src)
	g := &ccoretest.Golden{Name: name, Src: append([]byte(nil), src...), Tables: cfg.Tables, Digest: p.Digest(t), Stop: stop}
	if t != nil {
		g.RecordHash, g.RecordCount = p.Records(t, nil)
		t.Release()
	}
	return g, nil
}
