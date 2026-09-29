//go:build !gts_no_parsercorephase0

package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// This file is the campaign-v7 B4d adversarial probe for Erlang's compact
// converged-split-drop artifact certificate (grammars/runtime_profiles.go,
// CompactConvergedReductionSplitDropsCertified).
//
// A completed corpus-dependence investigation found zero routing difference
// for Erlang with the certificate on versus off (2 of 3 real-corpus files
// route either way; the third never routes). The banked R2 lesson binds:
// corpus-zero dependence is a LEAD, not proof. This probe tries to break that
// lead before any grant is retired. It drives a batch of ambiguity-heavy
// Erlang sources -- macro replacement bodies, case/receive/try,
// comprehensions, records, binaries, guards, funs, maps, deep operator and
// cons chains -- through the real compact candidate route with the
// certificate off, then on, for comparison.
//
// The embedded loader caches and returns one shared *Language per language
// name, and Language carries several sync.Once fields, so a struct copy trips
// go vet's copylocks check (the existing struct-copy precedent for the
// inverse direction, parsercore_phase0_state_relex_test.go, only avoids this
// because it lives behind the opt-in gts_parsercorephase0 tag that plain `go
// vet ./...` never compiles). This file instead flips
// CompactConvergedReductionSplitDropsCertified on the shared cached language
// for the exact duration of one candidate-route parse, then restores it
// (helper below). Tests in this package run sequentially (no t.Parallel), so
// the mutation window never overlaps another test's execution.
//
// OUTCOME: the probe broke the lead. macro_expanded_top_level_function (and
// its multi-clause sibling macro_function_clauses) are genuine counterexamples:
// without the certificate, the compact route correctly declines with "converged
// -path reduction split no-action drop lacks one exact unselected-to-selected
// lineage" -- the mid-run gate in dropGenericNoActionHeads
// (parsercore_phase0_driver.go) -- because the dropped head's unselected rank
// has no matching selected survivor of the same lineage (an unprovable drop).
// The paired control below proves the certificate is exactly what makes that
// same source route WITH the certificate: it accepts via the artifact escape
// with ConvergedReductionSplitDrops=1, ConvergedCoverageDrops=0 (an accepted,
// unprovable drop). The trigger is any macro that expands to a top-level
// function definition -- even a single clause with no guard; a plain,
// non-macro function with the identical clauses routes cleanly. Macro-defined
// boilerplate functions are an ordinary Erlang pattern, not an exotic edge
// case, so this blocks B4d: the erlang grant in
// grammars/runtime_profiles.go is NOT retired. Every other source below
// routes and is byte-identical to production, so this finding is narrow but
// real.
//
// Per the work order's stop rule, the counterexample subtests report the
// finding with t.Skip (naming this campaign finding) rather than failing the
// suite, so this file stays green as a permanent regression witness: it pins
// both halves of the counterexample (declines without the certificate, routes
// only via the artifact escape with the certificate) so a future B4b
// converged-alternative-set proof can be checked against it.

// erlangConvergedSplitDecertify clears CompactConvergedReductionSplitDropsCertified
// on the shared, cached Erlang language and returns it alongside a restore
// function. Call restore before any other code in the same subtest depends on
// the certified value (for example the paired control parse below), and defer
// it too as a safety net against an unexpected panic mid-parse; restore is
// idempotent.
func erlangConvergedSplitDecertify(t *testing.T) (lang *gts.Language, restore func()) {
	t.Helper()
	lang = grammars.ErlangLanguage()
	if !lang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("shared erlang language does not carry the converged-split artifact certificate")
	}
	lang.CompactConvergedReductionSplitDropsCertified = false
	return lang, func() { lang.CompactConvergedReductionSplitDropsCertified = true }
}

type erlangConvergedSplitProbeSource struct {
	name   string
	source string
	// wantCounterexample marks a source this probe already knows declines
	// without the certificate and routes only via the artifact escape with it.
	// The subtest documents the finding (t.Skip) instead of failing.
	wantCounterexample bool
}

