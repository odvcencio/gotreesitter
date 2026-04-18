package gotreesitter

// normalizeResultCompatibility applies narrow post-build tree rewrites that
// keep gotreesitter output aligned with C tree-sitter and existing recovery
// expectations for grammars with known normalization gaps.
func normalizeResultCompatibility(root *Node, source []byte, p *Parser) {
	var lang *Language
	if p != nil {
		lang = p.language
	}
	if root == nil || lang == nil {
		return
	}

	switch lang.Name {
	case "bash":
		normalizeBashProgramVariableAssignments(root, lang)
	case "c":
		normalizeCCompatibility(root, source, lang)
	case "c_sharp":
		normalizeCSharpCompatibility(root, source, p, lang)
	case "caddy":
		normalizeTopLevelTrailingLineBreakSpan(root, source, lang)
	case "cobol", "COBOL":
		normalizeCobolCompatibility(root, source, lang)
	case "comment":
		normalizeCommentTrailingExtraTrivia(root, source, lang)
	case "cooklang":
		normalizeCooklangTrailingStepTail(root, source, lang)
	case "d":
		normalizeDCompatibility(root, source, lang)
	case "dart":
		normalizeDartCompatibility(root, source, lang)
	case "elixir":
		normalizeElixirNestedCallTargetFields(root, lang)
	case "erlang":
		normalizeErlangSourceFileForms(root, lang)
	case "fortran":
		normalizeFortranStatementLineBreaks(root, source, lang)
		normalizeTopLevelTrailingLineBreakSpan(root, source, lang)
	case "go":
		normalizeGoReturnedTreeCompatibility(root, source, p, lang)
	case "haskell":
		normalizeHaskellCompatibility(root, source, lang)
	case "hcl":
		normalizeHCLConfigFileRoot(root, lang)
	case "html":
		normalizeHTMLCompatibility(root, source, lang)
	case "ini":
		normalizeIniSectionStarts(root, lang)
	case "javascript":
		normalizeJavaScriptCompatibility(root, source, lang)
	case "lua":
		normalizeLuaChunkLocalDeclarationFields(root, source, lang)
	case "make":
		normalizeMakeConditionalConsequenceFields(root, lang)
	case "nginx":
		normalizeNginxAttributeLineBreaks(root, source, lang)
	case "nim":
		normalizeNimTopLevelCallEnd(root, source, lang)
	case "pascal":
		normalizePascalTopLevelProgramEnd(root, source, lang)
		normalizePascalTrailingExtraTrivia(root, source, lang)
	case "perl":
		normalizePerlCompatibility(root, source, lang)
	case "php":
		normalizePHPCompatibility(root, source, lang)
	case "powershell":
		normalizePowerShellProgramShape(root, source, lang)
	case "pug":
		normalizeTopLevelTrailingLineBreakSpan(root, source, lang)
	case "python":
		normalizePythonCompatibility(root, source, lang)
	case "rst":
		normalizeRSTTopLevelSectionEnd(root, source, lang)
	case "rust":
		normalizeRustCompatibility(root, source, p, lang)
	case "ruby":
		normalizeRubyThenStarts(root, lang)
		normalizeRubyTopLevelModuleBounds(root, source, lang)
	case "scala":
		normalizeScalaCompatibility(root, source, lang)
	case "sql":
		normalizeSQLRecoveredSelectRoot(root, lang)
	case "svelte":
		normalizeSvelteTrailingExtraTrivia(root, source, lang)
	case "tsx", "typescript":
		normalizeTypeScriptTreeCompatibility(root, source, lang)
	case "yaml":
		normalizeYAMLRecoveredRoot(root, source, lang)
	case "zig":
		normalizeZigEmptyInitListFields(root, lang)
	}
}
