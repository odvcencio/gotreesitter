# C-reference atlas

This document records the G2 units from the parser graduation campaign.

## Unit G2.1

The event schema in `cgo_harness/work_count/reference_atlas_event_schema_v1.json`
names the events that the C and Go engines must expose.

The schema separates two states:

- `aggregate_only` means that counters exist without event order;
- `planned` means that the current engine exposes no event yet.

Do not treat either state as cross-engine event equality.

The contract excludes pointers, stack identifiers, and arena identifiers.
Use source position, parse state, symbol, call site, and event kind instead.

## Exit for G2.1

Require these conditions before adding emitters:

- every event has a stable identifier;
- both engines have an explicit status;
- aggregate mappings name existing counters;
- planned mappings name no counters;
- semantic keys exclude physical engine identity;
- the reference commit remains tree-sitter C 0.25.1.

## Unit G2.2

The Go diagnostic build now emits a bounded ordered stream for these events:

- action table lookup
- shift
- reduce
- accept
- recovery
- head merge

The stream uses source spans, parser state, grammar symbol, call site, event
kind, and outcome. It excludes pointers, stack identifiers, and arena data.

The stream is an observation surface. It does not change parser decisions.
Aggregate counters remain the comparison authority until the C stream exists.

## Next units

Add the ordered C stream in a separate pull request.
Use deterministic controls before reading a real corpus.
Align events by the schema key, not by final tree shape.
Reject a receipt when either engine drops or truncates an event stream.
