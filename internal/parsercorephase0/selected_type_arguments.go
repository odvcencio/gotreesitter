package parsercorephase0

// SelectedTypeArgumentPolicy describes the existing public builtin-call
// compatibility rule using authenticated symbol and field identities. The
// indexed publisher applies it without allocating public node headers.
type SelectedTypeArgumentPolicy struct {
	Call, Arguments, Identifier, TypeIdentifier                                                        Symbol
	Comment, TypeArguments                                                                             Symbol
	Selector, Unary, Qualified, Pointer, PackageIdentifier                                             Symbol
	Parenthesized, ParenthesizedType                                                                   Symbol
	FunctionField, ArgumentsField, OperandField, OperatorField, SelectorField, PackageField, NameField FieldID
}

func (s *SelectedStore) normalizeTypeArgument(id SelectedNodeID, p SelectedTypeArgumentPolicy, symbols []SelectedSymbolPolicy, source []byte, poll func() error) error {
	if p.Call == 0 || p.Arguments == 0 || p.Identifier == 0 || p.TypeIdentifier == 0 {
		return nil
	}
	childField := func(id SelectedNodeID, field FieldID) SelectedNodeID {
		if id == 0 || field == 0 {
			return 0
		}
		r := s.records[id-1]
		for _, c := range s.children[r.FirstChild : r.FirstChild+uint32(r.ChildCount)] {
			if s.records[c-1].Field == field {
				return c
			}
		}
		return 0
	}
	firstNamed := func(id SelectedNodeID) SelectedNodeID {
		r := s.records[id-1]
		for _, c := range s.children[r.FirstChild : r.FirstChild+uint32(r.ChildCount)] {
			n := s.records[c-1]
			if n.Named() && (p.Comment == 0 || n.Symbol != p.Comment) {
				return c
			}
		}
		return 0
	}
	textIs := func(id SelectedNodeID, expected string) bool {
		if id == 0 {
			return false
		}
		r := s.records[id-1]
		if r.EndByte < r.StartByte || uint64(r.EndByte) > uint64(len(source)) {
			return false
		}
		// Comparing in place avoids allocating a token-name string for every
		// call expression examined by the compatibility pass.
		return string(source[r.StartByte:r.EndByte]) == expected
	}
	structured := p.Selector != 0 && p.Unary != 0 && p.Qualified != 0 && p.Pointer != 0 && p.PackageIdentifier != 0 && p.OperandField != 0 && p.SelectorField != 0 && p.PackageField != 0 && p.NameField != 0
	canType := func(id SelectedNodeID) (bool, error) {
		// Unary and parenthesized forms have one successor. Iterate instead of
		// consuming the Go call stack on deeply nested input.
		for steps := 0; id != 0 && steps <= len(s.records); steps++ {
			if steps&255 == 0 {
				if err := poll(); err != nil {
					return false, err
				}
			}
			r := s.records[id-1]
			if r.Symbol == p.Identifier {
				return true, nil
			}
			if !structured {
				return false, nil
			}
			switch r.Symbol {
			case p.Selector:
				c := childField(id, p.OperandField)
				return c != 0 && s.records[c-1].Symbol == p.Identifier, nil
			case p.Unary:
				if !textIs(childField(id, p.OperatorField), "*") {
					return false, nil
				}
				id = childField(id, p.OperandField)
			case p.Parenthesized:
				if p.Parenthesized == 0 || p.ParenthesizedType == 0 {
					return false, nil
				}
				id = firstNamed(id)
			default:
				return false, nil
			}
		}
		return false, nil
	}
	retag := func(id SelectedNodeID, symbol Symbol) {
		r := &s.records[id-1]
		r.Symbol = symbol
		r.flags &^= selectedNodeFlagNamed
		if int(symbol) < len(symbols) && symbols[symbol].Named {
			r.flags |= selectedNodeFlagNamed
		}
	}
	r := s.records[id-1]
	if r.Symbol != p.Call {
		return nil
	}
	fn := childField(id, p.FunctionField)
	if fn == 0 || s.records[fn-1].Symbol != p.Identifier {
		return nil
	}
	if !textIs(fn, "new") && !textIs(fn, "make") {
		return nil
	}
	generic := false
	for _, c := range s.children[r.FirstChild : r.FirstChild+uint32(r.ChildCount)] {
		if p.TypeArguments != 0 && s.records[c-1].Symbol == p.TypeArguments && s.records[c-1].Named() {
			generic = true
			break
		}
	}
	if generic {
		return nil
	}
	args := childField(id, p.ArgumentsField)
	if args == 0 || s.records[args-1].Symbol != p.Arguments {
		return nil
	}
	first := firstNamed(args)
	valid, err := canType(first)
	if err != nil {
		return err
	}
	if !valid {
		return nil
	}
	for steps, cur := 0, first; cur != 0; steps++ {
		if steps&255 == 0 {
			if err := poll(); err != nil {
				return err
			}
		}
		node := s.records[cur-1]
		switch node.Symbol {
		case p.Identifier:
			retag(cur, p.TypeIdentifier)
			cur = 0
		case p.Selector:
			for _, c := range s.children[node.FirstChild : node.FirstChild+uint32(node.ChildCount)] {
				child := &s.records[c-1]
				old := child.Field
				child.Field = 0
				switch old {
				case p.OperandField:
					retag(c, p.PackageIdentifier)
					child.Field = p.PackageField
				case p.SelectorField:
					retag(c, p.TypeIdentifier)
					child.Field = p.NameField
				}
			}
			retag(cur, p.Qualified)
			cur = 0
		case p.Unary:
			next := childField(cur, p.OperandField)
			for _, c := range s.children[node.FirstChild : node.FirstChild+uint32(node.ChildCount)] {
				s.records[c-1].Field = 0
			}
			retag(cur, p.Pointer)
			cur = next
		case p.Parenthesized:
			next := firstNamed(cur)
			retag(cur, p.ParenthesizedType)
			cur = next
		default:
			cur = 0
		}
	}
	return nil
}
