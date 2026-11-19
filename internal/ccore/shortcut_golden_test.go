package ccore_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest"
)

func TestShortcutGolden(t *testing.T) {
	g, err := ccoretest.Read("testdata/golden", "testdata/tables", "json-empty-array.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, old, src string
		pos            int
		oldLen, newLen int
		second, logger bool
		wantTaken      bool
		wantReason     string
	}{
		{"digit", "[123,456]", "[183,456]", 2, 1, 1, false, false, true, ""},
		{"quote", "[123,456]", "[\"23,456]", 1, 1, 1, false, false, false, "relex-differs"},
		{"bracket", "[123,456]", "[123,456}", 8, 1, 1, false, false, false, "relex-differs"},
		{"neighbor lookahead", "[12,3]", "[1243]", 3, 1, 1, false, false, false, "relex-differs"},
		{"newline", "[123]", "[1\n3]", 2, 1, 1, false, false, false, "newline"},
		{"length", "[123]", "[1223]", 2, 1, 2, false, false, false, "not-same-length"},
		{"two edits", "[123]", "[183]", 2, 1, 1, true, false, false, "no-memo"},
		{"logger", "[123]", "[183]", 2, 1, 1, false, true, false, "logger"},
		{"error", "[1,]", "[2,]", 1, 1, 1, false, false, false, "root-extent-or-error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
			if err != nil {
				t.Fatal(err)
			}
			p.SetShortcut(true)
			old, stop := p.Parse([]byte(tc.old))
			if stop != "" || old == nil {
				t.Fatalf("old parse: %q", stop)
			}
			defer old.Release()
			e := ccore.Edit{StartByte: uint32(tc.pos), OldEndByte: uint32(tc.pos + tc.oldLen), NewEndByte: uint32(tc.pos + tc.newLen), StartCol: uint32(tc.pos), OldEndCol: uint32(tc.pos + tc.oldLen), NewEndCol: uint32(tc.pos + tc.newLen)}
			old.Edit(e)
			if tc.second {
				old.Edit(e)
			}
			control := old.Copy()
			defer control.Release()
			if tc.logger {
				p.SetLogger(func(string) {})
			}
			next, stop := p.ParseWith([]byte(tc.src), old)
			if stop != "" || next == nil {
				t.Fatalf("reparse: %q", stop)
			}
			defer next.Release()
			if got := p.ShortcutTaken(); got != tc.wantTaken {
				t.Fatalf("taken %v, want %v (reason %q)", got, tc.wantTaken, p.ShortcutWhy())
			}
			if got := p.ShortcutWhy(); got != tc.wantReason {
				t.Errorf("reason %q, want %q", got, tc.wantReason)
			}
			if p.ShortcutCount(map[bool]string{true: "taken", false: tc.wantReason}[tc.wantTaken]) != 1 {
				t.Error("outcome counter not updated")
			}
			p.SetLogger(nil)
			p.SetShortcut(false)
			normal, stop := p.ParseWith([]byte(tc.src), control)
			if stop != "" || normal == nil {
				t.Fatalf("normal reparse: %q", stop)
			}
			defer normal.Release()
			if p.Digest(next) != p.Digest(normal) {
				t.Error("result differs from normal reparse")
			}
			fresh, stop := p.Parse([]byte(tc.src))
			if stop != "" || fresh == nil {
				t.Fatalf("fresh: %q", stop)
			}
			defer fresh.Release()
			if tc.wantTaken && p.Digest(next) != p.Digest(fresh) {
				t.Error("shortcut differs from fresh parse")
			}
		})
	}
}

