package main

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
)

// BuildLanguage materializes a runtime language directly from extracted parser data.
func BuildLanguage(g *ExtractedGrammar) *gotreesitter.Language {
	lang := &gotreesitter.Language{
		Name:                                     g.Name,
		SymbolCount:                              uint32(g.SymbolCount),
		TokenCount:                               uint32(g.TokenCount),
		ExternalTokenCount:                       uint32(g.ExternalTokenCount),
		StateCount:                               uint32(g.StateCount),
		LargeStateCount:                          uint32(g.LargeStateCount),
		FieldCount:                               uint32(g.FieldCount),
		ProductionIDCount:                        uint32(g.ProductionIDCount),
		InitialState:                             1,
		CRecoveryCostCompetitionCapable:          g.CRecoveryCostCompetitionCapable,
		CRecoveryCostCompetitionEnabledByDefault: g.CRecoveryCostCompetitionEnabledByDefault,
	}

	if len(g.SymbolNames) > 0 {
		lang.SymbolNames = append([]string(nil), g.SymbolNames...)
	}

	if len(g.SymbolMetadata) > 0 {
		lang.SymbolMetadata = make([]gotreesitter.SymbolMetadata, len(g.SymbolMetadata))
		for i, m := range g.SymbolMetadata {
			name := ""
			if i < len(g.SymbolNames) {
				name = g.SymbolNames[i]
			}
			lang.SymbolMetadata[i] = gotreesitter.SymbolMetadata{
				Name:      name,
				Visible:   m.Visible,
				Named:     m.Named,
				Supertype: m.Supertype,
			}
		}
	}

	if len(g.FieldNames) > 0 {
		lang.FieldNames = append([]string(nil), g.FieldNames...)
	}

	if len(g.FieldMapSlices) > 0 {
		lang.FieldMapSlices = append([][2]uint16(nil), g.FieldMapSlices...)
	}

	if len(g.FieldMapEntries) > 0 {
		lang.FieldMapEntries = make([]gotreesitter.FieldMapEntry, len(g.FieldMapEntries))
		for i, e := range g.FieldMapEntries {
			lang.FieldMapEntries[i] = gotreesitter.FieldMapEntry{
				FieldID:    gotreesitter.FieldID(e.FieldID),
				ChildIndex: uint8(e.ChildIndex),
				Inherited:  e.Inherited,
			}
		}
	}

	if len(g.AliasSequences) > 0 {
		lang.AliasSequences = make([][]gotreesitter.Symbol, len(g.AliasSequences))
		for i, seq := range g.AliasSequences {
			row := make([]gotreesitter.Symbol, len(seq))
			for j, sym := range seq {
				row[j] = gotreesitter.Symbol(sym)
			}
			lang.AliasSequences[i] = row
		}
	}
	if len(g.NonTerminalAliasMap) > 0 {
		lang.NonTerminalAliasMap = make([][]gotreesitter.Symbol, len(g.NonTerminalAliasMap))
		for i, seq := range g.NonTerminalAliasMap {
			if len(seq) == 0 {
				continue
			}
			row := make([]gotreesitter.Symbol, len(seq))
			for j, sym := range seq {
				row[j] = gotreesitter.Symbol(sym)
			}
			lang.NonTerminalAliasMap[i] = row
		}
	}

	if len(g.ParseTable) > 0 {
		lang.ParseTable = make([][]uint16, len(g.ParseTable))
		for i, row := range g.ParseTable {
			lang.ParseTable[i] = append([]uint16(nil), row...)
		}
	}

	if len(g.SmallParseTable) > 0 {
		lang.SmallParseTable = append([]uint16(nil), g.SmallParseTable...)
	}
	if len(g.SmallParseTableMap) > 0 {
		lang.SmallParseTableMap = append([]uint32(nil), g.SmallParseTableMap...)
	}

	lang.ParseActions = buildParseActions(g.ParseActions)

	if len(g.LexModes) > 0 {
		lang.LexModes = make([]gotreesitter.LexMode, len(g.LexModes))
		for i, lm := range g.LexModes {
			lang.LexModes[i] = gotreesitter.LexMode{
				LexState:          uint16(lm.LexState),
				ExternalLexState:  uint16(lm.ExternalLexState),
				ReservedWordSetID: uint16(lm.ReservedWordSetID),
			}
		}
	}

	if len(g.LexStates) > 0 {
		lang.LexStates = convertLexStates(g.LexStates)
	}
	if len(g.KeywordLexStates) > 0 {
		lang.KeywordLexStates = convertLexStates(g.KeywordLexStates)
	}
	if g.KeywordCaptureToken > 0 {
		lang.KeywordCaptureToken = gotreesitter.Symbol(g.KeywordCaptureToken)
	}

	if len(g.ExternalSymbols) > 0 {
		lang.ExternalSymbols = make([]gotreesitter.Symbol, len(g.ExternalSymbols))
		for i, sym := range g.ExternalSymbols {
			lang.ExternalSymbols[i] = gotreesitter.Symbol(sym)
		}
	}
	if len(g.ExternalLexStates) > 0 {
		lang.ExternalLexStates = make([][]bool, len(g.ExternalLexStates))
		for i, row := range g.ExternalLexStates {
			lang.ExternalLexStates[i] = append([]bool(nil), row...)
		}
	}

	// ABI 15: reserved words
	if g.MaxReservedWordSetSize > 0 {
		lang.MaxReservedWordSetSize = uint16(g.MaxReservedWordSetSize)
		if len(g.ReservedWords) > 0 {
			lang.ReservedWords = make([]gotreesitter.Symbol, len(g.ReservedWords))
			for i, rw := range g.ReservedWords {
				lang.ReservedWords[i] = gotreesitter.Symbol(rw)
			}
		}
	}

	// ABI 15: supertype hierarchy
	if g.SupertypeCount > 0 {
		if len(g.SupertypeSymbols) > 0 {
			lang.SupertypeSymbols = make([]gotreesitter.Symbol, len(g.SupertypeSymbols))
			for i, sym := range g.SupertypeSymbols {
				lang.SupertypeSymbols[i] = gotreesitter.Symbol(sym)
			}
		}
		if len(g.SupertypeMapSlices) > 0 {
			lang.SupertypeMapSlices = append([][2]uint16(nil), g.SupertypeMapSlices...)
		}
		if len(g.SupertypeMapEntries) > 0 {
			lang.SupertypeMapEntries = make([]gotreesitter.Symbol, len(g.SupertypeMapEntries))
			for i, sym := range g.SupertypeMapEntries {
				lang.SupertypeMapEntries[i] = gotreesitter.Symbol(sym)
			}
		}
	}

	// ABI 15: language metadata (grammar semantic version)
	if g.LanguageMetadataMajor > 0 || g.LanguageMetadataMinor > 0 || g.LanguageMetadataPatch > 0 {
		lang.Metadata = gotreesitter.LanguageMetadata{
			MajorVersion: uint8(g.LanguageMetadataMajor),
			MinorVersion: uint8(g.LanguageMetadataMinor),
			PatchVersion: uint8(g.LanguageMetadataPatch),
		}
	}

	gotreesitter.InferGeneratedRepeatAuxMetadata(lang)
	lang.ConflictPolicies = buildConflictPolicies(g, lang)

	return lang
}

