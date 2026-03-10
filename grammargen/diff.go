package grammargen

import (
	"fmt"
	"strings"
)

// GrammarDiff describes the differences between two grammar versions.
type GrammarDiff struct {
	AddedRules   []string
	RemovedRules []string
	ModifiedRules []string // rules present in both but with different definitions
	ExtrasChanged   bool
	ConflictsChanged bool
	ExternalsChanged bool
	WordChanged      bool
	SupertypesChanged bool
}

// HasChanges returns true if any differences were found.
func (d *GrammarDiff) HasChanges() bool {
	return len(d.AddedRules) > 0 || len(d.RemovedRules) > 0 || len(d.ModifiedRules) > 0 ||
		d.ExtrasChanged || d.ConflictsChanged || d.ExternalsChanged ||
		d.WordChanged || d.SupertypesChanged
}

// String returns a human-readable summary of the diff.
func (d *GrammarDiff) String() string {
	if !d.HasChanges() {
		return "no changes"
	}

	var b strings.Builder
	if len(d.AddedRules) > 0 {
		fmt.Fprintf(&b, "Added rules (%d): %s\n", len(d.AddedRules), strings.Join(d.AddedRules, ", "))
	}
	if len(d.RemovedRules) > 0 {
		fmt.Fprintf(&b, "Removed rules (%d): %s\n", len(d.RemovedRules), strings.Join(d.RemovedRules, ", "))
	}
	if len(d.ModifiedRules) > 0 {
		fmt.Fprintf(&b, "Modified rules (%d): %s\n", len(d.ModifiedRules), strings.Join(d.ModifiedRules, ", "))
	}
	if d.ExtrasChanged {
		b.WriteString("Extras: changed\n")
	}
	if d.ConflictsChanged {
		b.WriteString("Conflicts: changed\n")
	}
	if d.ExternalsChanged {
		b.WriteString("Externals: changed\n")
	}
	if d.WordChanged {
		b.WriteString("Word token: changed\n")
	}
	if d.SupertypesChanged {
		b.WriteString("Supertypes: changed\n")
	}
	return b.String()
}

// DiffGrammars compares two grammar versions and returns a diff.
func DiffGrammars(old, new *Grammar) *GrammarDiff {
	d := &GrammarDiff{}

	// Compare rules.
	oldRules := make(map[string]bool, len(old.RuleOrder))
	for _, name := range old.RuleOrder {
		oldRules[name] = true
	}
	newRules := make(map[string]bool, len(new.RuleOrder))
	for _, name := range new.RuleOrder {
		newRules[name] = true
	}

	for _, name := range new.RuleOrder {
		if !oldRules[name] {
			d.AddedRules = append(d.AddedRules, name)
		}
	}
	for _, name := range old.RuleOrder {
		if !newRules[name] {
			d.RemovedRules = append(d.RemovedRules, name)
		}
	}

	// Check for modified rules (present in both but different).
	for _, name := range new.RuleOrder {
		if !oldRules[name] {
			continue // added, not modified
		}
		oldRule := old.Rules[name]
		newRule := new.Rules[name]
		if !rulesEqual(oldRule, newRule) {
			d.ModifiedRules = append(d.ModifiedRules, name)
		}
	}

	// Compare extras.
	d.ExtrasChanged = !ruleSlicesEqual(old.Extras, new.Extras)

	// Compare conflicts.
	d.ConflictsChanged = !conflictsEqual(old.Conflicts, new.Conflicts)

	// Compare externals.
	d.ExternalsChanged = !ruleSlicesEqual(old.Externals, new.Externals)

	// Compare word.
	d.WordChanged = old.Word != new.Word

	// Compare supertypes.
	d.SupertypesChanged = !stringSlicesEqual(old.Supertypes, new.Supertypes)

	return d
}

// rulesEqual recursively compares two rule trees.
func rulesEqual(a, b *Rule) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind || a.Value != b.Value || a.Prec != b.Prec || a.Named != b.Named {
		return false
	}
	if len(a.Children) != len(b.Children) {
		return false
	}
	for i := range a.Children {
		if !rulesEqual(a.Children[i], b.Children[i]) {
			return false
		}
	}
	return true
}

// ruleSlicesEqual compares two slices of rules.
func ruleSlicesEqual(a, b []*Rule) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !rulesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// conflictsEqual compares two conflict declarations.
func conflictsEqual(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !stringSlicesEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// stringSlicesEqual compares two string slices.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
