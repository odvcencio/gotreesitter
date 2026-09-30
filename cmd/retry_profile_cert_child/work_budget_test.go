//go:build gts_workcount

package main

import (
	gotreesitter "github.com/odvcencio/gotreesitter"
	"testing"
)

func TestWorkBudgetPairConfiguration(t *testing.T) {
	baseline, candidate := &gotreesitter.Language{}, &gotreesitter.Language{}
	if err := configureLanguages(modeWorkBudget, baseline, candidate); err != nil {
		t.Fatal(err)
	}
	if baseline.FullParseRetryWorkBudgetEnabled || !candidate.FullParseRetryWorkBudgetEnabled {
		t.Fatal("work policy was not isolated")
	}
}
