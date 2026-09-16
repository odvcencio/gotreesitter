//go:build !grammar_subset || grammar_subset_cuda

package grammarruntime

import gotreesitter "github.com/odvcencio/gotreesitter"

// External token indexes for the cuda grammar.
const (
	cudaTokRawStringDelimiter = 0
	cudaTokRawStringContent   = 1
)

const (
	cudaSymRawStringDelimiter gotreesitter.Symbol = 230
	cudaSymRawStringContent   gotreesitter.Symbol = 231
)

// CudaExternalScanner handles C++ R"delim(...)delim" raw string literals for CUDA.
type CudaExternalScanner struct{}

func (CudaExternalScanner) Create() any         { return rawStringCreate() }
func (CudaExternalScanner) Destroy(payload any) {}
func (CudaExternalScanner) Serialize(payload any, buf []byte) int {
	return rawStringSerialize(payload, buf)
}
func (CudaExternalScanner) Deserialize(payload any, buf []byte) { rawStringDeserialize(payload, buf) }

func (CudaExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	return rawStringScan(payload, lexer, validSymbols,
		cudaTokRawStringDelimiter, cudaTokRawStringContent,
		cudaSymRawStringDelimiter, cudaSymRawStringContent)
}
