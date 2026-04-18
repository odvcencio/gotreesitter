package parserresult_test

import "testing"

func TestPugTopLevelTagCarriesTrailingNewlineSpan(t *testing.T) {
	const src = "p hello\n"
	tree, lang := parseByLanguageName(t, "pug", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected pug parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("pug root childCount=%d, want 1", root.ChildCount())
	}
	tag := root.Child(0)
	if tag == nil || tag.Type(lang) != "tag" {
		t.Fatalf("pug child=%v, want tag", tag)
	}
	if got, want := tag.EndByte(), root.EndByte(); got != want {
		t.Fatalf("pug tag.EndByte=%d, want root.EndByte=%d", got, want)
	}
}

func TestCaddyTopLevelServerCarriesTrailingNewlineSpan(t *testing.T) {
	const src = ":8080 {\n}\n"
	tree, lang := parseByLanguageName(t, "caddy", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected caddy parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("caddy root childCount=%d, want 1", root.ChildCount())
	}
	server := root.Child(0)
	if server == nil || server.Type(lang) != "server" {
		t.Fatalf("caddy child=%v, want server", server)
	}
	if got, want := server.EndByte(), root.EndByte(); got != want {
		t.Fatalf("caddy server.EndByte=%d, want root.EndByte=%d", got, want)
	}
}

func TestCooklangStepCarriesTerminalPunctuationAndRootNewline(t *testing.T) {
	const src = "Add @salt{1%tsp}.\n"
	tree, lang := parseByLanguageName(t, "cooklang", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected cooklang parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("cooklang root childCount=%d, want 1", root.ChildCount())
	}
	step := root.Child(0)
	if step == nil || step.Type(lang) != "step" {
		t.Fatalf("cooklang child=%v, want step", step)
	}
	if got, want := step.EndByte(), uint32(len(src)-1); got != want {
		t.Fatalf("cooklang step.EndByte=%d, want %d", got, want)
	}
	if got, want := root.EndByte(), uint32(len(src)); got != want {
		t.Fatalf("cooklang root.EndByte=%d, want %d", got, want)
	}
}

func TestFortranProgramCarriesLineBreaks(t *testing.T) {
	const src = "program hello\n  implicit none\nend program hello\n"
	tree, lang := parseByLanguageName(t, "fortran", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected fortran parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("fortran root childCount=%d, want 1", root.ChildCount())
	}
	program := root.Child(0)
	if program == nil || program.Type(lang) != "program" {
		t.Fatalf("fortran child=%v, want program", program)
	}
	if got, want := program.EndByte(), root.EndByte(); got != want {
		t.Fatalf("fortran program.EndByte=%d, want root.EndByte=%d", got, want)
	}
	stmt := program.Child(0)
	if stmt == nil || stmt.Type(lang) != "program_statement" {
		t.Fatalf("fortran first child=%v, want program_statement", stmt)
	}
	if got, want := stmt.EndByte(), uint32(14); got != want {
		t.Fatalf("fortran program_statement.EndByte=%d, want %d", got, want)
	}
}

func TestCobolLeadingAreaStartMatchesC(t *testing.T) {
	const src = "       IDENTIFICATION DIVISION.\n       PROGRAM-ID. HELLO.\n"
	tree, lang := parseByLanguageName(t, "cobol", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected cobol parse error: %s", root.SExpr(lang))
	}
	if got, want := root.StartByte(), uint32(7); got != want {
		t.Fatalf("cobol root.StartByte=%d, want %d", got, want)
	}
	if root.ChildCount() != 1 {
		t.Fatalf("cobol root childCount=%d, want 1", root.ChildCount())
	}
	def := root.Child(0)
	if def == nil || def.Type(lang) != "program_definition" {
		t.Fatalf("cobol child=%v, want program_definition", def)
	}
	if got, want := def.StartByte(), uint32(7); got != want {
		t.Fatalf("cobol program_definition.StartByte=%d, want %d", got, want)
	}
	div := def.Child(0)
	if div == nil || div.Type(lang) != "identification_division" {
		t.Fatalf("cobol grandchild=%v, want identification_division", div)
	}
	if got, want := div.StartByte(), uint32(7); got != want {
		t.Fatalf("cobol identification_division.StartByte=%d, want %d", got, want)
	}
}

