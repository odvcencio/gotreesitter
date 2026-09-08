package benchfixtures

// GoGenericConflictFixture contains an expression and its stable test name.
type GoGenericConflictFixture struct {
	Name       string
	Expression string
}

// Source puts the expression in a short function body.
func (f GoGenericConflictFixture) Source() []byte {
	return []byte("package p\n\nfunc f() {\n\t_ = " + f.Expression + "\n}\n")
}

// GoGenericConflictFamily returns the ambiguity cases and their simpler controls.
func GoGenericConflictFamily() []GoGenericConflictFixture {
	return []GoGenericConflictFixture{
		{"one_type_arg", "Foo[int](a)"},
		{"two_type_args", "Pair[int, string](p)"},
		{"generic_call_two_args", "Max[int](1, 2)"},
		{"generic_call_one_arg", "Max[int](1)"},
		{"nested_generic", "Foo[Foo[int]](a)"},
		{"nested_type_args", "Map[string, []int](m)"},
		{"paren_form", "(Foo[int])(a)"},
		{"chain", "Foo[int](Foo[int](a))"},
		{"argument_generic", "g(Foo[int](a))"},
		{"real_index_call", "a[b](c)"},
		{"selector_generic_call", "pkg.Max[int](1, 2)"},
		{"in_expression", "Max[int](1, 2) + Max[int](3, 4)"},
		{"composite_literal", "Foo[int]{}"},
		{"bare_instantiation", "Max[int]"},
		{"plain_call", "g(x)"},
		{"plain_index", "a[b]"},
		{"selector_call", "pkg.F(x)"},
		{"make_slice", "make([]int, 0)"},
		{"new_type", "new(Foo)"},
	}
}
