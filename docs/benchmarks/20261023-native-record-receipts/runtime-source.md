# Canonical runtime snapshot

The runtime in the canonical checkout now matches the frozen `native-record-receipts-v2` source used by this bucket. The source-equivalence receipt records all Go files, module files, and the preceding PR commit. This moves the previously archived implementation into the existing draft branch; it does not promote it to main.

The source includes the ordinary-YAML ownership/checkpoint work, quiescent native publication, and removal of the duplicate raw-node buffer. Its evidence is in this directory and the preceding [native-publication qualification](../20261023-quiescent-native-publication/README.md). The page-reservation experiment is separate and is not included here.

The prior holds remain: the combined implementation has not passed its main-based throughput/allocation qualification; ordinary YAML throughput, warmed forced-legacy Go RSS, and the four exact Cognociente malformed-delete C cases remain unresolved. The completed buffer-removal comparison preserves every counter across all 206 languages. Native publication previously reduced 58 candidate-full node rows; all edit, reuse, and default rows stayed unchanged. These receipts do not authorize weakening a gate or graduating a language.

New architecture work proceeds from this pinned implementation in `/home/draco/work/gotreesitter`. The previous clean PR worktree remains detached at `57954e11a06588ce92833d489b48aced5f08d43d`; existing measurement sources and running processes are preserved.
