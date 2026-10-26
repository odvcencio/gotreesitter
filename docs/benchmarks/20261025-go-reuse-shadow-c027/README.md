# Go reuse shadow: C 0.27.1 supplement

The frozen native Go authentication shadow also passes the three existing Go
parity families against C 0.27.1: **127 test records**, plus the independent
header/link probe. This adds an oracle qualification to the [shadow receipt](../20261025-go-reuse-shadow/README.md).
The fresh verifier stays active. The shadow's work/reuse regressions still
block performance qualification and promotion; no counter exception is inferred.
All merges to main remain held.

The probe identifies C commit `10eb5ac119eec9cb4267a8e035066473983f5f3a`.
The 27-target test map contains the exact 11-target shadow runtime
`b8516549c515222d8aeb5d4b4a2609b50d8abbcbd4b691e091d73aff7f40f49d`
and the already frozen private C binding/probe controls. No compact raw-output
prototype is layered into this run. The unchanged tests cover Cognociente's
original 140,368-byte top edit on both APIs/routes, malformed declarations,
and 96 assignment-target cases. The run uses the pinned Go 1.26.6 Docker image,
one CPU, 4 GiB, no network and the shared measurement lock. All eight source,
control, oracle and binding guards pass before and after execution.

This tests default parsing/closing, tree editing and public node reads through
the prior private adaptation of the v0.25 Go binding. It does not establish
general C 0.27 binding compatibility, change repository dependencies, measure
speed, or qualify compact-origin reuse, the full grammar fleet or graduation.

The supplement retains the actual commands, composed map, logs and guards.
Its source bytes are bound to the two immutable archives named in
[summary.json](summary.json); no archived source is replaced. Run:

```sh
python3 docs/benchmarks/20261025-go-reuse-shadow-c027/validate-evidence.py
```

This offline validation checks the recorded execution and source bindings;
it does not rerun the parser.
