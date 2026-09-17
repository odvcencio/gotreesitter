//go:build !grammar_subset || grammar_subset_properties

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the properties grammar.
const (
	propertiesTokEof = 0
)

const (
	propertiesSymEof gotreesitter.Symbol = 16
)

type propertiesScannerState struct {
	emittedEOF bool
}

// PropertiesExternalScanner handles EOF detection for Java .properties files.
type PropertiesExternalScanner struct{}

func (PropertiesExternalScanner) Create() any {
	return &propertiesScannerState{}
}

func (PropertiesExternalScanner) Destroy(payload any) {}

func (PropertiesExternalScanner) Serialize(payload any, buf []byte) int {
	st := payload.(*propertiesScannerState)
	if len(buf) == 0 {
		return 0
	}
	if st.emittedEOF {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	return 1
}

func (PropertiesExternalScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*propertiesScannerState)
	if len(buf) == 0 {
		st.emittedEOF = false
		return
	}
	st.emittedEOF = buf[0] != 0
}

func (PropertiesExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	st := payload.(*propertiesScannerState)
	if !propertiesValid(validSymbols, propertiesTokEof) {
		return false
	}
	if lexer.Lookahead() == 0 {
		if st.emittedEOF {
			return false
		}
		st.emittedEOF = true
		lexer.MarkEnd()
		lexer.SetResultSymbol(propertiesSymEof)
		return true
	}
	st.emittedEOF = false
	return false
}

func propertiesValid(vs []bool, i int) bool { return i < len(vs) && vs[i] }
