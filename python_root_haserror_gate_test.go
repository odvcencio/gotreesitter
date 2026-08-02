package gotreesitter_test

// TestPythonRootHasErrorAgreementGate is the standing root-flag-agreement
// gate identified as missing by
// finding.pre-existing-route-divergences-2026-08-02 section 3: the
// incremental invariant gate (TestIncrementalInvariantGate,
// incremental_invariant_gate_test.go, a separate file in this same package)
// scopes "clean" strictly by a recursive IsError()/IsMissing() walk and
// deliberately never reads Node.HasError() (see that file's doc comment), so
// it has no way to notice when a tree's ROOT HasError() flag disagrees with
// the tree's own content.
//
// This gate closes that hole directly: it reuses the same corpus and the
// same delete/insert/replace edit-site generation as
// TestIncrementalInvariantGate (incrGateCorpus, incrGateBuildEdit), restricted
// to python (the only language the source investigation found the class on --
// a companion sweep of javascript, json, go, rust, css and c found zero), and
// asserts a single invariant per fresh-parsed edited site: if the tree
// contains ANY IsError()/IsMissing() node anywhere, RootNode().HasError() must
// be true. tree-sitter's own contract for HasError() is exactly "the node is
// a syntax error or contains any syntax errors" -- true recursively -- so
// there is no legitimate parse shape where this can disagree; the exception
// list below is expected to stay empty. If it ever needs an entry, that entry
// must carry a C-oracle receipt proving tree-sitter's C implementation
// independently agrees the root should read HasError()=false on that exact
// site, because Parse()'s root HasError() is defined against that oracle, not
// against this engine's own convenience.
import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// pythonRootHasErrorAgreementExceptions is the C-verified exception list for
// TestPythonRootHasErrorAgreementGate. Empty: no site swept by this gate has
// ever been found where the C oracle also reports root HasError()=false while
// the tree contains ERROR/MISSING. Adding an entry here without a C receipt
// in the accompanying PR/commit is a doctrine violation.
var pythonRootHasErrorAgreementExceptions = map[string]bool{}

type pythonRootHasErrorWitness struct {
	path        string
	pos         int
	class       string
	errN, missN int
	firstType   string
}

func (w pythonRootHasErrorWitness) key() string {
	return fmt.Sprintf("%s|%s|pos=%d", w.path, w.class, w.pos)
}

func TestPythonRootHasErrorAgreementGate(t *testing.T) {
	var entries []incrGateCorpusEntry
	for _, e := range incrGateCorpus {
		if e.language == "python" {
			entries = append(entries, e)
		}
	}
	if len(entries) == 0 {
		t.Fatal("no python entries in incrGateCorpus to sweep")
	}

	for _, entry := range entries {
		entry := entry
		t.Run(entry.path, func(t *testing.T) {
			witnesses, totalSites := sweepPythonRootHasErrorWitnesses(t, entry)
			t.Logf("sites=%d witnesses=%d", totalSites, len(witnesses))
			for _, w := range witnesses {
				if pythonRootHasErrorAgreementExceptions[w.key()] {
					continue
				}
				t.Errorf("ROOT-FLAG DISAGREEMENT unlisted: %s pos=%d class=%s: "+
					"tree contains %d ERROR + %d MISSING node(s) (first: %s) but RootNode().HasError()=false. "+
					"Either this is a regression in root HasError derivation, or a genuinely new site needs a "+
					"C-oracle-verified entry in pythonRootHasErrorAgreementExceptions.",
					entry.path, w.pos, w.class, w.errN, w.missN, w.firstType)
			}
		})
	}
}

// sweepPythonRootHasErrorWitnesses parses entry's fixture, then walks every
// delete/insert/replace edit site at entry's stride (matching
// TestIncrementalInvariantGate's own methodology exactly, per incrGateBuildEdit),
// fresh-parses the edited buffer, and records every site where the resulting
// tree contains an ERROR/MISSING node while its root reports HasError()=false.
func sweepPythonRootHasErrorWitnesses(t *testing.T, entry incrGateCorpusEntry) ([]pythonRootHasErrorWitness, int) {
	t.Helper()

	e := grammars.DetectLanguageByName(entry.language)
	if e == nil {
		t.Fatalf("unknown language %q", entry.language)
	}
	lang := e.Language()
	support := grammars.EvaluateParseSupport(*e, lang)
	if support.Backend == grammars.ParseBackendUnsupported || support.Backend == grammars.ParseBackendDFAPartial {
		t.Fatalf("language %q has no usable full-parse backend for the gate: %s", entry.language, support.Reason)
	}

	src, err := os.ReadFile(filepath.Join("testdata", "incremental_gate", entry.path))
	if err != nil {
		t.Fatalf("read corpus file: %v", err)
	}
	if len(src) < 8 {
		t.Fatalf("corpus file too small to sweep: %s", entry.path)
	}

	p := gts.NewParser(lang)
	parseFresh := func(s []byte) *gts.Tree {
		if support.Backend == grammars.ParseBackendTokenSource {
			tr, _ := p.ParseWithTokenSource(s, e.TokenSourceFactory(s, lang))
			return tr
		}
		tr, _ := p.Parse(s)
		return tr
	}

	stride := entry.stride
	if stride < 1 {
		stride = 1
	}

	var witnesses []pythonRootHasErrorWitness
	totalSites := 0
	editClasses := []string{"delete", "insert", "replace"}

	for i := 1; i < len(src)-1; i += stride {
		for _, cls := range editClasses {
			edited, _ := incrGateBuildEdit(src, i, cls)
			tree := parseFresh(edited)
			if tree == nil || tree.RootNode() == nil {
				continue
			}
			totalSites++
			root := tree.RootNode()
			errN, missN := incrGateTreeStats(root)
			if errN == 0 && missN == 0 {
				continue // nothing to disagree about at this site
			}
			if root.HasError() {
				continue // agrees with its own content -- not a witness
			}
			firstType := firstErrorOrMissingNodeType(lang, root)
			witnesses = append(witnesses, pythonRootHasErrorWitness{
				path: entry.path, pos: i, class: cls,
				errN: errN, missN: missN, firstType: firstType,
			})
		}
	}
	return witnesses, totalSites
}

// firstErrorOrMissingNodeType returns the type of the first IsError() or
// IsMissing() node found in a pre-order walk of n, or "<none>" if none exists.
func firstErrorOrMissingNodeType(lang *gts.Language, n *gts.Node) string {
	if n == nil {
		return "<none>"
	}
	if n.IsError() || n.IsMissing() {
		return n.Type(lang)
	}
	for i := 0; i < n.ChildCount(); i++ {
		if t := firstErrorOrMissingNodeType(lang, n.Child(i)); t != "<none>" {
			return t
		}
	}
	return "<none>"
}
