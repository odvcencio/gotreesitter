package ccore

import (
	"strings"
	"testing"
)

// newTestParser returns a parser over a minimal table (symbols 0 to 15, all
// hidden) with a fresh store, ready for the allocation tests.
func newTestParser(t *testing.T) *Parser {
	t.Helper()
	p := &Parser{t: &Tables{Meta: make([]SymbolMeta, 16), TokenCount: 4}}
	p.Store = newStoreCore().view()
	p.beginParse(true)
	return p
}

// catchLimit runs f and returns the limitError it panics with (nil if none).
func catchLimit(f func()) (le *limitError) {
	defer func() {
		if r := recover(); r != nil {
			var ok bool
			if le, ok = r.(*limitError); !ok {
				panic(r)
			}
		}
	}()
	f()
	return nil
}

func zeroLeaf(p *Parser, sym Symbol) Handle {
	return p.newLeaf(sym, Length{}, Length{}, 0, 0, false, false, false)
}

// A record that is refused by the record-index limit strands no child list: the
// node reserves its record before it allocates the list it will own.
func TestRefusedRecordStrandsNoChildList(t *testing.T) {
	p := newTestParser(t)
	c := p.core
	// Heap leaves (300 bytes does not fit an inline leaf), so that a record page exists.
	a := p.newLeaf(1, Length{}, Length{bytes: 300}, 0, 0, false, false, false)
	b := p.newLeaf(2, Length{}, Length{bytes: 300}, 0, 0, false, false, false)
	c.recLimit.Store(uint32(c.pc.recPage)<<recShift | uint32(c.pc.recSlot) - 1) // the next record is over the limit
	page, off := c.pc.kidPage, c.pc.kidOff
	for i := 0; i < 5; i++ {
		le := catchLimit(func() { p.newNode(5, []Handle{a, b}, 0) })
		if le == nil || !strings.Contains(le.Error(), "record index limit") {
			t.Fatalf("attempt %d: limit error %v", i, le)
		}
		if c.pc.kidPage != page || c.pc.kidOff != off {
			t.Fatalf("attempt %d: a refused record took a child list (page %d offset %d, was %d and %d)", i, c.pc.kidPage, c.pc.kidOff, page, off)
		}
	}
}

// The child-page limit refuses a list whose start offset would reach largeBit,
// before the page is claimed.
func TestChildPageLimit(t *testing.T) {
	c := newStoreCore()
	s := c.view()
	c.kidPageLimit.Store(1)
	var refused *limitError
	for i := 0; i < 600 && refused == nil; i++ { // 512 lists of 8 fill one page
		refused = catchLimit(func() { s.allocKids(&c.pc, 8, false) })
	}
	if refused == nil || !strings.Contains(refused.Error(), "child page limit") {
		t.Fatalf("limit error %v", refused)
	}
	if len(c.kids) != 1 {
		t.Fatalf("%d child pages claimed, want 1", len(c.kids))
	}
}

// The scanner-state page limit refuses an offset that would wrap, before the
// page is claimed.
func TestStatePageLimit(t *testing.T) {
	c := newStoreCore()
	s := c.view()
	c.statePageLimit.Store(1)
	var refused *limitError
	for i := 0; i < 40 && refused == nil; i++ { // 16 blocks of 4,096 fill one page
		refused = catchLimit(func() { s.allocState(&c.pc, 4096, false) })
	}
	if refused == nil || !strings.Contains(refused.Error(), "state page limit") {
		t.Fatalf("limit error %v", refused)
	}
	if len(c.state) != 1 {
		t.Fatalf("%d state pages claimed, want 1", len(c.state))
	}
}
