package grammars

import (
	"sort"

	"github.com/odvcencio/gotreesitter"
)

// ParseBackend describes how a language can be parsed in this runtime.
type ParseBackend string

const (
	ParseBackendUnsupported ParseBackend = "unsupported"
	ParseBackendDFA         ParseBackend = "dfa"
	ParseBackendTokenSource ParseBackend = "token_source"
)

// ParseSupport summarizes parser support status for one registered language.
type ParseSupport struct {
	Name                    string
	LanguageVersion         uint32
	VersionCompatible       bool
	Backend                 ParseBackend
	Reason                  string
	HasTokenSourceFactory   bool
	HasDFALexer             bool
	RequiresExternalScanner bool
	HasExternalScanner      bool
}

// EvaluateParseSupport reports whether a language can parse using either the
// built-in DFA lexer or a registered custom token source factory.
func EvaluateParseSupport(entry LangEntry, lang *gotreesitter.Language) ParseSupport {
	report := ParseSupport{
		Name:                    entry.Name,
		LanguageVersion:         lang.Version(),
		VersionCompatible:       lang.CompatibleWithRuntime(),
		HasTokenSourceFactory:   entry.TokenSourceFactory != nil,
		HasDFALexer:             len(lang.LexStates) > 0,
		RequiresExternalScanner: lang.ExternalTokenCount > 0,
		HasExternalScanner:      lang.ExternalScanner != nil,
		Backend:                 ParseBackendUnsupported,
	}

	if !report.VersionCompatible {
		report.Reason = "language version is incompatible with runtime"
		return report
	}

	if report.HasTokenSourceFactory {
		report.Backend = ParseBackendTokenSource
		report.Reason = "custom token source factory"
		return report
	}

	if !report.HasDFALexer {
		report.Reason = "missing DFA lexer tables (LexStates)"
		return report
	}

	if report.RequiresExternalScanner && !report.HasExternalScanner {
		report.Reason = "requires external scanner, but none is registered"
		return report
	}

	report.Backend = ParseBackendDFA
	report.Reason = "dfa lexer"
	return report
}

// AuditParseSupport evaluates parse support for all registered languages.
func AuditParseSupport() []ParseSupport {
	entries := AllLanguages()
	reports := make([]ParseSupport, 0, len(entries))
	for _, entry := range entries {
		lang := entry.Language()
		reports = append(reports, EvaluateParseSupport(entry, lang))
	}
	sort.Slice(reports, func(i, j int) bool {
		return reports[i].Name < reports[j].Name
	})
	return reports
}

func defaultTokenSourceFactory(name string) func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
	switch name {
	case "c":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewCTokenSourceOrEOF(src, lang)
		}
	case "cpp":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewGenericTokenSourceOrEOF(src, lang)
		}
	case "go":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewGoTokenSourceOrEOF(src, lang)
		}
	case "html":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewHTMLTokenSourceOrEOF(src, lang)
		}
	case "java":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewJavaTokenSourceOrEOF(src, lang)
		}
	case "javascript":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewGenericTokenSourceOrEOF(src, lang)
		}
	case "json":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewJSONTokenSourceOrEOF(src, lang)
		}
	case "lua":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewLuaTokenSourceOrEOF(src, lang)
		}
	case "rust":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewGenericTokenSourceOrEOF(src, lang)
		}
	case "typescript":
		return func(src []byte, lang *gotreesitter.Language) gotreesitter.TokenSource {
			return NewGenericTokenSourceOrEOF(src, lang)
		}
	default:
		return nil
	}
}
