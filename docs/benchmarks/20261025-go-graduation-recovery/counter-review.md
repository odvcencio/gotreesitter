# Go assignment recovery: counter review

Status: owner exception pending. This is the narrower missing-version protocol; the full version-order experiment is rejected. No ledger baseline is reset. The original Cognociente declaration fixture and standard Go R4 ledgers are exactly unchanged.

The new assignment witness is 138,608 bytes, SHA-256 `8dc8aad35490cffcb042bf3a79dc75d3fdd63be87fc92c638ea36351c57fce4d`. Delete the target at byte 57. Before: Go elects a missing-identifier assignment; after: Go matches C's skipped-equals ERROR. Independent fresh/incremental Go and C checks pass at 16 sites for each edit class and both routes (96 cases).

## Guarded/public-profile rows requiring owner review

| Row | Default before → after | Candidate before → after |
|---|---:|---:|
| Delete edit tokens | 3,572 → 3,832 | 3,585 → 3,850 |
| Delete edit new nodes | 7,210 → 8,652 | 7,175 → 8,641 |
| Delete edit maximum stacks | 2 → 4 | 2 → 4 |
| Full delete merge attempts | 15,840 → 16,527 | 15,840 → 16,527 |
| Edit delete merge attempts | 15,846 → 17,015 | 15,843 → 17,015 |

The merge rows are complete-operation work. The first three are public incremental-profile fields; they do not substitute for total work across full retries. Every changed public field, including GSS, scratch/arena storage and smaller reuse declines, is in [public-profile-changes.json](public-profile-changes.json). Reused bytes fall 135,028→134,736 and subtrees 1,769→1,763 on both routes; block splices fall 3,538→3,526 default and 3,536→3,522 candidate. These decreases remain below 2% but are retained as directional costs.

## Complete public-operation work

Fresh uses `Parse`; edit includes Tree.Edit, ParseIncrementalProfiled, and both releases. Producer construction happens outside the edit observation. Timing fields are excluded. Insert and replacement work is identical; declaration insert/replace/delete work is all identical. The table below lists every changed numeric event on the assignment delete.

| Counter | Default fresh | Default edit | Candidate fresh | Candidate edit |
|---|---:|---:|---:|---:|
| shifts | 211227 → 211764 | 214815 → 216026 | 211227 → 211764 | 214801 → 216024 |
| reductions | 221781 → 222333 | 225389 → 226637 | 221781 → 222333 | 225374 → 226630 |
| reduction_pop_requests | 221781 → 222333 | 225389 → 226637 | 221781 → 222333 | 225374 → 226630 |
| emitted_pop_paths | 221781 → 222333 | 225389 → 226637 | 221781 → 222333 | 225374 → 226630 |
| emitted_pop_payloads | 417165 → 418194 | 427891 → 430214 | 417165 → 418194 | 427863 → 430201 |
| table_lookups_proxy | 998562 → 1000809 | 1024179 → 1029376 | 998849 → 1001096 | 1024454 → 1029718 |
| action_entries_examined_proxy | 216504 → 217053 | 220092 → 221331 | 216504 → 217053 | 220077 → 221329 |
| stack_version_creations_proxy | 10581 → 10611 | 10599 → 10665 | 10581 → 10611 | 10597 → 10665 |
| merge_attempts_proxy | 15840 → 16527 | 15846 → 17015 | 15840 → 16527 | 15843 → 17015 |
| graph_link_additions_proxy | 353868 → 355086 | 354076 → 356692 | 353868 → 355086 | 354048 → 356690 |
| leaf_constructions_proxy | 211230 → 211767 | 214820 → 216031 | 211230 → 211767 | 214806 → 216029 |
| parent_constructions_proxy | 132027 → 132348 | 135627 → 136562 | 132027 → 132348 | 135612 → 136555 |
| lexer_front_door_calls_proxy | unchanged | 177836 → 178096 | unchanged | 177829 → 178094 |

The four merge-attempt increases are +4.34%, +7.38%, +4.34%, +7.40%; other complete-operation increases stay below 2%. The C-correct recovery needs both competing versions to receive their proper turn. The old cheaper result is structurally wrong. This is a correctness exception request, with no fresh-verifier removal or V4 activation.

[v1 design R3](../../v1-design.md#r3-counter-ledger-changes) requires explicit owner approval: “Agents do not approve ledger exceptions.” Earlier Liquid/YAML approvals do not cover these Go changes. The draft PR and all merges remain held pending approval and the separate graduation gates.
