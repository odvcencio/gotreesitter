package tablebuild

import (
	"fmt"
	"sort"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// ChildType is node_types.rs: enum ChildType. Aliased false is
// ChildType::Normal(Symbol); Aliased true is ChildType::Aliased(Alias).
type ChildType struct {
	Aliased bool
	Symbol  Symbol // Normal only
	Alias   Alias  // Aliased only
}

// childTypeLess orders child types as node_types.rs: enum ChildType (derived
// Ord) does, except that an alias is ordered by its name where the generator
// orders it by the intern id of the name (see the package documentation:
// every reader uses the lists as sets).
func childTypeLess(a, b ChildType) bool {
	if a.Aliased != b.Aliased {
		return !a.Aliased
	}
	if !a.Aliased {
		return lexbuild.SymbolLess(a.Symbol, b.Symbol)
	}
	if a.Alias.Value != b.Alias.Value {
		return a.Alias.Value < b.Alias.Value
	}
	return !a.Alias.Named && b.Alias.Named
}

func childTypeEq(a, b ChildType) bool {
	if a.Aliased != b.Aliased {
		return false
	}
	if !a.Aliased {
		return a.Symbol == b.Symbol
	}
	return aliasEq(a.Alias, b.Alias)
}

// FieldInfo is node_types.rs: struct FieldInfo.
type FieldInfo struct {
	Quantity ChildQuantity
	Types    []ChildType
}

// defaultFieldInfo is FieldInfo::default(): the quantity is ChildQuantity::one().
func defaultFieldInfo() FieldInfo { return FieldInfo{Quantity: quantityOne()} }

func (f FieldInfo) clone() FieldInfo {
	return FieldInfo{Quantity: f.Quantity, Types: append([]ChildType(nil), f.Types...)}
}

// VariableInfo is node_types.rs: struct VariableInfo. The generator keeps the
// fields in a hash map; FieldNames lists the keys in name order.
type VariableInfo struct {
	Fields                 map[string]*FieldInfo
	Children               FieldInfo
	ChildrenWithoutFields  FieldInfo
	HasMultiStepProduction bool
}

// FieldNames returns the names of the fields in byte order.
func (v *VariableInfo) FieldNames() []string {
	out := make([]string, 0, len(v.Fields))
	for k := range v.Fields {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// defaultVariableInfo is VariableInfo::default().
func defaultVariableInfo() VariableInfo {
	return VariableInfo{Fields: map[string]*FieldInfo{}, Children: defaultFieldInfo(), ChildrenWithoutFields: defaultFieldInfo()}
}

func (v *VariableInfo) clone() VariableInfo {
	out := VariableInfo{
		Fields:                 make(map[string]*FieldInfo, len(v.Fields)),
		Children:               v.Children.clone(),
		ChildrenWithoutFields:  v.ChildrenWithoutFields.clone(),
		HasMultiStepProduction: v.HasMultiStepProduction,
	}
	for k, f := range v.Fields {
		c := f.clone()
		out.Fields[k] = &c
	}
	return out
}

// field is fields.entry(name).or_default().
func (v *VariableInfo) field(name string) *FieldInfo {
	f, ok := v.Fields[name]
	if !ok {
		d := defaultFieldInfo()
		f = &d
		v.Fields[name] = f
	}
	return f
}

// ChildQuantity is node_types.rs: struct ChildQuantity.
type ChildQuantity struct {
	Exists, Required, Multiple bool
}

// quantityZero is node_types.rs: fn ChildQuantity::zero.
func quantityZero() ChildQuantity { return ChildQuantity{} }

// quantityOne is node_types.rs: fn ChildQuantity::one.
func quantityOne() ChildQuantity { return ChildQuantity{Exists: true, Required: true} }

// append is node_types.rs: fn ChildQuantity::append.
func (q *ChildQuantity) append(other ChildQuantity) {
	if other.Exists {
		if q.Exists || other.Multiple {
			q.Multiple = true
		}
		if other.Required {
			q.Required = true
		}
		q.Exists = true
	}
}

// union is node_types.rs: fn ChildQuantity::union. It reports a change.
func (q *ChildQuantity) union(other ChildQuantity) bool {
	result := false
	if !q.Exists && other.Exists {
		result = true
		q.Exists = true
	}
	if q.Required && !other.Required {
		result = true
		q.Required = false
	}
	if !q.Multiple && other.Multiple {
		result = true
		q.Multiple = true
	}
	return result
}

// VariableInfoError is node_types.rs: enum VariableInfoError.
type VariableInfoError struct {
	// SupertypeAlias is set for VariableInfoError::SupertypeAliasCollision.
	SupertypeAlias string
	// Supertype and Child (if HasChild) are set for
	// VariableInfoError::InvalidSupertype (struct InvalidSupertypeError).
	Supertype string
	Child     string
	HasChild  bool
}

func (e *VariableInfoError) Error() string {
	if e.SupertypeAlias != "" {
		return fmt.Sprintf("Named alias `%s` conflicts with a supertype of the same name.", e.SupertypeAlias)
	}
	s := fmt.Sprintf("Supertypes must have a single visible child, but `%s` can have multiple.", e.Supertype)
	if e.HasChild {
		s += fmt.Sprintf(" The hidden child `%s` can expand into multiple nodes. Consider making `%s` visible.", e.Child, e.Child)
	}
	return s
}

// GetVariableInfo is node_types.rs: fn get_variable_info. It summarizes, for
// each variable, the visible children and fields that its nodes can have.
// Only the productions of the variables are read (sg.VarProds), not the
// productions that inlining appended.
func GetVariableInfo(sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar, defaultAliases AliasMap) ([]VariableInfo, error) {
	if err := validateSupertypeAliases(sg, defaultAliases); err != nil {
		return nil, err
	}
	result := computeVariableInfoFixedPoint(sg, lg, defaultAliases)
	if err := validateSupertypeStructure(result, sg, lg, defaultAliases); err != nil {
		return nil, err
	}
	stripHiddenChildTypes(result, sg, lg)
	return result, nil
}

// validateSupertypeAliases is node_types.rs: fn validate_supertype_aliases.
func validateSupertypeAliases(sg *lexbuild.SyntaxGrammar, defaultAliases AliasMap) error {
	aliasesBySymbol := getAliasesBySymbol(sg, defaultAliases)
	for _, st := range sg.SupertypeSymbols {
		name := sg.Variables[st.Index].Ident
		collision := Alias{Set: true, Value: name, Named: true}
		// Any symbol's set: the walk order cannot change whether one holds it.
		for _, set := range aliasesBySymbol {
			if set.contains(collision) {
				return &VariableInfoError{SupertypeAlias: name}
			}
		}
	}
	return nil
}

// childType is grammars.rs: fn ProductionStep::child_type.
func childType(step *lexbuild.ProductionStep, defaultAliases AliasMap) ChildType {
	if step.Alias.Set {
		return ChildType{Aliased: true, Alias: step.Alias}
	}
	if a, ok := defaultAliases.get(step.Symbol); ok {
		return ChildType{Aliased: true, Alias: a}
	}
	return ChildType{Symbol: step.Symbol}
}

// computeVariableInfoFixedPoint is node_types.rs: fn compute_variable_info_fixed_point.
func computeVariableInfoFixedPoint(sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar, defaultAliases AliasMap) []VariableInfo {
	visible := func(t ChildType) bool { return variableTypeForChildType(t, sg, lg) >= lexbuild.VarAnonymous }
	named := func(t ChildType) bool { return variableTypeForChildType(t, sg, lg) == lexbuild.VarNamed }

	didChange := true
	allInitialized := false
	result := make([]VariableInfo, len(sg.Variables))
	for i := range result {
		result[i] = defaultVariableInfo()
	}
	for didChange {
		didChange = false

		for i := range sg.Variables {
			variableInfo := result[i].clone()

			// The child types of a variable combine across its productions at once;
			// the child quantities are recorded for each production on its own.
			vp := sg.VarProds[i]
			for prodID := vp[0]; prodID < vp[1]; prodID++ {
				production := &sg.Productions[prodID]
				// The generator keeps a hash map; each entry is read and written on
				// its own, so the order of the map reaches no result.
				productionFieldQuantities := map[string]*ChildQuantity{}
				fieldQuantity := func(name string) *ChildQuantity {
					q, ok := productionFieldQuantities[name]
					if !ok {
						z := quantityZero()
						q = &z
						productionFieldQuantities[name] = q
					}
					return q
				}
				productionChildrenQuantity := quantityZero()
				productionChildrenWithoutFieldsQuantity := quantityZero()
				productionHasUninitializedInvisibleChildren := false

				if len(production.Steps) > 1 {
					variableInfo.HasMultiStepProduction = true
				}

				for si := range production.Steps {
					step := &production.Steps[si]
					childSymbol := step.Symbol
					ct := childType(step, defaultAliases)

					childIsHidden := !visible(ct) && !containsSymbol(sg.SupertypeSymbols, childSymbol)

					// The set of all child types of this variable, and the quantity of
					// visible children in this production.
					if extendSorted(&variableInfo.Children.Types, ct) {
						didChange = true
					}
					if !childIsHidden {
						productionChildrenQuantity.append(quantityOne())
					}

					// The child types of each field, and the quantity of children of
					// each field in this production.
					if step.HasField {
						fieldInfo := variableInfo.field(step.Field)
						if extendSorted(&fieldInfo.Types, ct) {
							didChange = true
						}
						pfq := fieldQuantity(step.Field)

						// Inherit the types and quantities of hidden children associated
						// with fields.
						if childIsHidden && childSymbol.IsNonTerminal() {
							childInfo := &result[childSymbol.Index]
							if extendSorted(&fieldInfo.Types, childInfo.Children.Types...) {
								didChange = true
							}
							pfq.append(childInfo.Children.Quantity)
						} else {
							pfq.append(quantityOne())
						}
					} else if named(ct) {
						// The named children without fields.
						productionChildrenWithoutFieldsQuantity.append(quantityOne())
						if extendSorted(&variableInfo.ChildrenWithoutFields.Types, ct) {
							didChange = true
						}
					}

					// Inherit all child information from hidden children.
					if childIsHidden && childSymbol.IsNonTerminal() {
						if inheritHiddenChildInfo(&result[childSymbol.Index], !step.HasField, &variableInfo, fieldQuantity, &productionChildrenQuantity, &productionChildrenWithoutFieldsQuantity) {
							didChange = true
						}
					}

					// Whether this production holds children whose summaries are not
					// computed yet. The generator compares the index of any symbol,
					// terminals too.
					if int(childSymbol.Index) >= i && !allInitialized {
						productionHasUninitializedInvisibleChildren = true
					}
				}

				// If all of this production's children have summaries, add the
				// quantities that this production allows.
				if !productionHasUninitializedInvisibleChildren {
					if variableInfo.Children.Quantity.union(productionChildrenQuantity) {
						didChange = true
					}
					if variableInfo.ChildrenWithoutFields.Quantity.union(productionChildrenWithoutFieldsQuantity) {
						didChange = true
					}
					// Each field is updated on its own: the order of the map reaches
					// no result.
					for name, info := range variableInfo.Fields {
						q := quantityZero()
						if pq, ok := productionFieldQuantities[name]; ok {
							q = *pq
						}
						if info.Quantity.union(q) {
							didChange = true
						}
					}
				}
			}

			result[i] = variableInfo
		}

		allInitialized = true
	}

	return result
}

// inheritHiddenChildInfo is node_types.rs: fn inherit_hidden_child_info. It
// carries the fields, children and children without fields of a hidden child
// variable into the parent, and reports a change.
func inheritHiddenChildInfo(child *VariableInfo, stepHasNoField bool, variableInfo *VariableInfo, fieldQuantity func(string) *ChildQuantity, productionChildrenQuantity, productionChildrenWithoutFieldsQuantity *ChildQuantity) bool {
	didChange := false

	// If a hidden child can have multiple children, its parent can appear to
	// have multiple children.
	if child.HasMultiStepProduction {
		variableInfo.HasMultiStepProduction = true
	}

	// If a hidden child has fields, the parent can appear to have them. Each
	// field is handled on its own, so the order of the map reaches no result.
	for name, childField := range child.Fields {
		fieldQuantity(name).append(childField.Quantity)
		if extendSorted(&variableInfo.field(name).Types, childField.Types...) {
			didChange = true
		}
	}

	// If a hidden child has children, the parent can appear to have them.
	productionChildrenQuantity.append(child.Children.Quantity)
	if extendSorted(&variableInfo.Children.Types, child.Children.Types...) {
		didChange = true
	}

	// If a hidden child can have named children without fields, the parent can
	// appear to have them.
	if stepHasNoField {
		grandchildren := &child.ChildrenWithoutFields
		if len(grandchildren.Types) > 0 {
			productionChildrenWithoutFieldsQuantity.append(child.ChildrenWithoutFields.Quantity)
			if extendSorted(&variableInfo.ChildrenWithoutFields.Types, child.ChildrenWithoutFields.Types...) {
				didChange = true
			}
		}
	}

	return didChange
}

// validateSupertypeStructure is node_types.rs: fn validate_supertype_structure.
// A supertype with a multi-step production can have more than one visible child.
func validateSupertypeStructure(result []VariableInfo, sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar, defaultAliases AliasMap) error {
	visible := func(t ChildType) bool { return variableTypeForChildType(t, sg, lg) >= lexbuild.VarAnonymous }
	for _, st := range sg.SupertypeSymbols {
		if !result[st.Index].HasMultiStepProduction {
			continue
		}
		variable := &sg.Variables[st.Index]
		// A symbol can have a multi-step production directly or through a hidden
		// child; in the second case the error names the child.
		e := &VariableInfoError{Supertype: variable.Ident}
		vp := sg.VarProds[st.Index]
		for prodID := vp[0]; prodID < vp[1]; prodID++ {
			p := &sg.Productions[prodID]
			if len(p.Steps) != 1 {
				continue
			}
			step := &p.Steps[0]
			childSymbol := step.Symbol
			ct := childType(step, defaultAliases)
			childIsHidden := !visible(ct) && !containsSymbol(sg.SupertypeSymbols, childSymbol)
			if childIsHidden && childSymbol.IsNonTerminal() && result[childSymbol.Index].HasMultiStepProduction {
				e.Child, e.HasChild = sg.Variables[childSymbol.Index].Ident, true
				break
			}
		}
		return e
	}
	return nil
}

// stripHiddenChildTypes is node_types.rs: fn strip_hidden_child_types. It
// removes hidden child types from the children of supertypes, from the fields
// and from the children without fields, and drops the fields left empty.
func stripHiddenChildTypes(result []VariableInfo, sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar) {
	visible := func(t ChildType) bool { return variableTypeForChildType(t, sg, lg) >= lexbuild.VarAnonymous }
	retain := func(types []ChildType) []ChildType {
		out := types[:0]
		for _, t := range types {
			if visible(t) {
				out = append(out, t)
			}
		}
		return out
	}
	for _, st := range sg.SupertypeSymbols {
		result[st.Index].Children.Types = retain(result[st.Index].Children.Types)
	}
	for i := range result {
		info := &result[i]
		for name, f := range info.Fields {
			f.Types = retain(f.Types)
			if len(f.Types) == 0 {
				delete(info.Fields, name)
			}
		}
		info.ChildrenWithoutFields.Types = retain(info.ChildrenWithoutFields.Types)
	}
}

// aliasSet is the BTreeSet<Option<Alias>> of get_aliases_by_symbol, as a list
// without repeats. An entry with Set false is None.
type aliasSet []Alias

func (s aliasSet) contains(a Alias) bool {
	for _, x := range s {
		if aliasEq(x, a) {
			return true
		}
	}
	return false
}

func (s *aliasSet) insert(a Alias) {
	if !s.contains(a) {
		if !a.Set {
			a = Alias{}
		}
		*s = append(*s, a)
	}
}

// getAliasesBySymbol is node_types.rs: fn get_aliases_by_symbol: each symbol
// with the aliases it appears under (an absent alias for an unaliased
// appearance). The generator returns a hash map of ordered sets. The callers
// of this package only ask whether a set holds an alias, so the map and set
// orders reach no result; Sorted gives the order of the generator's sets
// (absent first) for callers that walk a set.
func getAliasesBySymbol(sg *lexbuild.SyntaxGrammar, defaultAliases AliasMap) map[Symbol]aliasSet {
	out := map[Symbol]aliasSet{}
	for s, a := range defaultAliases {
		out[s] = aliasSet{a}
	}
	for _, extra := range sg.ExtraSymbols {
		if _, ok := defaultAliases[extra]; !ok {
			set := out[extra]
			set.insert(Alias{})
			out[extra] = set
		}
	}
	for i := range sg.Variables {
		vp := sg.VarProds[i]
		for prodID := vp[0]; prodID < vp[1]; prodID++ {
			for _, step := range sg.Productions[prodID].Steps {
				a := step.Alias
				if !a.Set {
					if d, ok := defaultAliases.get(step.Symbol); ok {
						a = d
					}
				}
				set := out[step.Symbol]
				set.insert(a)
				out[step.Symbol] = set
			}
		}
	}
	out[lexbuild.NonTerminalSym(0)] = aliasSet{Alias{}}
	return out
}

// Sorted returns the set in the order of BTreeSet<Option<Alias>>, except that
// aliases are ordered by name where the generator orders them by intern id.
func (s aliasSet) Sorted() []Alias {
	out := append([]Alias(nil), s...)
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Set != b.Set {
			return !a.Set
		}
		if a.Value != b.Value {
			return a.Value < b.Value
		}
		return !a.Named && b.Named
	})
	return out
}

// SupertypeSubtypes is one entry of the map that get_supertype_symbol_map
// returns: a supertype and the child types of its variable.
type SupertypeSubtypes struct {
	Supertype Symbol
	Subtypes  []ChildType
}

// GetSupertypeSymbolMap is node_types.rs: fn get_supertype_symbol_map. The
// generator returns a BTreeMap keyed by symbol; the entries here are in that
// order (supertypes are non-terminals, so the order is the variable order).
// The generator also builds a map from alias to symbols that it never reads;
// it is not built here.
func GetSupertypeSymbolMap(sg *lexbuild.SyntaxGrammar, defaultAliases AliasMap, info []VariableInfo) []SupertypeSubtypes {
	var out []SupertypeSubtypes
	for i := range info {
		symbol := lexbuild.NonTerminalSym(i)
		if containsSymbol(sg.SupertypeSymbols, symbol) {
			out = append(out, SupertypeSubtypes{symbol, append([]ChildType(nil), info[i].Children.Types...)})
		}
	}
	return out
}

// variableTypeForChildType is node_types.rs: fn variable_type_for_child_type.
// An alias decides first; then a supertype is named and an inlined rule is
// hidden; else the symbol's own kind.
func variableTypeForChildType(t ChildType, sg *lexbuild.SyntaxGrammar, lg *lexbuild.LexicalGrammar) lexbuild.VariableType {
	if t.Aliased {
		if t.Alias.Named {
			return lexbuild.VarNamed
		}
		return lexbuild.VarAnonymous
	}
	s := t.Symbol
	if containsSymbol(sg.SupertypeSymbols, s) {
		return lexbuild.VarNamed
	}
	if containsSymbol(sg.VariablesToInline, s) {
		return lexbuild.VarHidden
	}
	switch s.Kind {
	case lexbuild.NonTerminal:
		return sg.Variables[s.Index].Kind
	case lexbuild.Terminal:
		return lg.Variables[s.Index].Kind
	case lexbuild.External:
		return sg.ExternalTokens[s.Index].Kind
	}
	return lexbuild.VarHidden
}

// extendSorted is node_types.rs: fn extend_sorted. It inserts each value that
// the sorted list does not hold at its sorted place, and reports an insertion.
func extendSorted(list *[]ChildType, values ...ChildType) bool {
	changed := false
	for _, v := range values {
		i := sort.Search(len(*list), func(k int) bool { return !childTypeLess((*list)[k], v) })
		if i < len(*list) && childTypeEq((*list)[i], v) {
			continue
		}
		*list = append(*list, ChildType{})
		copy((*list)[i+1:], (*list)[i:])
		(*list)[i] = v
		changed = true
	}
	return changed
}
