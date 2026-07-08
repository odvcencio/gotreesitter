//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestCobolCGOTrailingTriviaSpans(t *testing.T) {
	goLang := grammars.CobolLanguage()
	cLang, err := ParityCLanguage("cobol")
	if err != nil {
		t.Skipf("C parser unavailable: %v", err)
	}
	cParser := sitter.NewParser()
	defer cParser.Close()
	if err := cParser.SetLanguage(cLang); err != nil {
		t.Fatalf("C SetLanguage: %v", err)
	}
	goParser := gotreesitter.NewParser(goLang)

	cases := []struct {
		name string
		src  string
	}{
		{
			name: "perform_forever_continue",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"       perform forever\n" +
				"         continue\n" +
				"       end-perform.\n",
		},
		{
			name: "evaluate_goto",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"       evaluate 1\n" +
				"       when 1\n" +
				"         go to aa\n" +
				"       end-evaluate.\n" +
				"       aa.\n",
		},
		{
			name: "evaluate_two_whens_goto",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"       evaluate 1\n" +
				"       when 1\n" +
				"       when 2\n" +
				"         go to aa\n" +
				"       end-evaluate.\n" +
				"       aa.\n",
		},
		{
			name: "fixed_format_data_description_tails",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"003300 ENVIRONMENT DIVISION.                                            NC1014.2\n" +
				"003900 INPUT-OUTPUT SECTION.                                            NC1014.2\n" +
				"004000 FILE-CONTROL.                                                    NC1014.2\n" +
				"004100     SELECT PRINT-FILE ASSIGN TO                                  NC1014.2\n" +
				"004200     \"report.log\".                                                NC1014.2\n" +
				"004300 DATA DIVISION.                                                   NC1014.2\n" +
				"004400 FILE SECTION.                                                    NC1014.2\n" +
				"004500 FD  PRINT-FILE.                                                  NC1014.2\n" +
				"004600 01  PRINT-REC PICTURE X(120).                                    NC1014.2\n" +
				"004700 01  DUMMY-RECORD PICTURE X(120).                                 NC1014.2\n" +
				"004800 WORKING-STORAGE SECTION.                                         NC1014.2\n" +
				"004900 77  WRK-DS-18V00                PICTURE S9(18).                  NC1014.2\n" +
				"       PROCEDURE DIVISION.\n" +
				"       DISPLAY 1.",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := []byte(tc.src)
			goTree, err := goParser.Parse(src)
			if err != nil {
				t.Fatalf("go parse: %v", err)
			}
			goRoot := goTree.RootNode()
			if goRoot == nil {
				t.Fatal("go nil root")
			}
			if goRoot.HasError() {
				t.Fatalf("go root has error:\n%s", goRoot.SExpr(goLang))
			}

			cTree := cParser.Parse(src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C nil tree")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if cRoot.HasError() {
				t.Fatalf("C root has error:\n%s", dumpCTree(cRoot, 0))
			}

			var errs []string
			compareNodes(goRoot, goLang, cRoot, "root", &errs)
			if len(errs) > 0 {
				t.Fatalf("go-vs-C divergences:\n%s\n\ngo:\n%s\n\nc:\n%s", joinTopErrors(errs), goRoot.SExpr(goLang), dumpCTree(cRoot, 0))
			}
		})
	}
}
