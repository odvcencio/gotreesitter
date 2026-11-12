//go:build cgo && treesitter_c_parity && !scandiff_mutants

package cgoharness

// Normal and CI builds: the harness's mutant switches do not exist here.

const sdMutantsCompiled = false

func sdMut(name string) bool { return false }
