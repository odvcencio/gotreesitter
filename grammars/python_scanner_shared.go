//go:build !grammar_subset || grammar_subset_python || grammar_subset_bitbake || grammar_subset_mojo || grammar_subset_starlark

package grammars

import "github.com/odvcencio/gotreesitter/grammars/internal/pythonruntime"

// Keep derivative scanner state independent of Python grammar registration.
type pythonScannerState = pythonruntime.State
type pyDelimiter = pythonruntime.Delimiter

const (
	pyDelimSingleQuote = pythonruntime.DelimiterSingleQuote
	pyDelimDoubleQuote = pythonruntime.DelimiterDoubleQuote
	pyDelimBackQuote   = pythonruntime.DelimiterBackQuote
	pyDelimRaw         = pythonruntime.DelimiterRaw
	pyDelimFormat      = pythonruntime.DelimiterFormat
	pyDelimTriple      = pythonruntime.DelimiterTriple
	pyDelimBytes       = pythonruntime.DelimiterBytes
)

func serializePythonScannerState(state *pythonScannerState, buffer []byte) int {
	return pythonruntime.SerializeState(state, buffer)
}

func deserializePythonScannerState(state *pythonScannerState, buffer []byte) {
	pythonruntime.DeserializeState(state, buffer)
}
