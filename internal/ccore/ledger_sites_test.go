package ccore

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"
)

// servedFiles are the files of this package whose code runs while a served request parses.
// Every other non-test file must be listed in notServedFiles with the reason, so that a new
// file cannot add an allocation to the served path without being scanned.
var servedFiles = []string{"parser.go", "subtree.go", "stack.go", "stack_summary.go", "recover.go", "lexer.go", "lexdfa.go", "included_lexer.go", "forest_transport.go", "scanner.go", "reduce_linear.go", "reuse.go", "store.go", "lifetime.go", "edit.go", "tree.go", "api.go", "store_ledger.go", "shortcut.go", "scanprovider.go"}

var notServedFiles = map[string]string{
	"changed_ranges.go": "compares finished trees outside a parse request",
	"doc.go":            "documentation",
	"field_answers.go":  "field answers of one node occurrence: read after the request has ended, into caller-owned storage",
	"field_lookup.go":   "field lookup paths: read after the request has ended, into a caller-owned buffer",
	"handle.go":         "handle encoding; no allocation",
	"ledger.go":         "the ledger itself",
	"records.go":        "record types; the arrays are made through the ledger",
	"node.go":           "value-node readers (cursor, field search): they run when a caller reads a tree, after the request has ended",
	"tables.go":         "tables are built once per binding",
	"trace.go":          "diagnostic switch",
	"validate.go":       "validation of bound tables, once per binding",
	"walk.go":           "tree walks used by tests and the conversion's read-only checks",

	"scandiff_cursor_probe.go": "standalone scanner comparison built only with the C parity tag",
}

// A non-test file in the package that is neither scanned nor listed with a reason fails.
func TestEveryCoreFileIsScannedOrListed(t *testing.T) {
	ents, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	scanned := map[string]bool{}
	for _, f := range servedFiles {
		scanned[f] = true
	}
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		if _, ok := notServedFiles[n]; !ok && !scanned[n] {
			t.Errorf("%s is neither in servedFiles (scanned for append and make) nor in notServedFiles", n)
		}
	}
	for n := range notServedFiles {
		if scanned[n] {
			t.Errorf("%s is in both lists", n)
		}
		if _, err := os.Stat(n); err != nil {
			t.Errorf("notServedFiles names %s, which does not exist", n)
		}
	}
}

// exempt lists, by "file:function", the functions of served files that call append, make or
// new without the ledger helpers, with the reason. Everything else in a served file must
// go through the helpers of ledger.go. An entry whose function does not exist, or no
// longer allocates, fails: the list cannot go stale.
var exempt = map[string]string{
	// pages: the page itself is reserved first by chargePage, with the bytes of its page-table entries
	"subtree.go:addMulti": "map buckets are reserved before each new key by addMulti",
	"store.go:growRec":    "page and its table entries; chargePage reserves the page first",
	"store.go:growKids":   "page and its table entry; chargePage reserves the page first",
	"store.go:growState":  "page and its table entry; chargePage reserves the page first",
	"store.go:allocKids":  "a list longer than a page: chargePage reserves its array first",
	"store.go:presize":    "the page tables of a new store: presizeTables (api.go) reserves their bytes first (presizeBytes) and makes the request stop when refused",
	"store.go:closeSpan":  "one 24-byte span per record page, claimed with that page",
	// pool: retention, at most poolMaxBytes (128 MiB), idle pages dropped after 30 s; stated, not charged to a request
	"store.go:putPages": "page pool retention",
	"store.go:trimPool": "page pool retention",
	// reclamation: the free-list batches, the sort scratch and the release walk stack go
	// through fpush and fmake (ledger.go), which count their growth without the possibility of
	// a refusal; the retained capacities are in StoreBytes (scratchBytes)
	"store.go:publish":         "publication walk stack: reserveWalk (api.go) makes it at the size it can need before the walk starts, so these appends do not allocate",
	"lifetime.go:appendOps":    "queued retain and release operations, 24 bytes per operation, outside a request",
	"lifetime.go:drain":        "queue handover during a drain",
	"lifetime.go:retainEdited": "retain list of Tree.Edit, one entry per child of an edited record",
	// edits and tree operations of the caller: they run outside a request
	"edit.go:subtreeEdit": "Tree.Edit: the edit path, two handles per level of the tree",
	"tree.go:drop":        "Tree.Release: one queued operation",
	"tree.go:Copy":        "Tree.Copy: one queued operation",
	"tree.go:edit":        "Tree.Edit: one queued operation",
	"tree.go:commitEdit":  "Tree.Edit: the queued operations of the edit",
	// bound once
	"lexdfa.go:BuildLexAscii": "built once per binding",
}

