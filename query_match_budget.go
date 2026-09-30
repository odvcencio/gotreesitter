package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/queryexec"

// defaultQueryMatchWorkBudget bounds enumeration per pattern/node attempt.
// Nested alternatives share the same work and active-state bounds.
const defaultQueryMatchWorkBudget = 1_000_000

type queryMatchBudget = queryexec.Budget

func newQueryMatchBudget(limit int) *queryMatchBudget {
	return queryexec.NewBudget(limit)
}
