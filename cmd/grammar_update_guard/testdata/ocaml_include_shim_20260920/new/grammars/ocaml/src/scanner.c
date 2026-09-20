// Synthetic fixture standing in for tree-sitter-ocaml's scanner.c shim,
// which does not define tree_sitter_ocaml_external_scanner_scan itself: its
// whole body #includes a scanner implementation shared across the upstream
// repo's several grammars. Not vendored upstream source.
#include "../../../common/scanner.h"