func TestCobolDivisionAndPerformStartsMatchC(t *testing.T) {
	const src = "" +
		"       identification division.\n" +
		"       program-id. a.\n" +
		"       procedure division.\n" +
		"       perform aa.\n" +
		"       aa.\n"
	tree, lang := parseByLanguageName(t, "cobol", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected cobol parse error: %s", root.SExpr(lang))
	}
	def := root.Child(0)
	if def == nil || def.Type(lang) != "program_definition" {
		t.Fatalf("cobol child=%v, want program_definition", def)
	}
	if def.ChildCount() != 2 {
		t.Fatalf("cobol program_definition.ChildCount=%d, want 2", def.ChildCount())
	}
	idDiv := def.Child(0)
	if idDiv == nil || idDiv.Type(lang) != "identification_division" {
		t.Fatalf("cobol first child=%v, want identification_division", idDiv)
	}
	if got, want := idDiv.StartByte(), uint32(7); got != want {
		t.Fatalf("cobol identification_division.StartByte=%d, want %d", got, want)
	}
	if got, want := idDiv.EndByte(), uint32(53); got != want {
		t.Fatalf("cobol identification_division.EndByte=%d, want %d", got, want)
	}
	procDiv := def.Child(1)
	if procDiv == nil || procDiv.Type(lang) != "procedure_division" {
		t.Fatalf("cobol second child=%v, want procedure_division", procDiv)
	}
	if got, want := procDiv.StartByte(), uint32(61); got != want {
		t.Fatalf("cobol procedure_division.StartByte=%d, want %d", got, want)
	}
	if procDiv.ChildCount() < 2 {
		t.Fatalf("cobol procedure_division.ChildCount=%d, want >= 2", procDiv.ChildCount())
	}
	performStmt := procDiv.Child(1)
	if performStmt == nil || performStmt.Type(lang) != "perform_statement_call_proc" {
		t.Fatalf("cobol procedure_division.Child(1)=%v, want perform_statement_call_proc", performStmt)
	}
	if got, want := performStmt.StartByte(), uint32(88); got != want {
		t.Fatalf("cobol perform_statement_call_proc.StartByte=%d, want %d", got, want)
	}
}

func TestCobolPerformOptionEndMatchesC(t *testing.T) {
	const src = "" +
		"       identification division.\n" +
		"       program-id. a.\n" +
		"       procedure division.\n" +
		"       perform aa forever.\n" +
		"       aa.\n"
	tree, lang := parseByLanguageName(t, "cobol", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected cobol parse error: %s", root.SExpr(lang))
	}
	procDiv := root.Child(0).Child(1)
	if procDiv == nil || procDiv.Type(lang) != "procedure_division" {
		t.Fatalf("cobol procedure_division=%v, want procedure_division", procDiv)
	}
	performStmt := procDiv.Child(1)
	if performStmt == nil || performStmt.Type(lang) != "perform_statement_call_proc" {
		t.Fatalf("cobol procedure_division.Child(1)=%v, want perform_statement_call_proc", performStmt)
	}
	if got, want := performStmt.EndByte(), uint32(106); got != want {
		t.Fatalf("cobol perform_statement_call_proc.EndByte=%d, want %d", got, want)
	}
	if performStmt.ChildCount() != 2 {
		t.Fatalf("cobol perform_statement_call_proc.ChildCount=%d, want 2", performStmt.ChildCount())
	}
	option := performStmt.Child(1)
	if option == nil || option.Type(lang) != "perform_option" {
		t.Fatalf("cobol perform_statement_call_proc.Child(1)=%v, want perform_option", option)
	}
	if got, want := option.EndByte(), uint32(106); got != want {
		t.Fatalf("cobol perform_option.EndByte=%d, want %d", got, want)
	}
}

func TestNginxAttributesCarryTrailingLineBreaks(t *testing.T) {
	const src = "http {\n  server {\n    listen 80;\n    server_name example.com;\n  }\n}\n"
	tree, lang := parseByLanguageName(t, "nginx", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected nginx parse error: %s", root.SExpr(lang))
	}
	if root.ChildCount() != 1 {
		t.Fatalf("nginx root childCount=%d, want 1", root.ChildCount())
	}
	httpAttr := root.Child(0)
	if httpAttr == nil || httpAttr.Type(lang) != "attribute" {
		t.Fatalf("nginx child=%v, want attribute", httpAttr)
	}
	if got, want := httpAttr.EndByte(), root.EndByte(); got != want {
		t.Fatalf("nginx outer attribute.EndByte=%d, want root.EndByte=%d", got, want)
	}
	serverAttr := httpAttr.Child(1).Child(1)
	if serverAttr == nil || serverAttr.Type(lang) != "attribute" {
		t.Fatalf("nginx nested attribute=%v, want attribute", serverAttr)
	}
	if got, want := serverAttr.EndByte(), uint32(66); got != want {
		t.Fatalf("nginx server attribute.EndByte=%d, want %d", got, want)
	}
	listenAttr := serverAttr.Child(1).Child(1)
	if listenAttr == nil || listenAttr.Type(lang) != "attribute" {
		t.Fatalf("nginx listen attribute=%v, want attribute", listenAttr)
	}
	if got, want := listenAttr.EndByte(), uint32(33); got != want {
		t.Fatalf("nginx listen attribute.EndByte=%d, want %d", got, want)
	}
	nameAttr := serverAttr.Child(1).Child(2)
	if nameAttr == nil || nameAttr.Type(lang) != "attribute" {
		t.Fatalf("nginx server_name attribute=%v, want attribute", nameAttr)
	}
	if got, want := nameAttr.EndByte(), uint32(62); got != want {
		t.Fatalf("nginx server_name attribute.EndByte=%d, want %d", got, want)
	}
}
