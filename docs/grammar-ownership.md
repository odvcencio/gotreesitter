# Leading grammar ownership policy

Status: proposed

Pilot: YAML

## Decision

gotreesitter will maintain leading versions of critical grammars when upstream
maintenance does not meet user needs.

Upstream acceptance will not block a gotreesitter fix or release. Upstream
remains a source of changes, history, tests, and compatibility evidence.

gotreesitter will preserve the Tree-sitter node interface unless a reviewed
language change requires a new interface.

## User promise

A user who selects gotreesitter receives more than a pure-Go parser runtime.
The built-in grammar set includes reviewed fixes for important languages.

For critical grammars, gotreesitter will:

- accept confirmed correctness reports;
- ship evidence-backed fixes without waiting for upstream;
- preserve stable node names, fields, and query behavior;
- track upstream changes without accepting them automatically;
- retire a local delta after upstream ships an equivalent fix.

This policy makes grammar maintenance a runtime advantage.

## Scope

This policy applies to grammars in the built-in registry. It does not require
gotreesitter to own every grammar in the registry.

The policy covers:

- grammar rules;
- external scanners;
- generated parse tables;
- node type metadata;
- highlight, tag, and injection queries;
- corpus fixtures and compatibility evidence.

## Priority tiers

The priority tier defines the maintenance promise. The maintenance class
defines source custody. These two labels are independent.

### Tier 1

Tier 1 contains the critical grammar set. Keep this set between 20 and 30
grammar artifacts until the evidence supports a larger set.

Tier 1 provides these commitments:

- accept direct gotreesitter correctness reports;
- investigate valid reports without waiting for upstream;
- ship a local correction after its evidence gates pass;
- maintain realistic corpus fixtures;
- protect node, field, and query compatibility;
- review upstream updates before adoption;
- provide a reproducible grammar artifact.

The initial Tier 1 contains 30 grammar artifacts:

- Bash;
- C;
- C++;
- C#;
- CSS;
- Elixir;
- Go;
- GraphQL;
- HCL;
- HTML;
- Java;
- JavaScript;
- JSON;
- Kotlin;
- Lua;
- Markdown;
- Nix;
- PHP;
- Python;
- Ruby;
- Rust;
- Scala;
- SQL;
- Swift;
- TOML;
- TSX;
- TypeScript;
- XML;
- YAML;
- Zig.

Review the Tier 1 set at each major release. Use adoption, defect volume,
consumer impact, and maintenance risk to change membership.

A new Tier 1 admission above the limit must replace an existing member or
receive an explicit capacity decision.

### Tier 2

Tier 2 contains the remaining built-in grammars. These grammars retain the
registry correctness and release safety gates.

Tier 2 does not carry the leading-maintenance promise. A strong user report
can promote a Tier 2 grammar to Tier 1.

## Maintenance classes

Each built-in grammar has one maintenance class. Tier 1 grammars can use any
class, but their maintenance promise authorizes immediate local corrections.

### Mirror

gotreesitter locks an upstream revision and generates a grammar blob from that
revision. The project can still fix runtime defects that affect this grammar.

Mirror is the default class.

### Lead

gotreesitter owns one or more reviewed changes above a locked upstream
revision. The local changes can include grammar rules, scanners, or queries.

The upstream revision remains reproducible. The local delta remains explicit
and ordered.

### Own

gotreesitter stores the canonical editable grammar source in its toolchain.
Grammargen compiles that source into the shipped blob.

The source can be:

- an imported and checked-in `grammar.json` file;
- generated Go domain-specific language source;
- reviewed Go domain-specific language source.

Upstream remains a comparison source for an owned grammar.

## Promotion rules

A confirmed defect can move a grammar from Mirror to Lead immediately. The
project does not require a waiting period.

Promote a grammar to Lead when one condition applies:

- a valid source file produces an incorrect tree;
- a parser crash, stop, or memory failure affects users;
- a consumer cannot ship because of a grammar defect;
- an external scanner needs a gotreesitter-specific correction;
- an upstream fix has no usable release;
- an upstream project does not respond within the required release window.

Promote a grammar from Lead to Own when all conditions apply:

- grammargen can regenerate the grammar deterministically;
- the owned source passes the required evidence gates;
- node and field compatibility remains classified;
- scanner ownership is complete when the grammar has external tokens;
- local maintenance costs less than repeated upstream patch management.

Repeated semantic deltas strongly support promotion to Own. One urgent defect
does not require a full source conversion.

## Source and provenance model

Every Lead or Own grammar must record:

- the upstream repository;
- the locked upstream revision;
- the upstream source license;
- each local change and its purpose;
- the relevant language specification section;
- upstream issues or pull requests, when they exist;
- the generator version and command;
- the source, blob, query, and node metadata hashes;
- the evidence receipt for the current artifact.

Keep every local grammar change as source. Do not edit a generated blob by
hand.

Apply Lead changes as ordered and pinned overlays. Regeneration must fail when
an overlay no longer applies cleanly.

## Change workflow

Use this workflow for a grammar correction:

1. Capture the smallest valid failing source.
2. Link the applicable language specification.
3. Record the current Go and C trees.
4. Fix the narrowest owned source layer.
5. Add the minimal fixture and a realistic consumer fixture.
6. Regenerate all affected artifacts.
7. Run the evidence gates.
8. Classify every changed tree.
9. Release after the gates pass.
10. Offer the fix upstream when that work has value.

An upstream pull request is an asynchronous contribution. Its state does not
control the gotreesitter release.

## Evidence gates

