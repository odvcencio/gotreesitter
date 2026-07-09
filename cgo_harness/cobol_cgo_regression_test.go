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
				t.Fatalf("go-vs-C divergences:\n%s\n\ngo:\n%s\n\ngo spans:\n%s\n\nc:\n%s", joinTopErrors(errs), goRoot.SExpr(goLang), dumpGoTree(goRoot, goLang, 0), dumpCTree(cRoot, 0))
			}
		})
	}
}

func TestCobolCGOErrorOracleParity(t *testing.T) {
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
			name: "exec_cics_tail_after_clean_prefix",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"      * Procedure comment\n" +
				"           MOVE SPACES TO ABEND-REASON\n" +
				"           COPY CABENDPO.\n" +
				"           IF BANK-MAP-FUNCTION-GET\n" +
				"              EXEC CICS LINK PROGRAM(X)\n",
		},
		{
			name: "move_led_exec_tail_retains_error",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"      * Procedure comment\n" +
				"           MOVE A TO B\n" +
				"           IF FLAG\n" +
				"              EXEC CICS RETURN\n",
		},
		{
			name: "z_literal_data_root_recovery_retains_error",
			src: "       data division.\n" +
				"       working-storage section.\n" +
				"       01  OK PIC X.\n" +
				"       01  BAD PIC X(120)\n" +
				"           VALUE Z'HELLO'.\n" +
				"      * banner\n" +
				"       procedure division.\n",
		},
		{
			name: "exec_cics_error_preserves_has_error_after_paragraph_recovery",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"           MOVE A TO B\n" +
				"           IF FLAG\n" +
				"              EXEC CICS RETURN\n" +
				"       aa.\n",
		},
		{
			name: "exec_cics_error_trims_before_trailing_recovered_comment",
			src: "       identification division.\n" +
				"       program-id. a.\n" +
				"       procedure division.\n" +
				"           MOVE A TO B\n" +
				"           IF FLAG\n" +
				"              EXEC CICS RETURN\n" +
				"       aa.\n" +
				"      * trailing banner\n",
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

			cTree := cParser.Parse(src, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C nil tree")
			}
			defer cTree.Close()
			cRoot := cTree.RootNode()
			if !cRoot.HasError() {
				t.Fatalf("C root has no error; adversarial fixture no longer exercises error parity:\n%s", dumpCTree(cRoot, 0))
			}
			if !goRoot.HasError() {
				t.Fatalf("go root swallowed C error signal:\n\ngo:\n%s\n\nc:\n%s", goRoot.SExpr(goLang), dumpCTree(cRoot, 0))
			}

			var errs []string
			compareNodes(goRoot, goLang, cRoot, "root", &errs)
			if len(errs) > 0 {
				t.Fatalf("go-vs-C divergences:\n%s\n\ngo:\n%s\n\ngo spans:\n%s\n\nc:\n%s", joinTopErrors(errs), goRoot.SExpr(goLang), dumpGoTree(goRoot, goLang, 0), dumpCTree(cRoot, 0))
			}
		})
	}
}