// guardedAppends recognizes an append immediately preceded by a capacity check
// which grows the same destination through the ledger.
func guardedAppends(fd *ast.FuncDecl, fset *token.FileSet) map[token.Pos]bool {
	result := map[token.Pos]bool{}
	expr := func(e ast.Expr) string { var b bytes.Buffer; _ = format.Node(&b, fset, e); return b.String() }
	ast.Inspect(fd, func(n ast.Node) bool {
		block, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i := 1; i < len(block.List); i++ {
			a, ok := block.List[i].(*ast.AssignStmt)
			if !ok || len(a.Lhs) != 1 || len(a.Rhs) != 1 {
				continue
			}
			call, ok := a.Rhs[0].(*ast.CallExpr)
			if !ok || len(call.Args) < 2 {
				continue
			}
			id, ok := call.Fun.(*ast.Ident)
			if !ok || id.Name != "append" || expr(a.Lhs[0]) != expr(call.Args[0]) {
				continue
			}
			guard, ok := block.List[i-1].(*ast.IfStmt)
			if !ok || len(guard.Body.List) != 1 {
				continue
			}
			growAssign, ok := guard.Body.List[0].(*ast.AssignStmt)
			if !ok || len(growAssign.Lhs) != 1 || len(growAssign.Rhs) != 1 || expr(growAssign.Lhs[0]) != expr(a.Lhs[0]) {
				continue
			}
			growCall, ok := growAssign.Rhs[0].(*ast.CallExpr)
			if !ok || len(growCall.Args) != 3 || expr(growCall.Args[1]) != expr(a.Lhs[0]) {
				continue
			}
			growID, ok := growCall.Fun.(*ast.Ident)
			if !ok || (growID.Name != "grow" && growID.Name != "fgrow") {
				continue
			}
			dst := expr(a.Lhs[0])
			need := expr(growCall.Args[2])
			if call.Ellipsis.IsValid() {
				if len(call.Args) != 2 || need != "len("+expr(call.Args[1])+")" {
					continue
				}
			} else if need != strconv.Itoa(len(call.Args)-1) {
				continue
			}
			condition := expr(guard.Cond)
			if need == "1" && condition == "len("+dst+") == cap("+dst+")" || condition == need+" > cap("+dst+")-len("+dst+")" || condition == need+" > cap("+dst+") - len("+dst+")" {
				result[call.Pos()] = true
			}
		}
		return true
	})
	return result
}

// Test 2 (core half): on the served path, append, make and new are called only inside
// ledger.go, except in the functions listed in exempt. arenaUse and hashArena are
// statistics and determinism checks, not on the served path.
func TestServedPathAllocatesOnlyThroughTheLedger(t *testing.T) {
	files := servedFiles
	skip := map[string]bool{"arenaUse": true, "hashArena": true}
	fset := token.NewFileSet()
	used := map[string]bool{}
	for _, name := range files {
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || skip[fd.Name.Name] {
				continue
			}
			key := name + ":" + fd.Name.Name
			guarded := guardedAppends(fd, fset)
			ast.Inspect(fd, func(n ast.Node) bool {
				if c, ok := n.(*ast.CallExpr); ok {
					if id, ok := c.Fun.(*ast.Ident); ok && (id.Name == "append" || id.Name == "make" || id.Name == "new") {
						if id.Name == "append" && guarded[c.Pos()] {
							return true
						}
						if _, listed := exempt[key]; listed {
							used[key] = true
						} else {
							t.Errorf("%s: %s calls %s; use the ledger helpers (push, grow, mk, mkCap) or list the function in exempt with a reason", fset.Position(c.Pos()), fd.Name.Name, id.Name)
						}
					}
				}
				if lit, ok := n.(*ast.CompositeLit); ok {
					if _, isMap := lit.Type.(*ast.MapType); isMap {
						if _, listed := exempt[key]; listed {
							used[key] = true
						} else {
							t.Errorf("%s: %s allocates a map literal without ledger accounting", fset.Position(lit.Pos()), key)
						}
					}
				}
				return true
			})
		}
	}
	for k := range exempt {
		if !used[k] {
			t.Errorf("exempt lists %s, which does not exist or no longer allocates", k)
		}
	}
}

func TestGuardedAppendRecognition(t *testing.T) {
	cases := []struct {
		body string
		want bool
	}{
		{`if len(s) == cap(s) { s = grow(l, s, 1) }; s = append(s, x)`, true},
		{`if len(v) > cap(s)-len(s) { s = grow(l, s, len(v)) }; s = append(s, v...)`, true},
		{`s = append(s, x)`, false},
		{`if len(s) == cap(s) { s = grow(l, s, 1) }; s = append(s, x, x)`, false},
		{`if len(s) == cap(s) { other = grow(l, other, 1) }; s = append(s, x)`, false},
		{`if len(s) == cap(s) { s = grow(l, s, 1) }; s = append(s, v...)`, false},
	}
	for _, tc := range cases {
		fs := token.NewFileSet()
		f, err := parser.ParseFile(fs, "case.go", "package ccore\nfunc f(){"+tc.body+"}", 0)
		if err != nil {
			t.Fatal(err)
		}
		fd := f.Decls[0].(*ast.FuncDecl)
		if got := len(guardedAppends(fd, fs)) != 0; got != tc.want {
			t.Errorf("%s: recognized %v, want %v", tc.body, got, tc.want)
		}
	}
}