Every grammar change must pass these gates.

### Correctness

- The minimal regression fixture parses as specified.
- A realistic consumer fixture proves the reported outcome.
- The grammar's existing corpus does not gain an unexplained error.
- The parser does not crash, stop, or exceed its memory contract.

### Differential classification

- Compare the old artifact with the candidate artifact.
- Classify every changed acceptance result.
- Classify every changed tree shape.
- Reject unexplained changes.

### Tree-sitter compatibility

- Compare the candidate Go parser with a C parser built from matching sources.
- Require matching deep tree digests for unchanged semantics.
- Label each intentional difference and preserve its fixture.
- Compare node names, field names, and named-node status.

### Consumer surfaces

- Run applicable highlight queries.
- Run applicable tag and outline queries.
- Run injection queries when the grammar supports them.
- Check document boundaries and root spans.

### Supply chain

- Rebuild the blob twice and require identical hashes.
- Verify the source license and required notices.
- Verify external token order and scanner source hashes.
- Record the exact source and tool revisions.

### Performance

Measure parse time, allocation, and peak memory on affected fixtures. A
correctness fix can change performance, but the receipt must explain it.

Reject a change that breaks a parser memory contract or causes an unbounded
performance failure.

## Large language model policy

A large language model can help with:

- defect reduction;
- grammar translation;
- corpus generation;
- specification lookup;
- differential classification;
- candidate patch generation.

A large language model cannot provide acceptance authority. Reproducible tests
and reviewed evidence provide acceptance authority.

## Upstream reconciliation

Check upstream revisions as candidates. Do not replace a Lead or Own artifact
automatically.

When upstream contains an equivalent fix:

1. Build the old local source and the upstream candidate.
2. Run the same evidence gates against both artifacts.
3. Remove the local delta only when behavior is equivalent or better.
4. Retain a regression fixture after the local delta is removed.
5. Update the lock and provenance record together.

Keep the local behavior when the upstream candidate regresses a certified
fixture.

## Toolchain requirements

The implementation should add these durable surfaces:

- a manifest that records Mirror, Lead, or Own for each grammar;
- ordered patch declarations outside a language-name switch;
- one regeneration command for each Lead or Own grammar;
- update reports that preserve local deltas;
- continuous integration gates selected by maintenance class;
- registry metadata that exposes the grammar source and maintenance class;
- a receipt that binds source, generator, blob, queries, and tests.

`cmd/grammar_updater` should report upstream candidates. It must not replace a
Lead or Own source without review.

`cmd/grammar_update_guard` should continue to block unreviewed scanner changes.
It should also verify each declared local delta.

`cmd/ts2go` should apply manifest-declared overlays. It should not require a
new language-name condition for each overlay.

Grammargen should remain the final compiler for every Own grammar.

## Go ownership anchor

Go is Tier 1 and Own. Its existing grammargen source defines the first complete
ownership example.

Use Go to establish the required lifecycle:

- editable source in the Go domain-specific language;
- deterministic blob generation;
- registry source metadata;
- scanner attachment;
- focused and real-corpus tests;
- comparison with the locked C grammar;
- a documented regeneration command.

New Own grammars should meet the same lifecycle before promotion.

## YAML pilot

YAML is Tier 1. It will test the Lead path before a full Own conversion.

### Y0: ship the confirmed correction

- Keep the locked upstream revision.
- Apply the multiline single-quoted scalar correction in the Go scanner.
- Preserve the minimal fixture.
- Preserve the Kubernetes multi-document fixture.
- Require two documents and no parser error.

### Y1: establish leading provenance

- Classify YAML as Lead.
- Record the equivalent C scanner overlay for oracle builds.
- Link upstream issue 39 and pull requests 40 and 50.
- Record the gotreesitter fix and its evidence receipt.

### Y2: prove toolchain ownership

- Import the locked YAML `grammar.json` into grammargen.
- Emit a reviewable Go domain-specific language source.
- Rebuild the candidate blob deterministically.
- Compare the candidate with the locked and patched C grammar.
- Classify all node and field differences.

### Y3: promote only after proof

Promote YAML to Own only after Y2 passes every gate. Keep YAML as Lead if the
conversion changes unclassified semantics.

## Release rule

A Lead or Own grammar can ship when its local gates pass. Upstream review,
merge, and release states are not release gates.

The release notes must identify intentional behavior ahead of upstream.

## Non-goals

This policy does not:

- claim ownership of a language specification;
- require a fork of every upstream grammar;
- permit undocumented syntax extensions;
- permit silent node interface changes;
- replace tests with generated evidence summaries;
- require upstream agreement before a release.

## Initial deliverables

1. Add the priority-tier, maintenance-class, and provenance manifest.
2. Record Go as the Tier 1 Own reference.
3. Record the initial 30 Tier 1 grammar artifacts.
4. Generalize the pinned overlay mechanism.
5. Add tier-specific and class-specific continuous integration gates.
6. Complete YAML stages Y0 and Y1.
7. Run the YAML grammargen ownership experiment.
8. Publish the runtime advantage in the main documentation.

## Acceptance criteria

The first policy release is complete when:

- YAML carries an explicit Lead record;
- Go carries an explicit Tier 1 Own record;
- the manifest identifies all initial Tier 1 members;
- a clean checkout regenerates its shipped artifacts;
- the multiline single-quoted scalar fixtures pass;
- the realistic fixture retains both YAML documents;
- matching Go and patched-C sources produce compatible deep trees;
- every artifact has a reproducible provenance receipt;
- upstream state cannot block the release command.
