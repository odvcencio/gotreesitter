package gotreesitter

type resultCompatibilityContext struct {
	root      *Node
	source    []byte
	parser    *Parser
	lang      *Language
	stopCheck parseStopCheck
}

// normalizeResultCompatibility applies narrow post-build tree rewrites that
// keep gotreesitter output aligned with C tree-sitter and existing recovery
// expectations for grammars with known normalization gaps.
func normalizeResultCompatibility(root *Node, source []byte, p *Parser) ParseStopReason {
	var lang *Language
	if p != nil {
		lang = p.language
	}
	if root == nil || lang == nil {
		return ParseStopNone
	}
	ctx := resultCompatibilityContext{
		root:      root,
		source:    source,
		parser:    p,
		lang:      lang,
		stopCheck: p.activeParseStopCheck(),
	}
	if reason := ctx.stopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := runLanguageResultCompatibility(ctx); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := ctx.stopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	normalizeResultCollapsedNamedLeafChildren(root, lang)
	return ctx.stopReason()
}

func (ctx resultCompatibilityContext) stopReason() ParseStopReason {
	if ctx.stopCheck == nil {
		return ParseStopNone
	}
	reason := ctx.stopCheck()
	if reason == "" {
		return ParseStopNone
	}
	return reason
}

func runLanguageResultCompatibility(ctx resultCompatibilityContext) ParseStopReason {
	if isCobolLanguage(ctx.lang) {
		normalizeCobolCompatibility(ctx.root, ctx.source, ctx.lang)
		return ctx.stopReason()
	}

	switch ctx.lang.Name {
	case "bash":
		normalizeBashProgramVariableAssignments(ctx.root, ctx.lang)
		normalizeBashGeneratedCommandAssignments(ctx.root, ctx.source, ctx.lang)
		normalizeBashCommandNameArguments(ctx.root, ctx.lang)
	case "c", "cpp":
		normalizeCCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "c_sharp":
		normalizeCSharpCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "caddy":
		normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
	case "comment":
		normalizeCommentTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
	case "cooklang":
		normalizeCooklangTrailingStepTail(ctx.root, ctx.source, ctx.lang)
	case "d":
		normalizeDCompatibility(ctx.root, ctx.source, ctx.lang)
	case "dart":
		normalizeDartCompatibility(ctx.root, ctx.source, ctx.lang)
	case "elixir":
		normalizeElixirCompatibility(ctx.root, ctx.source, ctx.lang)
	case "erlang":
		normalizeErlangSourceFileForms(ctx.root, ctx.lang)
	case "fortran":
		normalizeFortranStatementLineBreaks(ctx.root, ctx.source, ctx.lang)
		normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
	case "go":
		return normalizeGoReturnedTreeCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "graphql":
		normalizeGraphQLCompatibility(ctx.root, ctx.source, ctx.lang)
	case "haskell":
		normalizeHaskellCompatibility(ctx.root, ctx.source, ctx.lang)
	case "hcl":
		normalizeHCLConfigFileRoot(ctx.root, ctx.source, ctx.lang)
	case "html":
		normalizeHTMLCompatibility(ctx.root, ctx.source, ctx.lang)
	case "ini":
		normalizeIniSectionStarts(ctx.root, ctx.lang)
	case "java":
		normalizeJavaCompatibility(ctx.root, ctx.source, ctx.lang)
	case "javascript":
		normalizeJavaScriptCompatibility(ctx.root, ctx.source, ctx.lang)
	case "kotlin":
		normalizeKotlinCompatibility(ctx.root, ctx.source, ctx.lang)
	case "lua":
		normalizeLuaChunkLocalDeclarationFields(ctx.root, ctx.source, ctx.lang)
	case "make":
		normalizeMakeConditionalConsequenceFields(ctx.root, ctx.lang)
	case "nginx":
		normalizeNginxAttributeLineBreaks(ctx.root, ctx.source, ctx.lang)
	case "nim":
		normalizeNimTopLevelCallEnd(ctx.root, ctx.source, ctx.lang)
	case "ocaml":
		normalizeOCamlCompatibility(ctx.root, ctx.source, ctx.lang)
	case "pascal":
		normalizePascalTopLevelProgramEnd(ctx.root, ctx.source, ctx.lang)
		normalizePascalTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
	case "perl":
		normalizePerlCompatibility(ctx.root, ctx.source, ctx.lang)
	case "php":
		normalizePHPCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "powershell":
		normalizePowerShellProgramShape(ctx.root, ctx.source, ctx.lang)
		normalizePowerShellErrorProgramRoot(ctx.root, ctx.lang)
		normalizePowerShellAssignmentOperatorTokens(ctx.root, ctx.source, ctx.lang)
	case "pug":
		normalizeTopLevelTrailingLineBreakSpan(ctx.root, ctx.source, ctx.lang)
	case "python":
		normalizePythonCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "r":
		normalizeRCompatibility(ctx.root, ctx.source, ctx.lang)
	case "rst":
		normalizeRSTTopLevelSectionEnd(ctx.root, ctx.source, ctx.lang)
	case "rust":
		normalizeRustCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "ruby":
		normalizeRubyThenStarts(ctx.root, ctx.lang)
		normalizeRubyTopLevelModuleBounds(ctx.root, ctx.source, ctx.lang)
	case "scala":
		normalizeScalaCompatibility(ctx.root, ctx.source, ctx.lang)
	case "sql":
		normalizeSQLRecoveredSelectRoot(ctx.root, ctx.lang)
		normalizeSQLTrailingSelectListError(ctx.root, ctx.lang)
		if ctx.parser != nil && !ctx.parser.skipRecoveryReparse {
			normalizeSQLRecoveredTopLevelSelectStatements(ctx.root, ctx.source, ctx.parser, ctx.lang)
		}
		normalizeSQLSelectClauseBodyIntoFields(ctx.root, ctx.lang)
	case "svelte":
		normalizeSvelteTrailingExtraTrivia(ctx.root, ctx.source, ctx.lang)
	case "swift":
		normalizeSwiftCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "tsx", "typescript":
		normalizeTypeScriptTreeCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang)
	case "yaml":
		normalizeYAMLRecoveredRoot(ctx.root, ctx.source, ctx.lang)
	case "zig":
		normalizeZigEmptyInitListFields(ctx.root, ctx.lang)
	}
	return ctx.stopReason()
}
