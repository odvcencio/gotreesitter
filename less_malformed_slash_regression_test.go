package gotreesitter_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func TestIssue1336LessMalformedSlashDoesNotNestRules(t *testing.T) {
	lang := grammars.LessLanguage()
	clean := issue454Less()
	at := strings.Index(string(clean), "padding:") + len("padding:")
	source := append(append(append([]byte{}, clean[:at]...), '/'), clean[at:]...)
	for _, compact := range []bool{false, true} {
		t.Run(fmt.Sprintf("compact=%t", compact), func(t *testing.T) {
			parser := gts.NewParser(lang)
			parser.SetAdmissionCandidateRoute(compact)
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			root := tree.RootNode()
			if root.ChildCount() != 80 || !root.HasError() || root.EndByte() != uint32(len(source)) {
				t.Fatalf("root children=%d error=%t end=%d", root.ChildCount(), root.HasError(), root.EndByte())
			}
			// Pinned from the locked C runtime's fresh parse, including fields,
			// points and flags. The cgo regression also compares directly to C.
			inspection, err := benchfixtures.InspectGoTree(root, lang)
			if err != nil {
				t.Fatal(err)
			}
			const want = "03af075d5f4dd36e0210cff0ad986212dd8f895f6fbcee8772be8219a2e680d9"
			if inspection.SHA256 != want {
				t.Fatalf("fresh tree digest=%s, want %s", inspection.SHA256, want)
			}
		})
	}
}