func buildConflictPolicies(g *ExtractedGrammar, lang *gotreesitter.Language) []gotreesitter.ConflictPolicy {
	if g == nil || lang == nil || len(g.ConflictPolicies) == 0 {
		return nil
	}
	policies := make([]gotreesitter.ConflictPolicy, 0, len(g.ConflictPolicies))
	for _, policy := range g.ConflictPolicies {
		if policy.Kind != "repetition_shift" || policy.State < 0 || policy.Lookahead < 0 {
			continue
		}
		reduceSymbols := make([]gotreesitter.Symbol, 0, len(policy.ReduceSymbols))
		reject := false
		for _, sym := range policy.ReduceSymbols {
			if sym < 0 {
				reject = true
				break
			}
			runtimeSym := gotreesitter.Symbol(sym)
			if languageSymbolIsGeneratedRepeatAux(lang, runtimeSym) {
				reject = true
				break
			}
			reduceSymbols = append(reduceSymbols, runtimeSym)
		}
		if reject {
			continue
		}
		policies = append(policies, gotreesitter.ConflictPolicy{
			State:         gotreesitter.StateID(policy.State),
			Lookahead:     gotreesitter.Symbol(policy.Lookahead),
			Kind:          gotreesitter.ConflictPolicyRepetitionShift,
			ReduceSymbols: reduceSymbols,
		})
	}
	return policies
}

