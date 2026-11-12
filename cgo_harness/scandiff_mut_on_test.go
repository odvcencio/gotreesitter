//go:build cgo && treesitter_c_parity && scandiff_mutants

package cgoharness

import "os"

// Self-test job only (build tag scandiff_mutants): SCANDIFF_MUTANT=<name> puts back the behaviour that a fix
// replaced, so that the test of that fix can be shown to fail without it. TestScannerDiff refuses to run in
// a build with this tag, and docker/scandiff.sh never builds with it.

const sdMutantsCompiled = true

func sdMut(name string) bool { return os.Getenv("SCANDIFF_MUTANT") == name }
