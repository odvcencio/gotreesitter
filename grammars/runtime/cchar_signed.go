//go:build !(arm || arm64 || ppc64 || ppc64le || riscv64 || s390x)

package grammarruntime

// cCharIsSigned is whether `char` is a signed type in C code built for this architecture. The pinned C scanners
// are compiled for the architecture the port runs on, so a port that depends on the signedness of `char`
// (CHAR_MAX, a `char` read from a saved state) follows this constant. `char` is signed on x86 and x86-64
// (System V psABI) and unsigned on ARM and AArch64, PowerPC, RISC-V and s390x (GCC's defaults for Linux).
// Architectures not listed in the build constraints are taken as signed.
const cCharIsSigned = true