// erlangConvergedSplitProbeSources is deliberately biased toward Erlang
// constructs documented (grammars/erlang_native_ownership_test.go,
// grammargen/erlang_macro_canary_test.go) or structurally expected to produce
// grammar conflicts: macro replacement bodies (function clauses, case/receive
// clauses, guard wrappers, string concatenation, a bare top-level function);
// case/receive/try/catch; list and binary comprehensions; record and map
// construction/update/pattern; bit-syntax segments with guards; deep
// operator-precedence and cons chains; and multi-clause funs.
func erlangConvergedSplitProbeSources() []erlangConvergedSplitProbeSource {
	return []erlangConvergedSplitProbeSource{
		{
			name: "macro_function_clauses",
			source: "-module(m).\n" +
				"-define(FN, foo(0) -> zero; foo(N) when N > 0 -> pos; foo(_) -> neg).\n" +
				"?FN.\n",
			wantCounterexample: true,
		},
		{
			// Minimal reproduction: a single clause, no guard, no semicolon --
			// still declines. The trigger is macro-expansion into a top-level
			// function definition, not clause-ambiguity or guards.
			name: "macro_expanded_top_level_function",
			source: "-module(m).\n" +
				"-define(FN1, bar(1) -> one).\n" +
				"?FN1.\n",
			wantCounterexample: true,
		},
		{
			// Control: the identical clauses as macro_function_clauses, written
			// directly with no macro. Routes cleanly, isolating macro expansion
			// (not the clause shape itself) as the trigger.
			name: "plain_function_no_macro",
			source: "-module(m).\n" +
				"foo(0) -> zero;\n" +
				"foo(N) when N > 0 -> pos;\n" +
				"foo(_) -> neg.\n",
		},
		{
			name: "macro_cr_clauses",
			source: "-module(m).\n" +
				"-define(CR, [] -> empty; [_ | _] -> nonempty).\n" +
				"f(L) -> case L of ?CR end.\n",
		},
		{
			name: "macro_guard_wrapper",
			source: "-module(m).\n" +
				"-define(GUARD, X > 0 andalso Y < 10 orelse Z =:= ok).\n" +
				"f(X, Y, Z) when ?GUARD -> ok; f(_, _, _) -> no.\n",
		},
		{
			name: "macro_string_concat",
			source: "-module(m).\n" +
				"-define(TXT(Str), \"abc\" Str ??Str).\n" +
				"-define(TXT2(Str), Str \"abc\").\n" +
				"-define(TXT3(Str), ??Str \"abc\").\n",
		},
		{
			name: "case_nested_guards",
			source: "-module(m).\n" +
				"f(X, Y) ->\n" +
				"    case X of\n" +
				"        {ok, V} when V > 0 ->\n" +
				"            case Y of\n" +
				"                [A, B | _] when A =:= B -> match;\n" +
				"                [_ | _] -> tail;\n" +
				"                [] -> empty\n" +
				"            end;\n" +
				"        {ok, _} -> zero_or_neg;\n" +
				"        {error, Reason} when is_atom(Reason); is_list(Reason) -> Reason;\n" +
				"        _ -> unknown\n" +
				"    end.\n",
		},
		{
			name: "receive_after_multi",
			source: "-module(m).\n" +
				"loop(State) ->\n" +
				"    receive\n" +
				"        {From, ping} when is_pid(From) -> From ! pong, loop(State);\n" +
				"        {From, {get, K}} -> From ! maps:get(K, State, undefined), loop(State);\n" +
				"        stop -> ok\n" +
				"    after 5000 ->\n" +
				"        timeout\n" +
				"    end.\n",
		},
		{
			name: "try_catch_of_after",
			source: "-module(m).\n" +
				"f(X) ->\n" +
				"    try do_work(X) of\n" +
				"        {ok, V} -> V;\n" +
				"        {partial, V} when is_integer(V) -> V\n" +
				"    catch\n" +
				"        throw:{bad, Reason} -> {error, Reason};\n" +
				"        error:badarg:Stack -> {error, badarg, Stack};\n" +
				"        _:_ -> error\n" +
				"    after\n" +
				"        cleanup(X)\n" +
				"    end.\n",
		},
		{
			name: "list_comprehension_multi_qualifier",
			source: "-module(m).\n" +
				"f(Xs, Ys) ->\n" +
				"    [ {X, Y} || X <- Xs, Y <- Ys, X =/= Y, X > 0, is_integer(Y) ].\n" +
				"g(Xs) ->\n" +
				"    [ [Z || Z <- Row, Z > 0] || Row <- Xs ].\n",
		},
		{
			name: "binary_comprehension_segments",
			source: "-module(m).\n" +
				"f(Bin) ->\n" +
				"    << <<X:8, Y:8>> || <<X:8, Y:8>> <= Bin, X > Y >>.\n",
		},
		{
			name: "record_declare_construct_update_match",
			source: "-module(m).\n" +
				"-record(person, {name, age = 0 :: integer(), email = \"\"}).\n" +
				"f(#person{name = N, age = A} = P) when A >= 18 ->\n" +
				"    P#person{age = A + 1, email = N ++ \"@example.com\"};\n" +
				"f(P = #person{}) -> P.\n",
		},
		{
			name: "binary_pattern_guards",
			source: "-module(m).\n" +
				"parse(<<Len:16/unsigned-big-integer, Rest/binary>>) when Len =< byte_size(Rest) ->\n" +
				"    <<Payload:Len/binary, Tail/binary>> = Rest,\n" +
				"    {Payload, Tail};\n" +
				"parse(<<Tag:8, Value:32/float, _/binary>>) when Tag =:= 1 ->\n" +
				"    Value.\n",
		},
		{
			name: "operator_precedence_and_send_chain",
			source: "-module(m).\n" +
				"f(Pid, X, Y, Z) ->\n" +
				"    Pid ! X + Y * Z - 1 div 2 rem 3,\n" +
				"    (X band Y) bor (Z bxor X) bsl 1 bsr 1,\n" +
				"    X > 0 andalso (Y < 10 orelse Z =:= X) andalso not (X =:= Y).\n",
		},
		{
			name: "fun_multi_clause_guard_invoked",
			source: "-module(m).\n" +
				"f() ->\n" +
				"    G = fun\n" +
				"        (0) -> zero;\n" +
				"        (N) when N > 0, N < 10 -> small;\n" +
				"        (N) when N >= 10 -> big\n" +
				"    end,\n" +
				"    G(7).\n",
		},
		{
			name: "map_construct_update_pattern",
			source: "-module(m).\n" +
				"f(#{name := N, age := A} = M) when A > 0 ->\n" +
				"    M2 = M#{age => A + 1, active => true},\n" +
				"    #{n => N, m => M2}.\n",
		},
		{
			name: "guard_sequence_alternatives",
			source: "-module(m).\n" +
				"f(X, Y) when is_integer(X), X > 0; is_float(X), X > 0.0 ->\n" +
				"    pos;\n" +
				"f(X, Y) when (is_atom(Y) andalso Y =/= undefined); is_pid(Y) ->\n" +
				"    named;\n" +
				"f(_, _) -> other.\n",
		},
		{
			name: "list_cons_pattern_chain",
			source: "-module(m).\n" +
				"f([H1, H2 | [H3 | T]]) when H1 =:= H2 -> {dup, H3, T};\n" +
				"f([H | T]) -> {head, H, T};\n" +
				"f([]) -> empty.\n",
		},
		{
			name: "nested_case_fun_case",
			source: "-module(m).\n" +
				"f(X) ->\n" +
				"    Handler = fun(Y) ->\n" +
				"        case Y of\n" +
				"            {tag, V} when is_list(V) ->\n" +
				"                case V of\n" +
				"                    [] -> empty;\n" +
				"                    [_ | _] -> nonempty\n" +
				"                end;\n" +
				"            _ -> other\n" +
				"        end\n" +
				"    end,\n" +
				"    Handler(X).\n",
		},
		{
			name: "multi_clause_pattern_guard_fanout",
			source: "-module(m).\n" +
				"classify(0) -> zero;\n" +
				"classify(N) when N > 0, N < 10 -> single_digit;\n" +
				"classify(N) when N >= 10, N < 100 -> double_digit;\n" +
				"classify(N) when N < 0, N > -10 -> neg_single;\n" +
				"classify(N) when is_integer(N) -> other_int;\n" +
				"classify(N) when is_float(N) -> float_val;\n" +
				"classify(_) -> unknown.\n",
		},
	}
}

