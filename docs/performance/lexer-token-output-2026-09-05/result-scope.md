# Caller-owned lexer token experiment

Date: 2026-09-05.
Baseline: `da1150c6d6a2d581ce31f44ba4c5b8241ec431ae`.
Candidate: the same commit with the adjacent three-file patch.

The candidate writes contiguous scan results into caller-owned token storage.
It preserves value-returning wrappers and the existing included-range scanner.
The changed callers cover ordinary tokenization, error probes, and authenticated edit proofs.
No grammar-specific branch, admission policy, or parser graduation rule changes.
Neither earlier unaccepted experiment is included.

The compiler removes five complete token transfers on successful contiguous scans.
That removes 440 copied bytes per raw scan in the inspected binary.
It also adds conditional write barriers and increases the contiguous scanner frame.
Read `compiler-result.md` for the exact tradeoffs and instruction evidence.
These instructions establish no speed improvement without measurements.

## Correctness scope

Focused controls passed: 57 baseline tests and 60 candidate tests.
New tests cover poisoned output, output reuse, failed probes, Unicode frontiers, and included-range gaps.
Separate Go, JSON, and JavaScript C-oracle containers completed successfully.
Go repair and reuse controls passed on both parser routes.
JSON and JavaScript passed fresh and incremental structural parity without skips.
These cases do not establish universal language parity.

The untimed candidate preflight uses 500 generated Go functions and 19,294 bytes.
All five edits matched fresh deep trees and reused the root without reparsing or allocating parser nodes.
Each edit performed one authenticated dependency check.
The initial parse used the default compact route without fallback.
The preflight inspects trees between edits; the timed lifecycle does not.
This campaign does not repeat the baseline preflight.

## Performance scope

The primary campaign uses the unchanged generated Go benchmark trio.
A separate campaign uses the frozen human-authored `grammargen_lr` fixture with 235,626 bytes.
Both campaigns were prepared before observing candidate timing results.
Both use twenty alternating paired seeds with the repository randomized driver and shared lock.
Each process uses 750 milliseconds, count=1, GOMAXPROCS=1, and allocation reporting.

The human-authored benchmark pins the legacy parser and checks the frozen grammar and tree identity.
Its environment rejects GOT_* overrides.
Both timing campaigns exclude diagnostic statistics and the preflight overlay.
No C implementation or Go standard parser is timed.
Do not infer a universal speed improvement or C speed ratio.

The six memory probes use fresh legacy full-parse processes on the same frozen human-authored fixture.
Peak resident set size includes admission, validation, warm-up, and one parse.
These probes exercise the shared lexer, but they do not isolate temporary proof stack space.
They do not establish long-run retention or compact parser memory behavior.
