# Doxygen normalization blocker receipt proposal

Receipt date: 2026-08-22.

Base commit: `0c34a681db29a3e8d27e488c5f26d6d7a5f02592`.

Disposition: `NO-GO`. Keep `dispatch.doxygen` live. Do not remove the arm,
its source file, or its registry entry.

## Registry denominator

The registry test passed with these values:

| Measure | Value |
| --- | ---: |
| Dispatcher arms | 31 |
| Dispatcher languages | 33 |
| Dispatcher predicates | 1 |
| Generic passes | 0 |
| Post-finalization arms | 0 |
| Post-finalization languages | 0 |
| Live entries | 32 |
| Retired entries | 56 |
| Live language labels | 35 |

The A0 manifest test passed with 14 languages and 14 receipts after JSDoc
retirement. Doxygen retains three A0 fixtures.

## A0 raw and production receipt

All three registered A0 Doxygen fixtures produced equal raw and production
trees. Each route reported zero rewrites.

| Witness | Source SHA-256 | Raw and production digest | Root | Error |
| --- | --- | --- | --- | --- |
| `medium__CMakeLists.txt` | `66408d6539b27d7c49b1e51777605c38c91b6d924267db5109ee00e2a1cfcf41` | `01d09d1ffd9d09af0333bcd887c35e68bcb4a96d15ff0d96c29a1780971b7e04` | `document` | true |
| `medium__metrics.py` | `31622a6c075ffa6f78a16af6e379f517213d42ff67729bbd0d10551c5fca9702` | `5adbacb1ec949237a802a56a5c95c3c7a1ce17fe9c8db5423b63f083da62d5d1` | `ERROR` | true |
| `small__example.cfg` | `86998161914382f8152e4984db091e7bf486799c1091fc6c57db4e704eee4a3b` | `3b803e3d4b9ffcf99c771c352118f3f7026420ea5f26c8d934349ac848789b23` | `document` | true |

The A0 census receipt is `files=3 checked=2 run=2 visited=4 rewritten=0
error_roots=3 parse_errors=0`. The arm does not record a pass for the
`ERROR` root in `medium__metrics.py`.

The production, compact, forest, and incremental route receipt passed for all
three fixtures. The compact routes fell back with `parser-core fresh-full
runner did not accept EOF`. The forest routes fell back at `dead_end`. The
incremental routes fell back at `external_scanner_unsupported`. Each covered
route recorded zero Doxygen rewrites.

## Locked-C blocker

The one-grammar locked-C deep probe used the Doxygen lock
`https://github.com/amaanq/tree-sitter-doxygen` at commit
`ccd998f378c3f9345ea4eeb223f56d7b84d16687`. It used the C runtime contract
`tree-sitter-c-v1`, binding `github.com/tree-sitter/go-tree-sitter@v0.25.0`,
runtime `0.25.1`, and the locked C deep digest.

The production route diverges from locked C for every A0 fixture:

- `medium__CMakeLists.txt`: Go digest `01d09d1ffd9d09af0333bcd887c35e68bcb4a96d15ff0d96c29a1780971b7e04`; C digest `d6f623d2b87344001e98de5528b44e38b102e564491871a9ffb64c1b73d193c5`; first divergence `/document`, type `document` versus `ERROR`.
- `medium__metrics.py`: Go digest `5adbacb1ec949237a802a56a5c95c3c7a1ce17fe9c8db5423b63f083da62d5d1`; C digest `6660931c2bf1bf1e0f909a1cac1e4cd8446853ae4466781c943e28fbcc61e860`; first divergence `/ERROR`, children `0` versus `279`.
- `small__example.cfg`: Go digest `3b803e3d4b9ffcf99c771c352118f3f7026420ea5f26c8d934349ac848789b23`; C digest `f1938d5c7bc544856a5df6c204af75af10a5395bd1f89f560c74caef5acf191f`; first divergence `/document`, type `document` versus `ERROR`.

The registered smoke witness passes locked C exactly. Its source SHA-256 is
`e2d564b999c40b0a53450771ffa82adf7880375449e8628fefd118aae21056d7`, and
its raw, production, and C digest is
`1ae089a98760be594f06d0820951e01714097e99621cc2cd4428ce09ba867083`.

## Historical trigger evidence

The live arm has actual rewrites on the historical trigger shapes that its
unit test protects:

- Childless whole-comment `ERROR`: source SHA-256
  `ff90d209911d0d32bf44ebff0742e6f42ff40a6f4978860a00ec3f7228b2af24`.
  The direct arm changes `(ERROR (ERROR (_multiline_begin)))` to `(ERROR)`;
  the transcript records a root `child_count_delta=-1` and the census records
  `3` rewritten nodes. The production digest is
  `0e1129b2130636e62dd05b2494c22a9a2b5b6ec044aea2eeb4dc836380e38b38`, which
  matches locked C. Raw digest is
  `6c16ff1b99a3b116d575f90aa0fe5456381b442a58af021dac36e6954345ce4c`.
- Recovered document: source SHA-256
  `f6deae068bcf0fe684f8623d671ee5dfbfab47c93d7827ec03c3b4b5330f8309`.
  The direct arm changes the root from `ERROR` to `document`, removes two
  root children, and records `14` rewritten nodes. The production digest is
  `21374502deb13653ec081dd59a4e21311f501aa9adfd34ea1fe3a2f09bc5f8d5`.
  Locked C reports digest
  `05813d8b13788902a7f9b9322ca16127ecf5e9c3694d60726acc7a511be622fe` and
  first diverges at `/document`, children `4` versus `3`.

The route receipt confirms that these rewrites remain live on every covered
route. The childless witness reports `3` rewrites on compact fallback at
`compact route error`, forest fallback at `eof_no_root`, and incremental
fallback at `external_scanner_unsupported`. The recovered-document witness
reports `14` rewrites on compact fallback at `accepted-leaf-tiling-gap`,
forest fallback at `dead_end`, and incremental fallback at
`external_scanner_unsupported`.

## Census and structural trace

The mounted real-corpus census passed with 20 languages, 5 inert arms, 15
active arms, 14 uncovered registered arms, and 31 languages without a
dispatcher arm. Doxygen was uncovered because the mounted real corpus has no
Doxygen directory. The dedicated A0 fixtures supply its current firing
receipt.

Canopy traced the call path as follows:

`runLanguageResultCompatibility` -> `dispatcherArmCensus` ->
`normalizeDoxygenCompatibility` -> `normalizeDoxygenWholeBlockCommentError`
-> `doxygenErrorTreeHasRecoveredStructure` and
`retypeDoxygenRecoveredErrorRoot`.

## Docker artifacts

- A0 registry and route receipt:
  `harness_out/docker/20260822T201143Z-doxygen-a0-routes-final-main`.
- Locked-C deep parity receipt:
  `harness_out/docker/20260822T200654Z-doxygen-locked-c-deep-main`.
- Historical rewrite route receipt:
  `harness_out/docker/20260822T201217Z-doxygen-historical-routes-final-main`.
- Direct-arm transcript receipt:
  `harness_out/docker/20260822T201002Z-doxygen-direct-arm-transcript-main`.
- Post-JSDoc full census with the corpus mounted:
  `harness_out/docker/20260822T200749Z-doxygen-census-after-jsdoc-main-mounted`.
- Post-JSDoc registry and A0 manifest gates:
  `harness_out/docker/20260822T200855Z-doxygen-registry-post-jsdoc-main2`.

No production source, registry entry, changelog, or normalizer arm changed.
