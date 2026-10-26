# Every changed complete-operation counter

Baseline: pre-repair Go blob at `aa796ab8`. Candidate: repaired blob and matching executable identity pins. Ordinary insert/replace rows are identical. No instrumentation timing is used.

Fresh means public `Parse`; edit includes `Tree.Edit`, `ParseIncrementalProfiled` and both releases. Native old-tree creation is outside the edit observation. Each cell is before → after.

| Counter | Default fresh delete | Default edit delete | Candidate fresh delete | Candidate edit delete |
|---|---:|---:|---:|---:|
| shifts | 211218 → 211251 | 214800 → 214855 | 211218 → 211251 | 214784 → 214839 |
| reductions | 221769 → 221802 | 225369 → 225424 | 221769 → 221802 | 225356 → 225411 |
| reduction_pop_requests | 221769 → 221802 | 225369 → 225424 | 221769 → 221802 | 225356 → 225411 |
| emitted_pop_paths | 221769 → 221802 | 225369 → 225424 | 221769 → 221802 | 225356 → 225411 |
| emitted_pop_payloads | 417147 → 417183 | 427861 → 427921 | 417147 → 417183 | 427835 → 427895 |
| table_lookups_proxy | 998694 → 998841 | 1024399 → 1024644 | 998885 → 999307 | 1024581 → 1025101 |
| lexer_front_door_calls_proxy | 174264 → 174267 | 177836 → 177841 | 174264 → 174267 | 177827 → 177832 |
| stack_version_creations_proxy | 10569 → 10635 | 10579 → 10689 | 10569 → 10635 | 10577 → 10687 |
| graph_link_additions_proxy | 353838 → 353901 | 354030 → 354135 | 353838 → 353901 | 354004 → 354109 |
| leaf_constructions_proxy | 211224 → 211260 | 214810 → 214870 | 211224 → 211260 | 214794 → 214854 |
| parent_constructions_proxy | 132018 → 132027 | 135612 → 135643 | 132018 → 132027 | 135599 → 135630 |

All listed complete-operation increases are below 2%; the largest is stack-version creation in candidate delete edit, 10,577 → 10,687 (+1.04%). They pay for the colon/equals error-token path needed to match C. No reuse falls in the correctness-only comparison.

Full standard R4 ledger rows are identical, including route and decline reason. The separate [selected-profile changes](selected-profile-changes.json) retain diagnostic stack/GSS and scratch fields; these are not silently substituted for complete public-operation counts. Fresh public parses have no selected profile, represented as null in raw evidence.

The held reuse candidate has its own complete changes in [public-work-changes.json](public-work-changes.json). Those are not part of the runtime patch in this bucket.
