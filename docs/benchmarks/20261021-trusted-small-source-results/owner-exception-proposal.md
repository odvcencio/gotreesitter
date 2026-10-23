# Proposed R3 exception: authenticated small-source reuse

Status: proposed; owner approval is pending. This exception proposal applies no runtime patch and resets no ledger row.

The request covers only the pinned V4 patch and exact old→new counter values listed below. It does not waive large-file RSS, native-platform qualification, C correctness, incremental invariants, or compact graduation. A composition with another runtime change must be qualified again and must not introduce additional adverse values.

The private V4 candidate removes the redundant fresh verification parse for clean edits whose actual default DFA producer, complete reads, source identity and dispatch schedule are authenticated. On the exact Cogno Go fixture, 20 matched pairs on an M1 Max under Docker linux/arm64 show plain insert API time falling **107.981→5.705 ms** and replace **95.971→5.697 ms** (both p<.001). This receipt changes no runtime, allowlist, ledger baseline or approval.

The candidate remains held. The selected ledger has **178 adverse guarded changes**, including **142 above 2%**; the complete-operation view has **158 event increases**. [Every changed selected field and adverse operation event](counter-changes.md) is listed. [R3](../../v1-design.md#r3-counter-ledger-changes) says: “Agents do not approve ledger exceptions.” No exception is approved here.

## Measured scope

The source is 140,368 bytes, 1,760 functions, SHA-256 `5434575bb249338937331c3756e10dcec34237d107f523ea315996fb6de0ef8f`. Edits target byte 57, row 5, column 1, using the [October 21 downstream report](https://github.com/odvcencio/gotreesitter/issues/454#issuecomment-6311551850). The comparison is current D64 runtime against private V4, **not** v0.55.1 against v0.56.0. A separate native Windows amd64 comparison appears below; Windows ARM64 and a release-to-release comparison remain unverified.

Every edited benchmark creates a native old tree outside its timed region, applies Tree.Edit under a separate timer, then times the public incremental API and both owned-handle releases. Copy is not substituted for the native producer. No-edit benchmarks retain one native old tree for the entire benchmark and release each returned handle. Both plain and profiled APIs run. All processes use one explicit seed, count=1, GOMAXPROCS=1, 750ms and benchmem; 20 matched pairs alternate revision order and preserve identical child order. No pool or parser budget override is applied.

| Public API workload | D64 median | V4 median | Result |
|---|---:|---:|---|
| Plain insert | 107.981 ms | 5.705 ms | −94.72%, p<.001 |
| Plain replace | 95.971 ms | 5.697 ms | −94.06%, p<.001 |
| Profiled insert | 99.209 ms | 6.512 ms | −93.44%, p<.001 |
| Profiled replace | 105.915 ms | 7.434 ms | −92.98%, p<.001 |
| Plain delete | 296.3 ms | 285.6 ms | inconclusive, p=.678 |
| Profiled delete | 296.2 ms | 289.2 ms | inconclusive, p=.925 |
| Fresh full parse | 83.14 ms | 88.66 ms | inconclusive, p=.738 |
| Plain no edit | 69.74 ns | 65.81 ns | inconclusive, p=.835 |
| Profiled no edit | 138.9 ns | 138.0 ns | inconclusive, p=.433 |

Plain insert allocated bytes fall 555.5 KiB→1,160 B and allocations 37→7. Plain replace falls 107.5 KiB→1,160 B and 34→7. Delete allocated bytes fall 5.94% and allocations fall 100→97 plain, 102→99 profiled. No edit remains zero bytes and zero allocations. See the raw output and full benchstat tables for every metric and directional change. Timing variability is large on this host; these results do not establish native Windows ratios or a delete-time win.

## Tree.Edit cost remains visible

Tree.Edit regresses: plain replace **416.1 µs→1.712 ms** (+311.50%), insert **2.306→3.478 ms** (+50.79%) and delete **2.204→3.232 ms** (+46.67%). All are significant in this sample. Profiled counterparts also rise; all six rows appear in `cogno-benchstat.txt`.

For context, adding each process's separately measured Tree.Edit and API averages gives descriptive medians **110.263→9.133 ms** for plain insert and **96.404→7.396 ms** for replace. These sums exclude old-tree setup and do not carry a new significance claim. They do not hide the Tree.Edit regression or replace its separate attribution.

The standard production Go trio has no significant time change. All six YAML workload/mode time differences are inconclusive. Ordinary YAML still falls back; only the explicit diagnostic-owned cases complete on compact. This is not YAML ownership activation or compact graduation.

## Correctness and counters

- 1,236 standard locked-C checks pass: 206 languages, two routes, three checks, with no skips or OOM.
- Eight exact Cogno insert/replace C cases pass across both routes and both APIs. The known reduced Go delete lexical-context mismatch with C remains preexisting and unfixed.
- 412 canonical invariant sessions pass 72 edits each (29,664 steps), including zero no-edit allocations. Twenty memory contracts pass. This is not a new large-file RSS qualification.
- The exact Djot/TOML corpus first edits pass the existing caller-route fresh/C regression. A private forced-legacy Djot reference retains its known false error; that raw diagnostic stays failed.
- Complete-operation attribution covers all 412 fixture/route rows. Full Parse+Release and separate Edit+Profiled API+owned releases include hidden verifiers/retries. Their phase and attempt partitions are exact. On the Cogno production insert/replace, attempts fall **2→1**, shifts **72,200→1,792**, reductions **75,729→1,803**. Delete remains **5→5**; four extra shifts and a 0.345% table-lookup increase remain recorded.
- The selected ledger retains 92 unavailable candidate full profiles as unchanged declines. No zeros are fabricated. All full selected profiles and all candidate selected profiles are unchanged. The proposed default first-edit changes are held for review.

## Native Windows comparison

A separate native Windows run now validates the exact Cogno workload on **Intel Core Ultra 9 285, Windows amd64, Go 1.27.0**. This compares D64 with held V4; it does not compare releases or validate Windows ARM64. All 4,331 baseline and 4,332 candidate files match the qualified input manifests byte-for-byte after execution, and both benchmark binaries retain their original hashes.

Twenty matched pairs use explicit seeds `140001..140015` and `140022..140026`, GOMAXPROCS=1, count=1, 750ms, benchmem and one pinned logical CPU. The analyzed 40 processes retain identical workload order within each pair and alternate revision order globally. The unchanged repository runner drives the comparison. A WSL launch failure stopped the first collection after 15 pairs; Windows process inspection confirmed that the next benchmark had not started. Five incorrectly role-inverted resume pairs were then rejected solely on ordering metadata before statistical use, and five new seeds replaced that whole batch. Every failed launch, excluded sample, command, native receipt and script remains in the [287-member verified archive](windows-receipts.tar.gz). No successful native sample was rerun, and no GC/pool/parser-budget override is applied.

| Public API workload | D64 median | V4 median | Result |
|---|---:|---:|---|
| Plain insert | 117.43 ms | 10.54 ms | −91.02%, p<.001 |
| Plain replace | 119.09 ms | 10.65 ms | −91.06%, p<.001 |
| Profiled insert | 115.55 ms | 10.83 ms | −90.62%, p<.001 |
| Profiled replace | 117.05 ms | 10.57 ms | −90.97%, p<.001 |
| Plain delete | 363.5 ms | 371.0 ms | inconclusive, p=.989 |
| Profiled delete | 365.0 ms | 365.7 ms | inconclusive, p=.779 |
| Fresh full parse | 104.4 ms | 103.9 ms | inconclusive, p=.904 |
| Plain no edit | 28.53 ns | 27.58 ns | inconclusive, p=.068 |
| Profiled no edit | 56.13 ns | 55.11 ns | inconclusive, p=.387 |

Tree.Edit remains a significant regression on Windows too:

| Tree.Edit workload | D64 median | V4 median | Change |
|---|---:|---:|---:|
| Plain insert | 3.483 ms | 4.866 ms | +39.68% |
| Plain replace | 1.228 ms | 2.732 ms | +122.43% |
| Plain delete | 3.350 ms | 4.892 ms | +46.01% |
| Profiled insert | 3.344 ms | 4.823 ms | +44.23% |
| Profiled replace | 1.087 ms | 2.648 ms | +143.51% |
| Profiled delete | 3.557 ms | 5.177 ms | +45.53% |

All six Tree.Edit differences have p<.001. Descriptive per-process API-plus-Edit medians improve **120.832→15.602 ms** for plain insert and **120.280→13.280 ms** for replace; these sums exclude old-tree setup and have no new significance claim. Plain API allocations fall **53→13** for insert and **50→13** for replace; both no-edit APIs retain zero bytes and allocations. The full [benchstat table](windows-benchstat.txt) retains every metric and directional result. Its whitespace/labels are shortened for reading; the original tool output is in the archive.

The [collection audit](windows-collection-audit.json) verifies every selected process, seed, flag, source and binary hash. The [summary](windows-summary.json) retains exact numeric medians and descriptive sums. The archive pins all member hashes, includes the native PowerShell driver, strict Go-test flag bridge, compile commands and source verifier, and excludes executables and private Buckley logs. Start from the same curated source manifests and runtime/harness patches already listed below; only the execution platform changes. Native benchmark preflights compare each edited result with fresh parsing. The existing Linux C/invariant qualification remains separate; this is not a native Windows C-oracle sweep or a new large-file RSS pass.

This evidence changes none of the proposed counter values. R3 approval, the known Go-delete C mismatch, ordinary YAML ownership and compact graduation remain unresolved.

## Exact exception scope and reasons

The complete-work capture retains 161 stricter historical-state/BOM/reference diagnostic failures. Its forced-legacy Djot public-shape difference is also present in D64; the established caller-route/fresh/C check passes. These raw failures remain failed and are not new correctness passes. See [the full diagnostic distinction](README.md#historical-state-diagnostics).

R3 says the PR body must list every changed row, its old/new values and the reason. These tables include all 238 selected-field changes and all 1,457 complete-operation changes, including improvements and informational fields. The views overlap and must not be added as disjoint work. The 92 declined full candidate profiles remain unavailable, not zero. All complete full operations, all full selected profiles and all candidate selected profiles are unchanged.

**M1**, for every selected row: the authenticated native incremental result changes selected token/node/reuse accounting. The owner is asked to accept these exact adverse values for the measured complete-operation improvement above. No per-language timing win is claimed for the other languages.

**M2**, for every complete-operation row: the same producer-proof and eligible-retry change alters the full attempt sequence, including verifiers and owned-handle releases. Every event increase is retained for review. This is a measured trade request, not a per-language speed claim.

Each cell reads `metric=before→after`; `*` marks an adverse selected field or an increasing operation event according to the existing audits. All selected rows below are the corpus **default route, first insert, edit step 1**. The operation table names every changed route and uses phase `edit_operation`. Aliases are exact metric names, including diagnostic proxy suffixes.

### Selected metrics

- `S1` = `block_splices`
- `S2` = `max_live_versions`
- `S3` = `multi_version_token_share_ppm`
- `S4` = `new_nodes`
- `S5` = `reused_bytes`
- `S6` = `tokens`

| Language | Changed values | Reason |
|---|---|---|
| bass | S1*=27→26; S4*=108→109; S5*=972→966; S6*=46→47 | M1 |
| c | S1*=148→89; S2=8→7; S3=165625→160194; S4=1274→835; S5=1340→1802; S6=320→206 | M1 |
| circom | S1*=129→11; S3=314814→705069; S4=1405→821; S5=0→1397; S6=486→217 | M1 |
| css | S1*=34→32; S4*=65→67; S5*=2280→2278; S6*=17→19 | M1 |
| cue | S1*=76→74; S4*=410→412; S5*=1361→1346; S6*=82→84 | M1 |
| cylc | S1*=15→6; S3=540293→521201; S4*=3817→3869; S5*=238→134; S6*=546→566 | M1 |
| devicetree | S1=14→127; S4*=24→525; S5*=1984→492; S6*=11→264 | M1 |
| editorconfig | S1=18→20; S4*=180→200; S5*=876→738; S6*=84→94 | M1 |
| eex | S1*=39→38; S4*=41→46; S5*=1736→1727; S6*=2→5 | M1 |
| elisp | S1*=20→19; S4*=5→6; S5*=1905→1841; S6*=3→4 | M1 |
| elixir | S1=38→92; S2=12→5; S3=692307→156716; S4=2004→1039; S5*=1134→879; S6*=221→268 | M1 |
| embedded_template | S1=13→14; S4=314→301; S5=355→375; S6=144→137 | M1 |
| erlang | S1*=63→61; S4*=159→197; S5*=1757→1727; S6*=33→43 | M1 |
| faust | S1=43→91; S4*=161→566; S5*=1889→1472; S6*=47→183 | M1 |
| fennel | S1*=37→36; S4*=302→303; S5*=3752→3746; S6*=73→74 | M1 |
| fidl | S1*=46→45; S4*=63→64; S5*=1810→1723; S6*=18→19 | M1 |
| fish | S1*=61→56; S4*=97→102; S5*=1896→1866; S6*=13→18 | M1 |
| forth | S1*=51→39; S4*=54→89; S5*=1745→1590; S6*=2→14 | M1 |
| gitattributes | S1*=32→31; S4*=388→519; S5*=1169→1103; S6*=174→240 | M1 |
| gitignore | S1*=4→3; S4*=15→16; S5*=8→7; S6*=6→7 | M1 |
| gleam | S1*=42→38; S2*=1→3; S3=0→48780; S4*=115→150; S5*=1959→1925; S6*=29→41 | M1 |
| gn | S4*=107→110; S5*=1724→1723; S6*=6→7 | M1 |
| go | S1*=296→260; S2=12→8; S3=531468→495114; S4=3810→3219; S5*=1166→1044; S6*=572→614 | M1 |
| godot_resource | S4*=98→114; S5*=1769→1749; S6*=39→47 | M1 |
| graphql | S1*=172→171; S4*=376→377; S5*=854→850; S6*=83→84 | M1 |
| hare | S1=344→346; S4*=655→672; S5*=1393→1381; S6*=141→146 | M1 |
| hurl | S1*=173→163; S4*=873→917; S5*=1257→1247; S6*=287→297 | M1 |
| ini | S4*=18→27; S5*=756→750; S6*=11→20 | M1 |
| jq | S1=2→6; S3=468085→461538; S4*=724→728; S5*=41→30; S6*=141→143 | M1 |
| julia | S1=9→15; S3=975190→837704; S4*=4984→5279; S5*=799→601; S6*=524→610 | M1 |
| kconfig | S1=1→56; S2=12→2; S3=968421→0; S4=865→149; S5=294→1614; S6=95→40 | M1 |
| ledger | S1*=31→22; S3=16666→1751; S4*=141→1123; S5*=831→36; S6*=60→571 | M1 |
| llvm | S1*=254→225; S4*=378→670; S5*=6252→5853; S6*=64→172 | M1 |
| meson | S1*=131→73; S3=766310→772228; S4=8468→8083; S6=843→821 | M1 |
| move | S1*=113→109; S3=487046→477157; S4*=1092→1096; S5*=778→748; S6*=193→197 | M1 |
| objc | S1=8→74; S2=18→12; S3=671378→538834; S4=3747→1229; S5=0→729; S6=566→206 | M1 |
| pkl | S1=12→47; S4*=57→214; S5*=995→216; S6*=33→83 | M1 |
| powershell | S1*=216→149; S3=158620→90909; S4*=1536→2195; S5*=1190→295; S6*=290→462 | M1 |
| prolog | S1*=17→9; S4*=77→85; S5*=1860→1829; S6*=21→29 | M1 |
| ql | S1*=16→13; S4*=30→79; S5*=1964→1844; S6*=9→26 | M1 |
| regex | S1*=514→510; S4*=3589→3641; S5*=586→558; S6*=1596→1616 | M1 |
| robot | S1*=63→19; S3=464028→400621; S4*=956→1088; S5*=1002→92; S6*=278→322 | M1 |
| ron | S1*=83→58; S3=68181→43478; S4*=143→168; S5*=116→91; S6*=44→69 | M1 |
| smithy | S1*=88→18; S3=5494→3816; S4*=574→736; S5*=308→38; S6*=182→262 | M1 |
| sparql | S1*=141→2; S3=632352→484573; S4*=1149→1308; S5*=340→2; S6*=408→551 | M1 |
| ssh_config | S1*=40→39; S4*=132→133; S5*=1389→1319; S6*=47→48 | M1 |
| textproto | S1*=50→49; S4*=118→122; S5*=1700→1698; S6*=9→10 | M1 |
| thrift | S1*=53→22; S4=68→9; S5=942→1058; S6=19→2 | M1 |
| twig | S1*=2→1; S4*=8→19; S5*=42→14; S6*=3→7 | M1 |
| typescript | S1*=44→42; S4*=193→195; S5*=1776→1768; S6*=50→52 | M1 |
| v | S1*=117→96; S3=258373→234782; S4*=1121→1142; S5*=1437→1378; S6*=209→230 | M1 |
| vimdoc | S1=25→66; S4*=98→792; S5*=1927→429; S6*=28→256 | M1 |
| wat | S1*=68→37; S3=25974→18518; S4*=267→298; S5*=1151→999; S6*=77→108 | M1 |
| zig | S1=50→55; S4*=266→417; S5*=1819→1727; S6*=67→104 | M1 |

### Complete-operation metrics

- `W1` = `accept_actions`
- `W2` = `action_entries_examined_proxy`
- `W3` = `emitted_pop_paths`
- `W4` = `emitted_pop_payloads`
- `W5` = `graph_link_additions_proxy`
- `W6` = `leaf_constructions_proxy`
- `W7` = `lexer_front_door_calls_proxy`
- `W8` = `merge_attempts_proxy`
- `W9` = `merge_successes_proxy`
- `W10` = `parent_constructions_proxy`
- `W11` = `parse_attempts`
- `W12` = `reduction_pop_requests`
- `W13` = `reductions`
- `W14` = `shifts`
- `W15` = `stack_version_creations_proxy`
- `W16` = `table_lookups_proxy`

| Language | Route | Changed values | Reason |
|---|---|---|---|
| ada | default | W1=2→1; W10=345→7; W11=2→1; W12=343→6; W13=343→6; W14=282→1; W15=6→1; W16=1747→45; W2=320→3; W3=343→6; W4=648→11; W5=43→3; W6=282→1; W7=279→2 | M2 |
| apex | default | W1=2→1; W10=227→103; W11=2→1; W12=225→102; W13=225→102; W14=155→56; W15=20→10; W16=1557→730; W2=175→65; W3=225→102; W4=406→186; W5=166→84; W6=155→56; W7=127→42 | M2 |
| bass | default | W1=2→1; W10=433→63; W11=2→1; W12=431→62; W13=431→62; W14=325→46; W15=2→1; W16=1568→441; W2=453→59; W3=431→62; W4=903→179; W5=16→8; W6=325→46; W7=327→47 | M2 |
| bibtex | default | W1=2→1; W10=330→68; W11=2→1; W12=330→68; W13=330→68; W14=292→18; W15=2→1; W16=1195→440; W2=420→34; W3=330→68; W4=672→137; W5=2→1; W6=292→18; W7=294→19 | M2 |
| brightscript | candidate | W1=2→1; W10=481→8; W11=2→1; W12=536→9; W13=536→9; W14=315→2; W15=9→1; W16=2195→103; W2=392→5; W3=536→9; W4=803→11; W5=446→28; W6=315→2; W7=314→4; W8=8→0 | M2 |
| brightscript | default | W1=2→1; W10=481→8; W11=2→1; W12=536→9; W13=536→9; W14=315→2; W15=9→1; W16=1809→103; W2=392→5; W3=536→9; W4=803→11; W5=446→28; W6=315→2; W7=314→4; W8=8→0 | M2 |
| c | candidate | W10=1380→1127; W12=1298→1022; W13=1298→1022; W14=919→768; W15=1279→1042; W16=13822→11361; W2=2072→1692; W3=1298→1039; W4=2284→1843; W5=2693→2165; W6=1046→889; W7=750→636; W8=310→218; W9*=8→18 | M2 |
| c | default | W10=1380→1127; W12=1298→1022; W13=1298→1022; W14=919→768; W15=1279→1042; W16=13751→11290; W2=2072→1692; W3=1298→1039; W4=2284→1843; W5=2693→2165; W6=1046→889; W7=750→636; W8=310→218; W9*=8→18 | M2 |
| capnp | default | W1=2→1; W10=110→28; W11=2→1; W12=110→28; W13=110→28; W14=148→32; W15=2→1; W16=605→282; W2=168→39; W3=110→28; W4=354→114; W5=2→1; W6=148→32; W7=150→33 | M2 |
| circom | candidate | W10*=1009→1199; W12*=707→897; W13*=707→897; W14*=679→994; W15*=17→26; W16*=4724→5338; W2*=765→1123; W3*=707→897; W4*=1560→1994; W5*=643→1164; W6*=1027→1342; W7*=486→604; W8*=264→528 | M2 |
| circom | default | W10*=1009→1199; W12*=707→897; W13*=707→897; W14*=679→994; W15*=17→26; W16*=4255→4869; W2*=765→1123; W3*=707→897; W4*=1560→1994; W5*=643→1164; W6*=1027→1342; W7*=486→604; W8*=264→528 | M2 |
| clojure | default | W1=2→1; W10=285→107; W11=2→1; W12=285→107; W13=285→107; W14=163→46; W15=2→1; W16=951→490; W2=280→81; W3=285→107; W4=483→189; W5=2→1; W6=163→46; W7=165→47 | M2 |
| commonlisp | default | W1=2→1; W10=1719→855; W11=2→1; W12=1719→855; W13=1719→855; W14=765→338; W15=2→1; W16=4992→3154; W2=1307→572; W3=1719→855; W4=2565→1275; W5=2→1; W6=765→338; W7=767→339 | M2 |
| corn | default | W1=2→1; W10=20→10; W11=2→1; W12=20→10; W13=20→10; W14=16→6; W15=2→1; W16=99→48; W2=24→10; W3=20→10; W4=38→19; W5=2→1; W6=16→6; W7=18→7 | M2 |
| cpon | candidate | W16=2263→2262 | M2 |
| cpon | default | W16=2248→2247 | M2 |
| css | default | W1=2→1; W10=547→45; W11=2→1; W12=606→49; W13=606→49; W14=612→18; W15=2→1; W16=2364→239; W2=797→25; W3=606→49; W4=1264→101; W5=6→3; W6=612→18; W7=614→19 | M2 |
| csv | default | W1=2→1; W10=21→8; W11=2→1; W12=21→8; W13=21→8; W14=12→4; W15=2→1; W16=84→30; W2=16→6; W3=21→8; W4=33→13; W5=2→1; W6=12→4; W7=14→5 | M2 |
| cue | default | W1=2→1; W10=618→308; W11=2→1; W12=616→307; W13=616→307; W14=239→83; W15=2→1; W16=1961→1177; W2=269→97; W3=616→307; W4=907→450; W5=30→15; W6=239→83; W7=241→84 | M2 |
| cylc | candidate | W1=16→8; W10=4811→2420; W11=2→1; W12=5225→2627; W13=5225→2627; W14=2880→1446; W15=266→133; W16=16386→8030; W2=5970→2998; W3=5225→2627; W4=11156→5597; W5=8212→4107; W6=2880→1446; W7=1122→566; W8=5946→2973; W9=100→50 | M2 |
| cylc | default | W1=16→8; W10=4811→2420; W11=2→1; W12=5225→2627; W13=5225→2627; W14=2880→1446; W15=266→133; W16=15789→8030; W2=5970→2998; W3=5225→2627; W4=11156→5597; W5=8212→4107; W6=2880→1446; W7=1122→566; W8=5946→2973; W9=100→50 | M2 |
| desktop | default | W1=2→1; W10=158→79; W11=2→1; W12=158→79; W13=158→79; W14=176→78; W15=2→1; W16=592→382; W2=285→127; W3=158→79; W4=352→176; W5=2→1; W6=176→78; W7=178→79 | M2 |
| devicetree | default | W1=2→1; W10=276→262; W11=2→1; W12=274→261; W13=274→261; W14=400→263; W15=2→1; W16*=1289→2182; W2=447→284; W3=274→261; W4=682→648; W5=8→4; W6=400→263; W7=402→264 | M2 |
| dhall | default | W1=2→1; W10=749→374; W11=2→1; W12=783→391; W13=783→391; W14=278→86; W15=2→1; W16=2477→1507; W2=301→94; W3=783→391; W4=1161→579; W5=6→3; W6=278→86; W7=281→87 | M2 |
| diff | default | W1=2→1; W10=363→167; W11=2→1; W12=363→167; W13=363→167; W14=318→112; W15=2→1; W16=1487→969; W2=492→169; W3=363→167; W4=736→335; W5=2→1; W6=318→112; W7=320→113 | M2 |
| dot | default | W1=2→1; W10=1239→619; W11=2→1; W12=1407→703; W13=1407→703; W14=754→290; W15=2→1; W16=4645→3327; W2=950→356; W3=1407→703; W4=2333→1166; W5=2→1; W6=754→290; W7=756→291 | M2 |
| editorconfig | default | W1=2→1; W10=325→107; W11=2→1; W12=325→107; W13=325→107; W14=370→93; W15=2→1; W16=1151→390; W2=602→171; W3=325→107; W4=711→219; W5=2→1; W6=370→93; W7=372→94 | M2 |
| eds | candidate | W16=34020→34018 | M2 |
| eds | default | W16=33942→33940 | M2 |
| eex | default | W1=2→1; W10=141→42; W11=2→1; W12=141→42; W13=141→42; W14=108→4; W15=2→1; W16=526→241; W2=180→9; W3=141→42; W4=286→83; W5=2→1; W6=108→4; W7=110→5 | M2 |
| elisp | default | W1=2→1; W10=102→3; W11=2→1; W12=100→2; W13=100→2; W14=124→3; W15=2→1; W16=459→71; W2=152→5; W3=100→2; W4=212→6; W5=42→21; W6=124→3; W7=126→4 | M2 |
| elixir | candidate | W1=2→1; W10=1876→684; W11=2→1; W12=1848→657; W13=1848→657; W14=1246→350; W15=95→67; W16=8517→3226; W2=1391→405; W3=1875→684; W4=3110→1146; W5=2829→841; W6=1246→350; W7=581→268; W8=1652→31 | M2 |
| elixir | default | W1=2→1; W10=1876→684; W11=2→1; W12=1848→657; W13=1848→657; W14=1246→350; W15=95→67; W16=7567→3226; W2=1391→405; W3=1875→684; W4=3110→1146; W5=2829→841; W6=1246→350; W7=581→268; W8=1652→31 | M2 |
| elsa | default | W1=2→1; W10=511→27; W11=2→1; W12=509→26; W13=509→26; W14=374→9; W15=2→1; W16=1672→174; W2=428→11; W3=509→26; W4=977→70; W5=10→5; W6=374→9; W7=376→10 | M2 |
| embedded_template | default | W1=2→1; W10=398→165; W11=2→1; W12=398→165; W13=398→165; W14=355→136; W15=2→1; W16=1300→538; W2=692→263; W3=398→165; W4=764→314; W5=2→1; W6=355→136; W7=357→137 | M2 |
| enforce | default | W1=2→1; W10=14→7; W11=2→1; W12=18→9; W13=18→9; W14=11→4; W15=2→1; W16=179→48; W2=15→6; W3=18→9; W4=30→15; W5=2→1; W6=11→4; W7=13→5 | M2 |
| erlang | default | W1=2→1; W10=736→155; W11=2→1; W12=734→154; W13=734→154; W14=408→42; W15=610→111; W16=2352→627; W2=1043→156; W3=734→154; W4=1162→235; W5=1303→277; W6=408→42; W7=410→43 | M2 |
| faust | default | W1=2→1; W10=524→384; W11=2→1; W12=524→384; W13=524→384; W14=338→182; W15=2→1; W16*=1822→1999; W2=405→205; W3=524→384; W4=903→656; W5=2→1; W6=338→182; W7=340→183 | M2 |
| fennel | default | W1=2→1; W10=1701→214; W11=2→1; W12=1743→225; W13=1743→225; W14=856→73; W15=2→1; W16=5303→819; W2=993→84; W3=1743→225; W4=2644→329; W5=12→6; W6=856→73; W7=867→74 | M2 |
| fidl | default | W1=2→1; W10=117→46; W11=2→1; W12=115→45; W13=115→45; W14=112→18; W15=2→1; W16=531→321; W2=118→20; W3=115→45; W4=268→104; W5=10→5; W6=112→18; W7=114→19 | M2 |
| fish | default | W1=2→1; W10=819→85; W11=2→1; W12=819→85; W13=819→85; W14=340→17; W15=2→1; W16=2599→418; W2=723→36; W3=819→85; W4=1232→162; W6=340→17; W7=342→18 | M2 |
| foam | default | W1=2→1; W10=1261→337; W11=2→1; W12=1633→434; W13=1633→434; W14=436→57; W15=2→1; W16=4586→1503; W2=794→112; W3=1633→434; W4=2153→570; W5=6→3; W6=436→57; W7=444→61 | M2 |
| forth | default | W1=2→1; W10=339→76; W11=2→1; W12=339→76; W13=339→76; W14=162→13; W15=2→1; W16=1108→429; W2=355→29; W3=339→76; W4=550→127; W5=2→1; W6=162→13; W7=164→14 | M2 |
| git_config | default | W1=2→1; W10=86→40; W11=2→1; W12=86→40; W13=86→40; W14=80→36; W15=2→1; W16=359→201; W2=130→60; W3=86→40; W4=165→76; W5=2→1; W6=80→36; W7=82→37 | M2 |
| gitattributes | candidate | W10*=1431→1496; W12*=215→280; W13*=215→280; W14*=173→239; W16*=4374→4570; W2*=307→437; W3*=215→280; W4*=419→549; W6*=1358→1424; W7*=174→240 | M2 |
| gitattributes | default | W10*=1431→1496; W12*=215→280; W13*=215→280; W14*=173→239; W16*=4374→4570; W2*=307→437; W3*=215→280; W4*=419→549; W6*=1358→1424; W7*=174→240 | M2 |
| gitignore | candidate | W14*=5→6; W2*=9→10; W6*=18→19; W7*=6→7 | M2 |
| gitignore | default | W14*=5→6; W2*=9→10; W6*=18→19; W7*=6→7 | M2 |
| gleam | default | W1=2→1; W10=660→113; W11=2→1; W12=744→143; W13=744→143; W14=437→37; W15=10→5; W16=2617→570; W2=521→51; W3=744→143; W4=1279→239; W5=184→117; W6=437→37; W7=446→41 | M2 |
| glsl | default | W16*=4911→4967 | M2 |
| gn | default | W1=2→1; W10=475→104; W11=2→1; W12=475→104; W13=475→104; W14=208→6; W15=2→1; W16=1381→328; W2=292→10; W3=475→104; W4=755→160; W5=2→1; W6=208→6; W7=210→7 | M2 |
| go | default | W1=19→18; W10=6474→6197; W12=6918→6622; W13=6918→6622; W14=5967→5725; W15=578→550; W16=32375→31353; W2=6405→6141; W3=6918→6622; W4=13086→12534; W5=11193→10615; W6=6013→5769; W7*=2342→2384; W8=14636→13507 | M2 |
| godot_resource | default | W10*=325→333; W12*=321→329; W13*=321→329; W14*=362→370; W16*=1491→1667; W2*=494→503; W3*=321→329; W4*=706→722; W5=115→108; W6*=366→374; W7*=368→376 | M2 |
| graphql | default | W1=2→1; W10=588→294; W11=2→1; W12=586→293; W13=586→293; W14=336→83; W15=2→1; W16=2193→1808; W2=417→108; W3=586→293; W4=1127→562; W5=8→4; W6=336→83; W7=338→84 | M2 |
| hare | default | W1=2→1; W10=1042→527; W11=2→1; W12=1040→526; W13=1040→526; W14=631→145; W15=2→1; W16=4273→3318; W2=854→250; W3=1040→526; W4=2011→1015; W5=10→5; W6=631→145; W7=633→146 | M2 |
| heex | default | W1=2→1; W10=2→1; W11=2→1; W12=2→1; W13=2→1; W14=2→1; W15=2→1; W16=62→5; W2=6→3; W3=2→1; W4=2→1; W5=2→1; W6=2→1; W7=4→2 | M2 |
| http | candidate | W1=12→6; W10=3466→1733; W11=2→1; W12=3466→1733; W13=3466→1733; W14=2088→1044; W15=268→134; W16=9258→4562; W2=3262→1631; W3=3466→1733; W4=6678→3339; W5=5478→2739; W6=2088→1044; W7=342→171; W8=6218→3109; W9=24→12 | M2 |
| http | default | W1=12→6; W10=3466→1733; W11=2→1; W12=3466→1733; W13=3466→1733; W14=2088→1044; W15=268→134; W16=9177→4562; W2=3262→1631; W3=3466→1733; W4=6678→3339; W5=5478→2739; W6=2088→1044; W7=342→171; W8=6218→3109; W9=24→12 | M2 |
| hurl | default | W1=2→1; W10=1176→605; W11=2→1; W12=1176→605; W13=1176→605; W14=745→296; W15=2→1; W16=3971→2914; W2=1048→431; W3=1176→605; W4=2092→1063; W5=2→1; W6=745→296; W7=747→297 | M2 |
| hyprlang | default | W1=2→1; W10=146→54; W11=2→1; W12=198→77; W13=198→77; W14=93→3; W15=2→1; W16=714→449; W2=188→28; W3=198→77; W4=425→160; W5=6→3; W6=93→3; W7=116→5 | M2 |
| ini | default | W1=2→1; W10=50→13; W11=2→1; W12=64→14; W13=64→14; W14=89→14; W15=2→1; W16=274→77; W2=123→21; W3=64→14; W4=179→34; W5=12→6; W6=89→14; W7=103→20 | M2 |
| janet | default | W1=2→1; W10=262→36; W11=2→1; W12=260→35; W13=260→35; W14=255→7; W15=2→1; W16=1010→215; W2=405→12; W3=260→35; W4=546→74; W5=12→6; W6=255→7; W7=257→8 | M2 |
| java | default | W1=2→1; W10=42→16; W11=2→1; W12=40→15; W13=40→15; W14=34→8; W15=2→1; W16=428→101; W2=38→10; W3=40→15; W4=84→32; W5=6→3; W6=34→8; W7=36→9 | M2 |
| jq | default | W10*=669→671; W12*=271→273; W13*=271→273; W14*=358→360; W16*=36162→36171; W2*=228→230; W3*=271→273; W4*=663→671; W6*=836→838; W7*=296→298 | M2 |
| json | default | W1=2→1; W10=514→257; W11=2→1; W12=514→257; W13=514→257; W14=424→74; W15=2→1; W16=2077→1864; W2=494→98; W3=514→257; W4=1212→606; W5=2→1; W6=424→74; W7=426→75 | M2 |
| julia | candidate | W1=4→2; W10=6636→3414; W11=2→1; W12=6635→3413; W13=6635→3413; W14=3344→1705; W15=52→26; W16=19394→9747; W2=3560→1813; W3=6635→3413; W4=10144→5211; W5=9232→4616; W6=3344→1705; W7=1154→610; W8=4772→2386; W9=20→10 | M2 |
| julia | default | W1=4→2; W10=6636→3414; W11=2→1; W12=6635→3413; W13=6635→3413; W14=3344→1705; W15=52→26; W16=18793→9747; W2=3560→1813; W3=6635→3413; W4=10144→5211; W5=9232→4616; W6=3344→1705; W7=1154→610; W8=4772→2386; W9=20→10 | M2 |
| kconfig | candidate | W1=3→1; W10=556→101; W11=2→1; W12=606→110; W13=606→110; W14=528→48; W15=52→10; W16=2282→625; W2=658→56; W3=606→110; W4=1147→208; W5=935→142; W6=528→48; W7=199→40; W8=952→9; W9=8→0 | M2 |
| kconfig | default | W1=3→1; W10=556→101; W11=2→1; W12=606→110; W13=606→110; W14=528→48; W15=52→10; W16=2097→625; W2=658→56; W3=606→110; W4=1147→208; W5=935→142; W6=528→48; W7=199→40; W8=952→9; W9=8→0 | M2 |
| ledger | candidate | W10*=6883→7354; W12*=633→1104; W13*=633→1104; W14*=649→1160; W16*=4125→5772; W2*=1124→2006; W3*=633→1104; W4*=1311→2284; W5*=111→116; W6*=1069→1580; W7*=653→1164 | M2 |
| ledger | default | W10*=6883→7354; W12*=633→1104; W13*=633→1104; W14*=649→1160; W16*=3861→5508; W2*=1124→2006; W3*=633→1104; W4*=1311→2284; W5*=111→116; W6*=1069→1580; W7*=653→1164 | M2 |
| less | candidate | W16*=13826→13828; W2*=2883→2884 | M2 |
| less | default | W16*=13692→13694; W2*=2883→2884 | M2 |
| linkerscript | candidate | W16=31739→31738 | M2 |
| linkerscript | default | W16=31710→31709 | M2 |
| liquid | default | W1=2→1; W10=270→127; W11=2→1; W12=270→127; W13=270→127; W14=249→103; W15=6→3; W16=1195→574; W2=362→157; W3=270→127; W4=537→248; W5=201→101; W6=249→103; W7=159→58; W8=76→38 | M2 |
| llvm | default | W1=2→1; W10=2215→499; W11=2→1; W12=2213→498; W13=2213→498; W14=1419→171; W15=147→1; W16=8786→3223; W2=1779→227; W3=2213→498; W4=4160→897; W5=2410→3; W6=1419→171; W7=1276→172; W8=141→0 | M2 |
| make | candidate | W16=272594→272592 | M2 |
| make | default | W16=272531→272529 | M2 |
| meson | candidate | W10=4914→4663; W12=4914→4663; W13=4914→4663; W14=3554→3420; W15=952→895; W16=13943→13507; W2=4167→4018; W3=4914→4663; W4=9777→9255; W5=8646→8183; W6=3554→3420; W7=843→821; W8=7780→7278; W9=653→616 | M2 |
| meson | default | W10=4914→4663; W12=4914→4663; W13=4914→4663; W14=3554→3420; W15=952→895; W16=13860→13424; W2=4167→4018; W3=4914→4663; W4=9777→9255; W5=8646→8183; W6=3554→3420; W7=843→821; W8=7780→7278; W9=653→616 | M2 |
| move | candidate | W1=4→1; W10=3036→746; W11=2→1; W12=3096→771; W13=3096→771; W14=1469→334; W15=66→18; W16=10248→2933; W2=1752→406; W3=3096→771; W4=4875→1298; W5=4069→717; W6=1469→334; W7=499→197; W8=2335→176; W9=36→12 | M2 |
| move | default | W1=4→1; W10=3036→746; W11=2→1; W12=3096→771; W13=3096→771; W14=1469→334; W15=66→18; W16=10032→2933; W2=1752→406; W3=3096→771; W4=4875→1298; W5=4069→717; W6=1469→334; W7=499→197; W8=2335→176; W9=36→12 | M2 |
| ninja | default | W1=2→1; W10=338→35; W11=2→1; W12=336→34; W13=336→34; W14=371→3; W15=2→1; W16=1369→236; W2=498→6; W3=336→34; W4=734→67; W5=8→4; W6=371→3; W7=373→4 | M2 |
| objc | candidate | W1=3→1; W10=2105→718; W11=2→1; W12=2105→718; W13=2105→718; W14=1588→474; W15=132→52; W16=8497→3165; W2=1947→580; W3=2105→718; W4=3645→1229; W5=3633→1197; W6=1588→474; W7=566→206; W8=2389→360 | M2 |
| objc | default | W1=3→1; W10=2105→718; W11=2→1; W12=2105→718; W13=2105→718; W14=1588→474; W15=132→52; W16=8310→3165; W2=1947→580; W3=2105→718; W4=3645→1229; W5=3633→1197; W6=1588→474; W7=566→206; W8=2389→360 | M2 |
| odin | default | W1=2→1; W10=9→4; W11=2→1; W12=7→3; W13=7→3; W14=6→2; W15=2→1; W16=232→18; W2=10→4; W3=7→3; W4=10→4; W5=6→3; W6=6→2; W7=8→3 | M2 |
| pem | default | W1=2→1; W10=68→34; W11=2→1; W12=68→34; W13=68→34; W14=71→31; W15=2→1; W16=236→133; W2=131→61; W3=68→34; W4=146→73; W5=2→1; W6=71→31; W7=73→32 | M2 |
| pkl | default | W1=2→1; W10=183→144; W11=2→1; W12=212→164; W13=212→164; W14=140→70; W15=2→1; W16=830→725; W2=182→92; W3=212→164; W4=338→268; W5=28→14; W6=140→70; W7=166→83 | M2 |
| powershell | default | W10*=10536→11043; W12*=9986→10493; W13*=9986→10493; W14*=3754→3926; W16*=261289→262622; W2*=3939→4124; W3*=9986→10493; W4*=14302→14917; W5*=5306→5463; W6*=4414→4566; W7*=3512→3684; W8=1835→1693 | M2 |
| prolog | default | W14*=99→107; W16*=1102→1124; W2*=105→113; W6*=99→107; W7*=99→107 | M2 |
| puppet | default | W1=2→1; W10=448→224; W11=2→1; W12=450→225; W13=450→225; W14=227→82; W15=2→1; W16=1642→1139; W2=303→120; W3=450→225; W4=738→369; W5=2→1; W6=227→82; W7=229→83 | M2 |
| ql | default | W1=2→1; W10=345→54; W11=2→1; W12=345→54; W13=345→54; W14=206→25; W15=2→1; W16=1234→277; W2=229→28; W3=345→54; W4=565→91; W5=2→1; W6=206→25; W7=208→26 | M2 |
| racket | default | W1=2→1; W10=435→89; W11=2→1; W12=435→89; W13=435→89; W14=461→52; W15=2→1; W16=1622→425; W2=763→93; W3=435→89; W4=942→188; W6=461→52; W7=463→53 | M2 |
| regex | default | W1=2→1; W10=4033→2020; W11=2→1; W12=4033→2020; W13=4033→2020; W14=3745→1615; W15=27→1; W16=17556→12452; W2=3757→1621; W3=4033→2020; W4=8290→4144; W5=224→1; W6=3745→1615; W7=3722→1616 | M2 |
| requirements | candidate | W16=117675→117674 | M2 |
| requirements | default | W16=117663→117662 | M2 |
| robot | candidate | W1=2→1; W10=1180→634; W11=2→1; W12=1180→634; W13=1180→634; W14=875→450; W15=136→68; W16=7036→3605; W2=1261→643; W3=1180→634; W4=2338→1213; W5=2133→1099; W6=875→450; W7=619→322 | M2 |
| robot | default | W1=2→1; W10=1180→634; W11=2→1; W12=1180→634; W13=1180→634; W14=875→450; W15=136→68; W16=6619→3605; W2=1261→643; W3=1180→634; W4=2338→1213; W5=2133→1099; W6=875→450; W7=619→322 | M2 |
| ron | default | W14*=165→190; W16*=1481→1621; W2*=199→224; W6*=185→210; W7*=171→196 | M2 |
| smithy | default | W10*=1172→1242; W12*=1022→1094; W13*=1022→1094; W14*=463→543; W16*=4962→5292; W2*=611→705; W3*=1022→1094; W4*=1604→1693; W5*=193→202; W6*=687→767; W7*=467→547 | M2 |
| solidity | candidate | W1=4→2; W10=2293→1134; W11=2→1; W12=2289→1132; W13=2289→1132; W14=1217→584; W15=192→96; W16=6688→3139; W2=1328→637; W3=2289→1132; W4=3576→1760; W5=3656→1828; W6=1217→584; W7=323→137; W8=2184→1092; W9=156→78 | M2 |
| solidity | default | W1=4→2; W10=2293→1134; W11=2→1; W12=2289→1132; W13=2289→1132; W14=1217→584; W15=192→96; W16=6508→3139; W2=1328→637; W3=2289→1132; W4=3576→1760; W5=3656→1828; W6=1217→584; W7=323→137; W8=2184→1092; W9=156→78 | M2 |
| sparql | default | W10*=900→906; W12*=803→808; W13*=803→808; W14*=434→580; W15*=206→208; W16*=49886→50245; W2*=550→698; W3*=803→808; W4*=1568→1578; W5=1566→1416; W6*=1573→1726; W7*=961→1104; W8*=600→610 | M2 |
| ssh_config | default | W1=2→1; W10=172→86; W11=2→1; W12=172→86; W13=172→86; W14=132→47; W15=2→1; W16=1005→598; W2=261→93; W3=172→86; W4=342→171; W5=2→1; W6=132→47; W7=134→48 | M2 |
| tablegen | default | W1=2→1; W10=160→20; W11=2→1; W12=158→19; W13=158→19; W14=135→1; W15=2→1; W16=661→115; W2=157→4; W3=158→19; W4=298→32; W5=20→10; W6=135→1; W7=137→2 | M2 |
| textproto | default | W1=2→1; W10=1022→113; W11=2→1; W12=1267→118; W13=1267→118; W14=536→9; W15=2→1; W16=3560→424; W2=634→13; W3=1267→118; W4=1851→175; W5=2→1; W6=536→9; W7=538→10 | M2 |
| thrift | default | W1=2→1; W10=104→8; W11=2→1; W12=102→7; W13=102→7; W14=94→1; W15=2→1; W16=588→91; W2=107→3; W3=102→7; W4=209→10; W5=42→21; W6=94→1; W7=96→2 | M2 |
| todotxt | default | W1=2→1; W10=186→93; W11=2→1; W12=186→93; W13=186→93; W14=130→65; W15=2→1; W16=610→382; W2=230→115; W3=186→93; W4=314→157; W5=2→1; W6=130→65; W7=132→66 | M2 |
| toml | default | W1=2→1; W10=135→23; W11=2→1; W12=135→23; W13=135→23; W14=188→22; W15=2→1; W16=606→131; W2=240→33; W3=135→23; W4=350→60; W5=2→1; W6=188→22; W7=190→23 | M2 |
| turtle | default | W1=2→1; W10=678→331; W11=2→1; W12=678→331; W13=678→331; W14=382→107; W15=2→1; W16=2328→1669; W2=411→116; W3=678→331; W4=1203→582; W5=2→1; W6=382→107; W7=384→108 | M2 |
| twig | default | W1=2→1; W10=21→12; W11=2→1; W12=22→13; W13=22→13; W14=11→6; W15=2→1; W16=135→44; W2=20→9; W3=22→13; W4=33→19; W5=2→1; W6=11→6; W7=13→7 | M2 |
| typescript | default | W1=2→1; W10=645→137; W11=2→1; W12=648→137; W13=648→137; W14=393→51; W15=513→113; W16=2284→574; W2=934→167; W3=648→137; W4=1082→229; W5=967→231; W6=393→51; W7=387→52; W8=8→0 | M2 |
| v | candidate | W1=2→1; W10=2729→851; W11=2→1; W12=2790→871; W13=2790→871; W14=1097→281; W15=95→28; W16=13355→3659; W2=1545→428; W3=2790→871; W4=4003→1261; W5=1749→373; W6=1097→281; W7=885→230; W8=32→3 | M2 |
| v | default | W1=2→1; W10=2729→851; W11=2→1; W12=2790→871; W13=2790→871; W14=1097→281; W15=95→28; W16=10233→3659; W2=1545→428; W3=2790→871; W4=4003→1261; W5=1749→373; W6=1097→281; W7=885→230; W8=32→3 | M2 |
| vimdoc | default | W1=2→1; W10=660→532; W11=2→1; W12=903→733; W13=903→733; W14=380→255; W15=2→1; W16*=2824→3147; W2=792→520; W3=903→733; W4=1306→1053; W5=2→1; W6=380→255; W7=382→256 | M2 |
| wat | default | W14*=371→402; W16*=4050→4163; W2*=422→455; W4*=1213→1216; W5*=204→209; W6*=615→646; W7*=397→428 | M2 |
| yuck | default | W1=2→1; W10=674→32; W11=2→1; W12=674→32; W13=674→32; W14=388→5; W15=2→1; W16=2268→131; W2=676→11; W3=674→32; W4=1073→49; W6=388→5; W7=390→6 | M2 |
| zig | default | W1=2→1; W10=1488→314; W11=2→1; W12=1488→314; W13=1488→314; W14=727→103; W15=2→1; W16=4583→1387; W2=793→111; W3=1488→314; W4=2267→472; W5=2→1; W6=727→103; W7=729→104 | M2 |

## Pinned evidence

The [original qualification receipt](README.md), [expanded selected/adverse-operation table](counter-changes.md), and complete machine diffs remain unchanged. These hashes bind the proposed exception:

| Input | SHA-256 |
|---|---|
| runtime-source.patch.gz | `426450a34efc9a6666c38f4156ceddc7a3bc0885294bba24149efc6d170bf697` |
| source-go-sha256.json.gz | `f526c4f0c2c0a2ec2434517d0f056551ecd61443ca5b075962abc603b0226572` |
| selected-counter-audit.json.gz | `a9efac69cd8809c1eacd83bfa1fa8042482ed266ab9fa28ed33e55ce5ec2e4ed` |
| complete-work-diff.json.gz | `47fd90cf1f6ef41674d9bcd5cfdcf80ccea3b2eac6a3cee266fe30871fc974b7` |

Runtime source Go manifest SHA-256: `5893c5224310f600a2497b127e559a622f904ba699141e33196a01a9776b27cf`. No approval, ledger reset, runtime promotion or allowlist change is recorded by this proposal.
