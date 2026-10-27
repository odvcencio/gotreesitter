# Scoped W5 Go undo counter exception

The owner approved the specific Go `one_byte` undo step 1 `reused_subtrees` exception, 39 → 35, after reviewing [the original failed-row receipt](w5-go-undo-review.md). The raw strict comparison remains a failure. The approval does not change the global counter gate, the baseline, any other row, the absolute RSS hold, language graduation or the main-merge hold.

The gate remains strict by default. The draft PR's opt-in requires the exact approved fixture, revisions, tree and source evidence, and rejects any additional regression or altered result. The applied exception must be recorded in the output. The mechanism and its tests are the committed source; CI on that commit remains the final check.

Independent local tests ran 18 unit cases and replayed the original CI artifact: strict mode rejected the original row, the exact approved row passed with the opt-in, and changes to counts, other work or reuse, tree/source digests, fixture, step order, oracle and no-edit allocations were rejected. The [reviewed logs](w5-exception-tests.tar.gz) are diagnostic proof of the mechanism; they do not clear the ARM64 RSS hold or establish Windows performance.
