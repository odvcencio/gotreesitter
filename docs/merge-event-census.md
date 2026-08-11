# Merge-event census receipt

The checked-in receipt records the stage M0 baseline.

Read [merge_event_census_receipt_v1.json](../cgo_harness/work_count/merge_event_census_receipt_v1.json) for the exact provenance and totals.

The receipt records three facts:

- The instrument and the constructed 104-source census passed.
- The `Foo[int](a)` discriminator measured packed links elected at the pop.
- The archived continuous integration run had no generated real-corpus files.

Do not treat zero real-corpus rows as a real-corpus result. Run the tagged
census with the authenticated corpus lock before starting M1.

The receipt also records that the current compact census does not expose an
explicit fold count. Link-union counters and compact acceptances are not that
count. Add that counter before claiming exact `M_k/M_c` accounting.