func TestShortcutTokenCounters(t *testing.T) {
	g, err := ccoretest.Read("testdata/golden", "testdata/tables", "json-empty-array.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, src, reason string
		taken             bool
	}{
		{"taken", "[183,456]", "", true},
		{"declined", "[1\"3,456]", "relex-differs", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
			if err != nil {
				t.Fatal(err)
			}
			p.SetShortcut(true)
			old, stop := p.Parse([]byte("[123,456]"))
			if stop != "" || old == nil {
				t.Fatalf("old parse: %q", stop)
			}
			defer old.Release()
			old.Edit(ccore.Edit{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartCol: 2, OldEndCol: 3, NewEndCol: 3})
			control := old.Copy()
			defer control.Release()
			next, stop := p.ParseWith([]byte(tc.src), old)
			if stop != "" || next == nil {
				t.Fatalf("reparse: %q", stop)
			}
			defer next.Release()
			if p.ShortcutTaken() != tc.taken || p.ShortcutWhy() != tc.reason {
				t.Fatalf("taken=%t reason=%q", p.ShortcutTaken(), p.ShortcutWhy())
			}
			got := p.Stats()
			if tc.taken {
				if got.LexerCalls < 1 || got.Tokens != got.LexerCalls || p.Tokens() != got.LexerCalls || got.Shifts != 0 || got.Reductions != 0 {
					t.Fatalf("shortcut stats: %+v, Tokens()=%d", got, p.Tokens())
				}
				return
			}
			p.SetShortcut(false)
			normal, stop := p.ParseWith([]byte(tc.src), control)
			if stop != "" || normal == nil {
				t.Fatalf("normal reparse: %q", stop)
			}
			defer normal.Release()
			baseline := p.Stats()
			attemptCalls := got.LexerCalls - baseline.LexerCalls
			if attemptCalls < 1 || got.Tokens-baseline.Tokens != attemptCalls {
				t.Fatalf("attempt calls=%d, tokens with shortcut=%d without=%d; stats=%+v baseline=%+v", attemptCalls, got.Tokens, baseline.Tokens, got, baseline)
			}
		})
	}
}

// The shortcut lexes the leaves that the edit marked as changed and no other.
// C's edit does not mark a token that starts where the edit ends, so that token
// is not lexed again; it does mark a token whose lookahead reaches the first
// edited byte.
func TestShortcutLexesOnlyMarkedLeaves(t *testing.T) {
	g, err := ccoretest.Read("testdata/golden", "testdata/tables", "json-empty-array.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		at    uint32
		lexed int64
	}{
		{"middle_digit", 2, 1},
		{"last_digit_before_comma", 3, 1},
		{"first_digit_after_bracket", 1, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
			if err != nil {
				t.Fatal(err)
			}
			p.SetShortcut(true)
			src := []byte("[123,456]")
			old, stop := p.Parse(src)
			if stop != "" || old == nil {
				t.Fatalf("old parse: %q", stop)
			}
			defer old.Release()
			edited := append([]byte(nil), src...)
			edited[tc.at] = '9'
			old.Edit(ccore.Edit{StartByte: tc.at, OldEndByte: tc.at + 1, NewEndByte: tc.at + 1, StartCol: tc.at, OldEndCol: tc.at + 1, NewEndCol: tc.at + 1})
			next, stop := p.ParseWith(edited, old)
			if stop != "" || next == nil {
				t.Fatalf("reparse: %q", stop)
			}
			defer next.Release()
			if !p.ShortcutTaken() {
				t.Fatalf("shortcut not taken: %q", p.ShortcutWhy())
			}
			if got := p.Stats(); int64(got.LexerCalls) != tc.lexed || int64(got.Tokens) != tc.lexed {
				t.Fatalf("lexed %d tokens in %d calls, want %d", got.Tokens, got.LexerCalls, tc.lexed)
			}
		})
	}
}

