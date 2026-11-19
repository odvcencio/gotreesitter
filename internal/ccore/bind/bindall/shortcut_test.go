package bindall

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

func shortcutPoint(src []byte, at int) (row, col uint32) {
	for _, b := range src[:at] {
		if b == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}
	return
}

// TestShortcutRealGrammars compares each taken result with both parse paths.
func TestShortcutRealGrammars(t *testing.T) {
	generated := map[string]string{
		"json":       "[" + strings.Repeat("{\"name\":\"abc\",\"value\":123},", 7) + "{\"name\":\"abc\",\"value\":123}]\n",
		"python":     strings.Repeat("value = 123 + 456\n", 8),
		"c":          strings.Repeat("int value = 123;\n", 8),
		"typescript": strings.Repeat("let value = 123;\n", 8),
	}
	names := []string{"json", "python", "c", "typescript"}
	for _, entry := range grammars.AllLanguages() {
		if _, required := generated[entry.Name]; !required {
			names = append(names, entry.Name)
		}
	}
	for _, name := range names {
		_, required := generated[name]
		entry := grammars.DetectLanguageByName(name)
		if entry == nil {
			t.Fatalf("missing %s", name)
		}
		smoke := grammars.ParseSmokeSample(name)
		repeated := generated[name]
		if !required {
			repeated = strings.Repeat(smoke, 4)
		}
		for _, sample := range []struct{ name, src string }{{"smoke", smoke}, {"generated", repeated}} {
			t.Run(name+"/"+sample.name, func(t *testing.T) {
				p, err := bind.New(entry.Language())
				if err != nil {
					if required || !strings.Contains(err.Error(), scanmode.ReasonScannerNotRegistered) {
						t.Fatal(err)
					}
					t.Skipf("binding unavailable: %v", err)
				}
				p.SetShortcut(true)
				src := []byte(sample.src)
				if len(src) == 0 {
					t.Fatal("empty sample")
				}
				base, stop := p.Parse(src)
				if base == nil || stop != "" {
					t.Fatalf("base: %q", stop)
				}
				defer base.Release()
				rng := rand.New(rand.NewSource(72271))
				declines := map[string]int{}
				taken, diffs := 0, 0
				for i := 0; i < 2000; i++ {
					at := rng.Intn(len(src))
					alphabet := "abcxyz0123456789[]{} ,\""
					b := alphabet[rng.Intn(len(alphabet))]
					if b == src[at] {
						b = '7'
						if b == src[at] {
							b = '8'
						}
					}
					changed := append([]byte(nil), src...)
					changed[at] = b
					row, col := shortcutPoint(src, at)
					endRow, endCol := shortcutPoint(src, at+1)
					e := ccore.Edit{StartByte: uint32(at), OldEndByte: uint32(at + 1), NewEndByte: uint32(at + 1), StartRow: row, StartCol: col, OldEndRow: endRow, OldEndCol: endCol, NewEndRow: row, NewEndCol: col + 1}
					old := base.Copy()
					control := base.Copy()
					old.Edit(e)
					control.Edit(e)
					p.SetShortcut(true)
					next, stop := p.ParseWith(changed, old)
					if next == nil || stop != "" {
						t.Fatalf("edit %d: %q", i, stop)
					}
					if p.ShortcutTaken() {
						taken++
						got := p.Digest(next)
						p.SetShortcut(false)
						normal, stop := p.ParseWith(changed, control)
						if normal == nil || stop != "" {
							t.Fatalf("control %d: %q", i, stop)
						}
						fresh, stop := p.Parse(changed)
						if fresh == nil || stop != "" {
							t.Fatalf("fresh %d: %q", i, stop)
						}
						if got != p.Digest(normal) || got != p.Digest(fresh) {
							diffs++
							t.Errorf("edit %d at %d (%q to %q): shortcut differs", i, at, src[at], b)
						}
						normal.Release()
						fresh.Release()
					} else {
						declines[p.ShortcutWhy()]++
					}
					next.Release()
					old.Release()
					control.Release()
				}
				fmt.Printf("SHORTCUT lang=%s sample=%s edits=2000 taken=%d declined=%v differences=%d\n", name, sample.name, taken, declines, diffs)
				if required && sample.name == "generated" && taken == 0 {
					t.Fatal("no shortcut taken")
				}
			})
		}
	}
}

func TestShortcutScannerAndIdentifier(t *testing.T) {
	entry := grammars.DetectLanguageByName("python")
	p, err := bind.New(entry.Language())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, src, from, to string
		taken               bool
	}{
		{"scanner state", "if True:\n    value = 123\n", "2", "8", true},
		{"identifier end", "if True:\n    value = 123\n", "u", " ", false},
		{"indent change", "if True:\n    value = 123\n", "    value", "   xvalue", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p.SetShortcut(true)
			old, stop := p.Parse([]byte(tc.src))
			if old == nil || stop != "" {
				t.Fatalf("parse: %q", stop)
			}
			defer old.Release()
			at := strings.Index(tc.src, tc.from)
			if at < 0 {
				t.Fatal("fixture not found")
			}
			if tc.name == "scanner state" {
				rd := old.Reader()
				var pos uint32
				found := false
				var walk func(ccore.Handle)
				walk = func(h ccore.Handle) {
					kids := rd.Children(h)
					if len(kids) != 0 {
						for _, child := range kids {
							walk(child)
						}
						return
					}
					pos += rd.Padding(h).Bytes() + rd.Size(h).Bytes()
					if pos <= uint32(at) && len(rd.ExtState(h)) > 0 {
						found = true
					}
				}
				walk(old.Root())
				if !found {
					t.Fatal("fixture has no preceding external scanner state")
				}
			}
			src := []byte(tc.src)
			copy(src[at:at+len(tc.to)], tc.to)
			row, col := shortcutPoint([]byte(tc.src), at)
			e := ccore.Edit{StartByte: uint32(at), OldEndByte: uint32(at + len(tc.from)), NewEndByte: uint32(at + len(tc.to)), StartRow: row, StartCol: col, OldEndRow: row, OldEndCol: col + uint32(len(tc.from)), NewEndRow: row, NewEndCol: col + uint32(len(tc.to))}
			old.Edit(e)
			next, stop := p.ParseWith(src, old)
			if next == nil || stop != "" {
				t.Fatalf("reparse: %q", stop)
			}
			defer next.Release()
			if p.ShortcutTaken() != tc.taken {
				t.Fatalf("taken=%v reason=%q", p.ShortcutTaken(), p.ShortcutWhy())
			}
			fresh, stop := p.Parse(src)
			if fresh == nil || stop != "" {
				t.Fatalf("fresh: %q", stop)
			}
			defer fresh.Release()
			if tc.taken && p.Digest(next) != p.Digest(fresh) {
				t.Fatal("scanner shortcut differs from fresh")
			}
		})
	}
}
