package gotreesitter

// normalizeApexClassLiteralAccess retags a raw class_literal(type_identifier,
// ".", "class") node to the field_access(identifier, ".", identifier) shape
// the locked C oracle elects for `Type.class` (issue #601): the grammar's
// class_literal and field_access productions both stay live GLR alternatives
// through the trailing `.`, one needing the `class` keyword and the other
// needing the plain `identifier` token for the same bytes, with no
// grammar-authored dynamic precedence between them.
//
// The classic engine (production, and therefore also compact's fallback
// target and incremental) now elects field_access natively and never reaches
// this function for the shape it matches: a per-stack DFA re-probe
// (relexTokenForStackLexState, parser.go) stops the shared lexer from
// starving the field_access fork, and a certified primary-derivation
// preference at accept-time result selection
// (stackCompareForResultSelectionWithRawShape, parser_result.go) settles the
// resulting tie the way the compact route's own certified election already
// did. This function stays live solely for the GSS-forest experimental fast
// path (ParseForestExperimental), a separate engine with its own dispatch
// loop and accept-time tie-break that neither fix reaches; apex is not in
// that engine's automatic-dispatch set, so no ordinary Parse call needs this
// function anymore. See
// grammars/apex_class_literal_election_native_regression_test.go
// (TestApexClassLiteralForestStillNeedsResultCompatibility), which fails
// loudly once the forest engine no longer needs this rewrite either.
//
// This is also, independent of the above, narrower than the C oracle's own
// rule: it matches only an unqualified leading type name. A qualified one
// (`Outer.Inner.class`) never matched here even before the classic-engine
// fix and keeps its own, separate, pre-existing gap on the forest route.
func normalizeApexClassLiteralAccess(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "apex" {
		return
	}
	fieldAccessSym, fieldAccessNamed, ok := symbolMeta(lang, "field_access")
	if !ok {
		return
	}
	identifierSym, identifierNamed, ok := symbolMeta(lang, "identifier")
	if !ok {
		return
	}
	walkResultTree(root, func(n *Node) {
		if n == nil || n.Type(lang) != "class_literal" || resultChildCount(n) != 3 {
			return
		}
		left := resultChildAt(n, 0)
		dot := resultChildAt(n, 1)
		right := resultChildAt(n, 2)
		if left == nil || dot == nil || right == nil ||
			left.Type(lang) != "type_identifier" ||
			dot.Type(lang) != "." ||
			right.Type(lang) != "class" ||
			!apexNodeTextEquals(source, right, "class") {
			return
		}
		retagResultRoot(n, fieldAccessSym, fieldAccessNamed)
		retagResultRoot(left, identifierSym, identifierNamed)
		retagResultRoot(right, identifierSym, identifierNamed)
	})
}

func apexNodeTextEquals(source []byte, n *Node, want string) bool {
	return n != nil &&
		n.startByte <= n.endByte &&
		int(n.endByte) <= len(source) &&
		string(source[n.startByte:n.endByte]) == want
}
