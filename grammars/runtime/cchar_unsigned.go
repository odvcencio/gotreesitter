//go:build arm || arm64 || ppc64 || ppc64le || riscv64 || s390x

package grammarruntime

// cCharIsSigned is whether `char` is a signed type in C code built for this architecture; see cchar_signed.go.
const cCharIsSigned = false
