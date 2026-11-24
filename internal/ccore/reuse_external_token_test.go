package ccore_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

// TestReuseExternalTokenFromHead runs the benchmark edit session on Python and
// Bash, two grammars whose external scanners put external tokens inside reused
// internal nodes, with the cross-check of the reuse cursor's external-token
// shortcut switched on. Every use of the shortcut must give the token that the
// walk down the node gives, the session must use the shortcut, and every
// incremental tree must equal a fresh parse of the same text.
func TestReuseExternalTokenFromHead(t *testing.T) {
	for _, name := range []string{"python", "bash"} {
		entry := grammars.DetectLanguageByName(name)
		if entry == nil || entry.Language() == nil {
			t.Fatalf("language %q unavailable", name)
		}
		p, err := bind.New(entry.Language())
		if err != nil {
			t.Fatal(err)
		}
		fresh, err := bind.New(entry.Language())
		if err != nil {
			t.Fatal(err)
		}
		p.SetReuseCheck(true)
		sample, err := os.ReadFile(filepath.Join("..", "benchfixtures", "testdata", "real", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.HasSuffix(sample, []byte("\n")) {
			sample = append(sample, '\n')
		}
		src := bytes.Repeat(sample, 4)
		tree, stop := p.Parse(src)
		if tree == nil || stop != "" {
			t.Fatalf("%s: first parse stopped: %q", name, stop)
		}
		steps := 0
		for _, step := range benchfixtures.EditingSession(src) {
			e := step.Edit
			tree.Edit(ccore.Edit{
				StartByte: e.StartByte, OldEndByte: e.OldEndByte, NewEndByte: e.NewEndByte,
				StartRow: e.StartPoint.Row, StartCol: e.StartPoint.Column,
				OldEndRow: e.OldEndPoint.Row, OldEndCol: e.OldEndPoint.Column,
				NewEndRow: e.NewEndPoint.Row, NewEndCol: e.NewEndPoint.Column,
			})
			next, stop := p.ParseWith(step.Source, tree)
			if next == nil || stop != "" {
				t.Fatalf("%s step %d: incremental parse stopped: %q", name, steps, stop)
			}
			want, stop := fresh.Parse(step.Source)
			if want == nil || stop != "" {
				t.Fatalf("%s step %d: fresh parse stopped: %q", name, steps, stop)
			}
			if p.DigestOf(next, false) != fresh.DigestOf(want, false) {
				t.Errorf("%s step %d: the incremental tree differs from a fresh parse", name, steps)
			}
			want.Release()
			tree.Release()
			tree = next
			steps++
		}
		tree.Release()
		checked, differed := p.ReuseCheckStats()
		t.Logf("%s: %d edits, %d uses of the external-token shortcut checked, %d differed", name, steps, checked, differed)
		if differed != 0 {
			t.Errorf("%s: %d of %d uses of the shortcut gave another token than the walk", name, differed, checked)
		}
		if checked == 0 {
			t.Fatalf("%s: the session never took the shortcut: the check compared nothing", name)
		}
	}
}
