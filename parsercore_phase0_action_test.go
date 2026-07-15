//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"reflect"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestParserCoreActionConversionIsExhaustiveAndLossless(t *testing.T) {
	types := []struct {
		name string
		root ParseActionType
		core core.ActionType
	}{
		{name: "shift", root: ParseActionShift, core: core.ActionShift},
		{name: "reduce", root: ParseActionReduce, core: core.ActionReduce},
		{name: "accept", root: ParseActionAccept, core: core.ActionAccept},
		{name: "recover", root: ParseActionRecover, core: core.ActionRecover},
	}
	for _, test := range types {
		t.Run(test.name, func(t *testing.T) {
			input := ParseAction{
				Type: test.root, State: 1234, Symbol: 567, ChildCount: 8,
				DynamicPrecedence: -13, ProductionID: 901,
				Extra: true, ExtraChain: true, Repetition: true,
			}
			got, err := parserCoreAction(input)
			if err != nil {
				t.Fatal(err)
			}
			want := core.Action{
				Type: test.core, State: 1234, Symbol: 567, ChildCount: 8,
				DynamicPrecedence: -13, ProductionID: 901,
				Extra: true, ExtraChain: true, Repetition: true,
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("converted action = %+v, want %+v", got, want)
			}
		})
	}
	if _, err := parserCoreAction(ParseAction{Type: ParseActionType(255)}); err == nil {
		t.Fatal("invalid root action type was admitted")
	}
}

func TestParserCoreSameLookaheadGuardsDeclineBeforeMutation(t *testing.T) {
	tests := []struct {
		name    string
		token   Token
		actions []core.Action
	}{
		{name: "no-lookahead", token: Token{NoLookahead: true}, actions: []core.Action{{Type: core.ActionShift, State: 2}}},
		{name: "repetition", actions: []core.Action{{Type: core.ActionShift, State: 2, Repetition: true}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateDiagnosticParserCoreCell(test.token, test.actions)
			var decline *diagnosticParserCoreDecline
			if !errors.As(err, &decline) || decline.boundary != DiagnosticParserCoreRoute {
				t.Fatalf("guard error=%v, want unsupported-route decline", err)
			}
		})
	}
}