// A shortcut reparse does no balancing, so it reports no compressions even
// when the parse before it balanced a long list.
func TestShortcutReportsNoCompressionsOfAnEarlierParse(t *testing.T) {
	g, err := ccoretest.Read("testdata/golden", "testdata/tables", "json-empty-array.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
	if err != nil {
		t.Fatal(err)
	}
	p.SetShortcut(true)
	src := []byte("[" + strings.Repeat("11,", 400) + "11]")
	old, stop := p.Parse(src)
	if stop != "" || old == nil {
		t.Fatalf("old parse: %q", stop)
	}
	defer old.Release()
	if p.Stats().Compressions == 0 {
		t.Fatal("setup: the first parse balanced nothing, so the test would prove nothing")
	}
	edited := append([]byte(nil), src...)
	edited[1] = '9'
	old.Edit(ccore.Edit{StartByte: 1, OldEndByte: 2, NewEndByte: 2, StartCol: 1, OldEndCol: 2, NewEndCol: 2})
	next, stop := p.ParseWith(edited, old)
	if stop != "" || next == nil {
		t.Fatalf("reparse: %q", stop)
	}
	defer next.Release()
	if !p.ShortcutTaken() {
		t.Fatalf("shortcut not taken: %q", p.ShortcutWhy())
	}
	if got := p.Stats(); got.Compressions != 0 || got.Shifts != 0 || got.Reductions != 0 {
		t.Fatalf("shortcut stats carry work of the earlier parse: %+v", got)
	}
}

func TestShortcutEditReleaseRace(t *testing.T) {
	g, err := ccoretest.Read("testdata/golden", "testdata/tables", "json-empty-array.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
	if err != nil {
		t.Fatal(err)
	}
	p.SetShortcut(true)
	bad, doubles := p.BadRetain(), p.DoubleFree()
	for i := 0; i < 30; i++ {
		old, stop := p.Parse([]byte("[123,456]"))
		if old == nil || stop != "" {
			t.Fatalf("parse: %q", stop)
		}
		backup := old.Copy()
		e := ccore.Edit{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartCol: 2, OldEndCol: 3, NewEndCol: 3}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); old.Edit(e) }()
		go func() { defer wg.Done(); old.Release() }()
		wg.Wait()
		backup.Edit(e)
		next, stop := p.ParseWith([]byte("[183,456]"), backup)
		if next == nil || stop != "" || !p.ShortcutTaken() {
			t.Fatalf("reparse %d: stop %q reason %q", i, stop, p.ShortcutWhy())
		}
		next.Release()
		backup.Release()
	}
	if p.BadRetain() != bad || p.DoubleFree() != doubles {
		t.Fatalf("store lifetime counters changed: bad retain %d, double free %d", p.BadRetain()-bad, p.DoubleFree()-doubles)
	}
}

func TestShortcutLedgerRefusalLeavesOldTreeUsable(t *testing.T) {
	g, err := ccoretest.Read("testdata/golden", "testdata/tables", "json-empty-array.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ccore.NewParser(ccore.Config{Tables: g.Tables})
	if err != nil {
		t.Fatal(err)
	}
	p.SetShortcut(true)
	src := []byte("[" + strings.Repeat("123,", 70000) + "123]")
	old, stop := p.Parse(src)
	if old == nil || stop != "" {
		t.Fatalf("parse: %q", stop)
	}
	defer old.Release()
	blocked := old.Copy()
	oldRoot := blocked.Root()
	var editLedger ccore.Ledger
	editLedger.SetPoll(func(total int64, _ bool) bool { return total < int64(len(src)) })
	if !editLedger.Begin(0) {
		t.Fatal("empty edit account refused")
	}
	if blocked.EditWithin(ccore.Edit{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartCol: 2, OldEndCol: 3, NewEndCol: 3}, &editLedger) {
		t.Fatal("edit accepted without accounting for its retained source")
	}
	if blocked.Root() != oldRoot || blocked.StoreFailure() != "" {
		t.Fatal("refusal changed the tree or poisoned its store")
	}
	blocked.Release()
	src[2] = '8'
	old.Edit(ccore.Edit{StartByte: 2, OldEndByte: 3, NewEndByte: 3, StartCol: 2, OldEndCol: 3, NewEndCol: 3})
	refused := false
	p.Ledger().SetPoll(func(total int64, _ bool) bool {
		if total-p.Ledger().Base >= int64(len(src)) {
			refused = true
			return false
		}
		return true
	})
	next, stop := p.ParseWith(src, old)
	if next != nil || stop != "stopped" || !refused {
		t.Fatalf("tree=%v stop=%q refused=%v", next, stop, refused)
	}
	p.Ledger().SetPoll(nil)
	p.SetShortcut(false)
	next, stop = p.ParseWith(src, old)
	if next == nil || stop != "" {
		t.Fatalf("after refusal: %q", stop)
	}
	next.Release()
}
