package parserresult_test

import "testing"

func TestRustStructFieldNamedDefaultParses(t *testing.T) {
	const src = "pub struct TyParam {\n    pub default: Option<P<Ty>>,\n}\n"
	tree, lang := parseByLanguageName(t, "rust", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected rust parse error: %s", root.SExpr(lang))
	}
}

func TestRustDefaultFnKeywordStillParses(t *testing.T) {
	const src = "impl Foo for Bar { default fn f() {} }\n"
	tree, lang := parseByLanguageName(t, "rust", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected rust parse error: %s", root.SExpr(lang))
	}
}

func TestRustMatchArmInteractionParses(t *testing.T) {
	const src = `impl Pat {
    pub fn walk<F>(&self, it: &mut F) -> bool
    where
        F: FnMut(&Pat) -> bool,
    {
        match self.node {
            PatKind::Ident(_, _, Some(ref p)) => p.walk(it),
            PatKind::Struct(_, ref fields, _) => fields.iter().all(|field| field.node.pat.walk(it)),
            PatKind::TupleStruct(_, ref s, _) | PatKind::Tuple(ref s, _) => {
                s.iter().all(|p| p.walk(it))
            }
            PatKind::Box(ref s) | PatKind::Ref(ref s, _) => s.walk(it),
            PatKind::Slice(ref before, ref slice, ref after) => {
                before.iter().all(|p| p.walk(it))
                    && slice.iter().all(|p| p.walk(it))
                    && after.iter().all(|p| p.walk(it))
            }
            PatKind::Wild
            | PatKind::Lit(_)
            | PatKind::Range(..)
            | PatKind::Ident(..)
            | PatKind::Path(..)
            | PatKind::Mac(_) => true,
        }
    }
}
`
	tree, lang := parseByLanguageName(t, "rust", src)
	root := tree.RootNode()
	if root.HasError() {
		t.Fatalf("unexpected rust parse error: %s", root.SExpr(lang))
	}
}