func languageSymbolIsGeneratedRepeatAux(lang *gotreesitter.Language, sym gotreesitter.Symbol) bool {
	return lang != nil && int(sym) >= 0 && int(sym) < len(lang.SymbolMetadata) && lang.SymbolMetadata[sym].GeneratedRepeatAux
}

func buildParseActions(groups []ActionGroup) []gotreesitter.ParseActionEntry {
	if len(groups) == 0 {
		return nil
	}

	maxIdx := 0
	for _, ag := range groups {
		if ag.Index > maxIdx {
			maxIdx = ag.Index
		}
	}

	groupMap := make(map[int]*ActionGroup, len(groups))
	for i := range groups {
		groupMap[groups[i].Index] = &groups[i]
	}

	entries := make([]gotreesitter.ParseActionEntry, maxIdx+1)
	for i := 0; i <= maxIdx; i++ {
		ag, ok := groupMap[i]
		if !ok {
			continue
		}

		dst := gotreesitter.ParseActionEntry{
			Reusable: ag.Reusable,
		}
		if len(ag.Actions) > 0 {
			dst.Actions = make([]gotreesitter.ParseAction, len(ag.Actions))
			for j, a := range ag.Actions {
				action := gotreesitter.ParseAction{
					State:             gotreesitter.StateID(a.State),
					Symbol:            gotreesitter.Symbol(a.Symbol),
					ChildCount:        uint8(a.ChildCount),
					DynamicPrecedence: int16(a.Precedence),
					ProductionID:      uint16(a.ProductionID),
					Extra:             a.Extra,
					Repetition:        a.Repetition,
				}
				switch a.Type {
				case "shift":
					action.Type = gotreesitter.ParseActionShift
				case "reduce":
					action.Type = gotreesitter.ParseActionReduce
				case "accept":
					action.Type = gotreesitter.ParseActionAccept
				case "recover":
					action.Type = gotreesitter.ParseActionRecover
				default:
					action.Type = gotreesitter.ParseActionRecover
				}
				dst.Actions[j] = action
			}
		}
		entries[i] = dst
	}
	return entries
}

func convertLexStates(src []LexStateEntry) []gotreesitter.LexState {
	dst := make([]gotreesitter.LexState, len(src))
	for i, s := range src {
		state := gotreesitter.LexState{
			AcceptToken: gotreesitter.Symbol(s.Accept),
			Skip:        false,
			Default:     -1,
			EOF:         s.EOF,
		}
		if len(s.Transitions) > 0 {
			state.Transitions = make([]gotreesitter.LexTransition, len(s.Transitions))
			for j, t := range s.Transitions {
				state.Transitions[j] = gotreesitter.LexTransition{
					Lo:        t.Lo,
					Hi:        t.Hi,
					NextState: t.Next,
					Skip:      t.Skip,
				}
			}
		}
		dst[i] = state
	}
	return dst
}

// EncodeLanguageBlob serializes a language using gob+gzip.
func EncodeLanguageBlob(lang *gotreesitter.Language) ([]byte, error) {
	var out bytes.Buffer
	gzw := gzip.NewWriter(&out)
	if err := gob.NewEncoder(gzw).Encode(lang); err != nil {
		_ = gzw.Close()
		return nil, fmt.Errorf("encode language blob: %w", err)
	}
	if err := gzw.Close(); err != nil {
		return nil, fmt.Errorf("finalize language blob: %w", err)
	}
	return out.Bytes(), nil
}

// GenerateEmbeddedGo produces a small wrapper that lazy-loads an embedded blob.
func GenerateEmbeddedGo(g *ExtractedGrammar, pkg string, blobName string) string {
	var buf strings.Builder

	buf.WriteString("// Code generated by ts2go. DO NOT EDIT.\n\n")
	fmt.Fprintf(&buf, "package %s\n\n", pkg)
	buf.WriteString("import \"github.com/odvcencio/gotreesitter\"\n\n")

	funcName := languageFuncName(g.Name)
	fmt.Fprintf(&buf, "// %s returns the %s language definition.\n", funcName, g.Name)
	fmt.Fprintf(&buf, "func %s() *gotreesitter.Language {\n", funcName)
	fmt.Fprintf(&buf, "\treturn loadEmbeddedLanguage(%q)\n", blobName)
	buf.WriteString("}\n")

	return buf.String()
}