func TestAdmissionCandidateErlangConvergedSplitAdversarialProbe(t *testing.T) {
	sharedLang := grammars.ErlangLanguage()
	if !sharedLang.CompactConvergedReductionSplitDropsCertified {
		t.Fatal("shared erlang language does not carry the converged-split artifact certificate")
	}

	var (
		firedAndProved     int
		routedTotal        int
		counterexamplesHit int
	)

	for _, probe := range erlangConvergedSplitProbeSources() {
		probe := probe
		t.Run(probe.name, func(t *testing.T) {
			source := []byte(probe.source)

			production := gts.NewParser(sharedLang)
			production.SetAdmissionCandidateRoute(false)
			productionTree, err := production.Parse(source)
			if err != nil {
				t.Fatalf("production parse: %v", err)
			}
			defer productionTree.Release()
			if productionTree.RootNode().HasError() {
				t.Fatalf("production witness has a parse error; source is not valid Erlang:\n%s", productionTree.RootNode().SExpr(sharedLang))
			}

			decertifiedLang, restore := erlangConvergedSplitDecertify(t)
			defer restore() // safety net; the explicit call below covers the normal path
			gts.ResetAdmissionCandidateCountersForTest()
			candidate := gts.NewParser(decertifiedLang)
			candidate.SetAdmissionCandidateRoute(true)
			if err := gts.RetainAdmissionCandidateRunnerForTest(candidate); err != nil {
				t.Fatal(err)
			}
			candidateTree, err := candidate.Parse(source)
			restore() // restore immediately: later code in this subtest (the control parse) needs the certified value
			if err != nil {
				t.Fatalf("candidate parse: %v", err)
			}
			defer candidateTree.Release()

			routed, fallback := gts.AdmissionCandidateCounters()
			drops, proved, ok := gts.AdmissionCandidateConvergedSplitWorkForTest(candidate)

			switch {
			case routed == 1 && fallback == 0:
				if probe.wantCounterexample {
					t.Fatalf("expected source %q to decline without the certificate, but it routed (drops=%d proved=%d); the counterexample no longer reproduces -- re-verify before changing wantCounterexample", probe.name, drops, proved)
				}
				if !ok {
					t.Fatal("routed parse has no acceptance receipt")
				}
				if drops != proved {
					t.Fatalf("routed parse accepted with an unproved converged-split drop: drops=%d proved=%d", drops, proved)
				}
				routedTotal++
				if drops > 0 {
					firedAndProved++
					t.Logf("converged-split class fired and proved without the certificate: drops=%d proved=%d", drops, proved)
				} else {
					t.Logf("routed; converged-split class did not fire on this source")
				}

				candidateInspection, err := benchfixtures.InspectGoTree(candidateTree.RootNode(), decertifiedLang)
				if err != nil {
					t.Fatalf("inspect candidate tree: %v", err)
				}
				productionInspection, err := benchfixtures.InspectGoTree(productionTree.RootNode(), sharedLang)
				if err != nil {
					t.Fatalf("inspect production tree: %v", err)
				}
				if candidateInspection.SHA256 != productionInspection.SHA256 {
					t.Fatalf(
						"decertified candidate digest %s differs from production %s",
						candidateInspection.SHA256, productionInspection.SHA256,
					)
				}
			case fallback == 1 && routed == 0:
				reason := gts.AdmissionCandidateLastFallbackReason()
				if !strings.Contains(reason, "converged-path reduction split") {
					if probe.wantCounterexample {
						t.Fatalf("expected source %q to decline on a converged-path reduction split, got an unrelated decline: %s", probe.name, reason)
					}
					t.Skipf("candidate route declined for a reason unrelated to the converged-split certificate: %s", reason)
				}
				if !probe.wantCounterexample {
					t.Fatalf("COUNTEREXAMPLE: decertified route declined on a converged-path reduction split (source %q is not marked wantCounterexample): %s", probe.name, reason)
				}

				// Control: the same source, on the shared certified language,
				// must route -- and only via the artifact escape (an accepted,
				// UNPROVED drop) -- so the decline above is caused by the
				// certificate and not by an unrelated instability in this source.
				gts.ResetAdmissionCandidateCountersForTest()
				certifiedCandidate := gts.NewParser(sharedLang)
				certifiedCandidate.SetAdmissionCandidateRoute(true)
				if err := gts.RetainAdmissionCandidateRunnerForTest(certifiedCandidate); err != nil {
					t.Fatal(err)
				}
				certifiedTree, err := certifiedCandidate.Parse(source)
				if err != nil {
					t.Fatalf("certified control parse: %v", err)
				}
				defer certifiedTree.Release()
				certRouted, certFallback := gts.AdmissionCandidateCounters()
				certDrops, certProved, certOK := gts.AdmissionCandidateConvergedSplitWorkForTest(certifiedCandidate)
				if certRouted != 1 || certFallback != 0 || !certOK {
					t.Fatalf("control: certified route must accept this source: routed=%d fallback=%d ok=%v", certRouted, certFallback, certOK)
				}
				if certDrops == 0 || certDrops == certProved {
					t.Fatalf(
						"control: expected the certified route to accept via an UNPROVED converged-split drop (drops>0, proved<drops); got drops=%d proved=%d -- the certificate may no longer be load-bearing for this source",
						certDrops, certProved,
					)
				}

				counterexamplesHit++
				t.Skipf(
					"campaign-v7 B4d finding: erlang's CompactConvergedReductionSplitDropsCertified grant is load-bearing for this source. "+
						"Without the certificate: declined (%s). "+
						"With the certificate: routed only via the artifact escape with an unproved drop (ConvergedReductionSplitDrops=%d, ConvergedCoverageDrops=%d). "+
						"Erlang's corpus-zero dependence finding (hypha spore b4-provenance-families) is incomplete for macro-expanded top-level function definitions. "+
						"The grant in grammars/runtime_profiles.go is NOT retired; retirement is blocked pending a B4b converged-alternative-set proof.",
					reason, certDrops, certProved,
				)
			default:
				t.Fatalf("unexpected admission counters after one parse: routed=%d fallback=%d", routed, fallback)
			}
		})
	}

	wantCounterexamples := 0
	for _, probe := range erlangConvergedSplitProbeSources() {
		if probe.wantCounterexample {
			wantCounterexamples++
		}
	}
	if counterexamplesHit != wantCounterexamples {
		t.Fatalf("expected %d confirmed counterexample source(s), observed %d", wantCounterexamples, counterexamplesHit)
	}
	if counterexamplesHit == 0 && firedAndProved == 0 {
		t.Fatalf(
			"converged-reduction-split-drop class never fired across %d routed probe source(s) and no counterexample declined on it either; probe is vacuous",
			routedTotal,
		)
	}
	t.Logf(
		"probe summary: %d/%d routed source(s) fired-and-proved the converged-split class; %d confirmed counterexample(s) (decline without the certificate, route only via the artifact escape with it) -- see per-source SKIP output above",
		firedAndProved, routedTotal, counterexamplesHit,
	)
}
