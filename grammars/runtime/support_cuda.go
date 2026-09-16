//go:build !grammar_subset || grammar_subset_cuda

package grammarruntime

// RegisterCudaSupport registers the scanner support for cuda.
func RegisterCudaSupport() {
	RegisterExternalScanner("cuda", CudaExternalScanner{})
}
