Known differences of the scanner differential test (cgo_harness/scandiff_test.go).

One optional file per grammar, `<grammar>.txt`. Each line approves ONE discrepancy of ONE call:

    <kind> <category> <input hash> <offset> <mask> <state hash> <C record hash> <Go record hash> <history hash> # <reason>

`kind` is found, symbol, start, end, point, lookahead, column, state, gopanic or overflow; `category` is
ordinary, mask, midchar, damaged or range (how the call was generated). The hashes are the first 12 hex
digits of the SHA-256 of the input bytes, of the saved state, of the C result, of the Go result and of the
earlier calls of the payload; the mask is the valid-symbol mask as 0 and 1; a reason is required. There is
no count and no wildcard, and a different result at the same input has another signature. Run with
`SCANDIFF_SIGS=<file>` to list the signatures of a run, review them, and copy the approved lines. The test
fails on every difference without a line, on a line that no call used, and on a file that does not parse. A
missing file approves nothing.
