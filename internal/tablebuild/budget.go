package tablebuild

import (
	"sync/atomic"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// Size budgets of this package (see the package documentation). The
// generator has none. A pass checks the value that the item it is about to
// build would reach, and refuses the grammar with a *lexbuild.SizeError if it
// passes the limit. The limits are variables so that tests can lower them.
type budget struct {
	what  string
	limit int64
	peak  atomic.Int64
}

var (
	bInlineSteps   = &budget{what: "production steps built by inlining", limit: 1 << 20}
	bInlineScratch = &budget{what: "production steps held while inlining one step of one production", limit: 1 << 20}
	bInlineWork    = &budget{what: "steps of the inline work list", limit: 1 << 26}
	bAliasWork     = &budget{what: "steps compared while removing redundant default aliases", limit: 1 << 30}

	bParseStates   = &budget{what: "parse states before merging", limit: 1 << 19}
	bTableEntries  = &budget{what: "entries of the parse table before merging", limit: 1 << 23}
	bClosureWork   = &budget{what: "item insertions by the closures of the parse states", limit: 1 << 28}
	bLookaheadSets = &budget{what: "distinct lookahead sets", limit: 1 << 18}
)

// allBudgets lists the budgets in the order of the documentation table.
var allBudgets = []*budget{bInlineSteps, bInlineScratch, bInlineWork, bAliasWork, bParseStates, bTableEntries, bClosureWork, bLookaheadSets}

type tooLarge struct{ err *lexbuild.SizeError }

// check records n as a value seen and refuses it if it passes the limit.
func (b *budget) check(n int) {
	v := int64(n)
	for {
		old := b.peak.Load()
		if v <= old || b.peak.CompareAndSwap(old, v) {
			break
		}
	}
	if v > b.limit {
		panic(tooLarge{&lexbuild.SizeError{What: b.what, Limit: b.limit}})
	}
}

// checkWork is check for a running count that may pass the range of int.
func (b *budget) checkWork(v int64) {
	for {
		old := b.peak.Load()
		if v <= old || b.peak.CompareAndSwap(old, v) {
			break
		}
	}
	if v > b.limit {
		panic(tooLarge{&lexbuild.SizeError{What: b.what, Limit: b.limit}})
	}
}

// guardSize runs f and turns the panic of a passed size limit into an error.
func guardSize(f func()) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if l, ok := r.(tooLarge); ok {
				err = l.err
				return
			}
			panic(r)
		}
	}()
	f()
	return nil
}
