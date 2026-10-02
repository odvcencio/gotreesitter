//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestPythonGraduationTupleLists(t *testing.T) {
	lang := grammars.PythonLanguage()
	cl, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{
		"def _index(result):\n    for awaited_info in result:\n        id2name[task_id] = task_name\n    path, cycles = [], []\n    def dfs(v):\n        for w in graph.get(v, ()):\n            cycles.append(path[i:] + [w])\n",
		"path, cycles = [], []\n", "original_names, names = names, []\n",
		"def f():\n    path, cycles = [], []\n",
		"def f(graph):\n    WHITE, GREY, BLACK = 0, 1, 2\n    color = defaultdict(lambda: WHITE)\n    path, cycles = [], []\n",
		"class C:\n    def f(self, names):\n        if names:\n            original_names, names = names, []\n",
		"[] = values\n", "a, [] = values\n", "for [] in values:\n    pass\n", "del []\n", "match values:\n    case []:\n        pass\n",
		"a, b = [1], [2]\n", "a = [], []\n", "a = [[], []], []\n",
		"a, b = (), []\n", "[a, b] = values\n", "a, [b, c] = values\n",
		"for a, [b, c] in values:\n    pass\n", "del a, [b, c]\n",
		"match values:\n    case [a, b]:\n        pass\n",
	} {
		t.Run(source, func(t *testing.T) {
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse([]byte(source), nil)
			if ct == nil {
				t.Fatal("C nil")
			}
			defer ct.Close()
			for _, compact := range []bool{false, true} {
				p := gts.NewParser(lang)
				p.SetAdmissionCandidateRoute(compact)
				gts.ResetAdmissionCandidateCounters()
				tree, err := p.Parse([]byte(source))
				if err != nil {
					t.Fatal(err)
				}
				defer tree.Release()
				if compact && strings.HasPrefix(source, "def _index") {
					served, declined := gts.AdmissionCandidateCounters()
					if served != 1 || declined != 0 {
						t.Fatalf("load-bearing conflict witness fell back: served=%d declined=%d", served, declined)
					}
				}
				assertLockedCTreeExact(t, "tuple lists", tree, lang, ct)
			}
		})
	}
}

// TestPythonGraduationCommentListElection covers material alternatives that
// converge behind an extra token. The compact route must keep C's selected
// list node rather than the first branch's list_pattern.
func TestPythonGraduationCommentListElection(t *testing.T) {
	lang := grammars.PythonLanguage()
	cl, err := COracleLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []string{"s=s,[]#", "s=s,[]#x\n", "s=[],s#", "s=s,[]", "s=s,[]\n", "s=s,[1]#"} {
		t.Run(source, func(t *testing.T) {
			cp := sitter.NewParser()
			defer cp.Close()
			if err := cp.SetLanguage(cl); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse([]byte(source), nil)
			if ct == nil {
				t.Fatal("C nil")
			}
			defer ct.Close()
			p := gts.NewParser(lang)
			p.SetAdmissionCandidateRoute(true)
			gts.ResetAdmissionCandidateCounters()
			tree, err := p.Parse([]byte(source))
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			served, declined := gts.AdmissionCandidateCounters()
			if served != 1 || declined != 0 {
				t.Fatalf("comment election fell back: served=%d declined=%d", served, declined)
			}
			assertLockedCTreeExact(t, "comment list election", tree, lang, ct)
		})
	}
}
