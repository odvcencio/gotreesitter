package grammars

import (
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// These computed setters parsed cleanly before the contextual suffix scan.
// Cover JSX text, regex operands, private names, and non-ASCII identifiers.
func TestJavaScriptTypeScriptComputedSetterLookahead(t *testing.T) {
	cases := []struct{ name, language, source string }{
		{"param-18057", "javascript", "class A { set [k](v = a ^ /'/.source.length) {} }"},
		{"param-18058", "typescript", "class A { set [k](v = a ^ /'/.source.length) {} }"},
		{"param-18059", "tsx", "class A { set [k](v = a ^ /'/.source.length) {} }"},
		{"param-18063", "javascript", "({ set [k](v = a ^ /'/.source.length) {} });"},
		{"param-18064", "typescript", "({ set [k](v = a ^ /'/.source.length) {} });"},
		{"param-18065", "tsx", "({ set [k](v = a ^ /'/.source.length) {} });"},
		{"param-18066", "javascript", "class A { #in = 1; set [k](v = a ^ /'/.source.length) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18067", "typescript", "class A { #in = 1; set [k](v = a ^ /'/.source.length) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18068", "tsx", "class A { #in = 1; set [k](v = a ^ /'/.source.length) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18069", "javascript", "class A { set [`${k}`](v = a ^ /'/.source.length) {} }"},
		{"param-18070", "typescript", "class A { set [`${k}`](v = a ^ /'/.source.length) {} }"},
		{"param-18071", "tsx", "class A { set [`${k}`](v = a ^ /'/.source.length) {} }"},
		{"param-18117", "javascript", "class A { set [k](v = this.#in / 2) {} }"},
		{"param-18118", "typescript", "class A { set [k](v = this.#in / 2) {} }"},
		{"param-18119", "tsx", "class A { set [k](v = this.#in / 2) {} }"},
		{"param-18123", "javascript", "({ set [k](v = this.#in / 2) {} });"},
		{"param-18124", "typescript", "({ set [k](v = this.#in / 2) {} });"},
		{"param-18125", "tsx", "({ set [k](v = this.#in / 2) {} });"},
		{"param-18126", "javascript", "class A { #in = 1; set [k](v = this.#in / 2) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18127", "typescript", "class A { #in = 1; set [k](v = this.#in / 2) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18128", "tsx", "class A { #in = 1; set [k](v = this.#in / 2) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18129", "javascript", "class A { set [`${k}`](v = this.#in / 2) {} }"},
		{"param-18130", "typescript", "class A { set [`${k}`](v = this.#in / 2) {} }"},
		{"param-18131", "tsx", "class A { set [`${k}`](v = this.#in / 2) {} }"},
		{"param-18132", "javascript", "class A { set [k](v = éin / 2) {} }"},
		{"param-18133", "typescript", "class A { set [k](v = éin / 2) {} }"},
		{"param-18134", "tsx", "class A { set [k](v = éin / 2) {} }"},
		{"param-18138", "javascript", "({ set [k](v = éin / 2) {} });"},
		{"param-18139", "typescript", "({ set [k](v = éin / 2) {} });"},
		{"param-18140", "tsx", "({ set [k](v = éin / 2) {} });"},
		{"param-18141", "javascript", "class A { #in = 1; set [k](v = éin / 2) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18142", "typescript", "class A { #in = 1; set [k](v = éin / 2) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18143", "tsx", "class A { #in = 1; set [k](v = éin / 2) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18144", "javascript", "class A { set [`${k}`](v = éin / 2) {} }"},
		{"param-18145", "typescript", "class A { set [`${k}`](v = éin / 2) {} }"},
		{"param-18146", "tsx", "class A { set [`${k}`](v = éin / 2) {} }"},
		{"param-18237", "javascript", "class A { set [k](v = <a>x</a>) {} }"},
		{"param-18238", "tsx", "class A { set [k](v = <a>x</a>) {} }"},
		{"param-18241", "javascript", "({ set [k](v = <a>x</a>) {} });"},
		{"param-18242", "tsx", "({ set [k](v = <a>x</a>) {} });"},
		{"param-18243", "javascript", "class A { #in = 1; set [k](v = <a>x</a>) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18244", "tsx", "class A { #in = 1; set [k](v = <a>x</a>) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18245", "javascript", "class A { set [`${k}`](v = <a>x</a>) {} }"},
		{"param-18246", "tsx", "class A { set [`${k}`](v = <a>x</a>) {} }"},
		{"param-18257", "javascript", "class A { set [k](v = <p>don't</p>) {} }"},
		{"param-18258", "tsx", "class A { set [k](v = <p>don't</p>) {} }"},
		{"param-18261", "javascript", "({ set [k](v = <p>don't</p>) {} });"},
		{"param-18262", "tsx", "({ set [k](v = <p>don't</p>) {} });"},
		{"param-18263", "javascript", "class A { #in = 1; set [k](v = <p>don't</p>) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18264", "tsx", "class A { #in = 1; set [k](v = <p>don't</p>) { this.v = 1; } get [k]() { return 1; } }"},
		{"param-18265", "javascript", "class A { set [`${k}`](v = <p>don't</p>) {} }"},
		{"param-18266", "tsx", "class A { set [`${k}`](v = <p>don't</p>) {} }"},
		{"b2-144", "javascript", "class A { #typeof = 1; set [k](v = this.#typeof / 2) {} }"},
		{"b2-145", "typescript", "class A { #typeof = 1; set [k](v = this.#typeof / 2) {} }"},
		{"b2-146", "tsx", "class A { #typeof = 1; set [k](v = this.#typeof / 2) {} }"},
		{"b2-147", "javascript", "class A { #new = 1; set [k](v = this.#new / \"]\".length) {} }"},
		{"b2-148", "typescript", "class A { #new = 1; set [k](v = this.#new / \"]\".length) {} }"},
		{"b2-149", "tsx", "class A { #new = 1; set [k](v = this.#new / \"]\".length) {} }"},
		{"b2-156", "javascript", "class A { set [k](v = ünew / 2) {} }"},
		{"b2-157", "typescript", "class A { set [k](v = ünew / 2) {} }"},
		{"b2-158", "tsx", "class A { set [k](v = ünew / 2) {} }"},
		{"b2-162", "javascript", "class A { set [k](v = 変in / 2) {} }"},
		{"b2-163", "typescript", "class A { set [k](v = 変in / 2) {} }"},
		{"b2-164", "tsx", "class A { set [k](v = 変in / 2) {} }"},
		{"b2-177", "javascript", "class A { get [k]() { return 1 } set [k](v = await / 2) {} }"},
		{"b2-204", "javascript", "class A { set [k](v = <A.B>x</A.B>) {} }"},
		{"b2-205", "tsx", "class A { set [k](v = <A.B>x</A.B>) {} }"},
		{"b2-206", "javascript", "class A { set [k](v = <a></a>) {} }"},
		{"b2-207", "tsx", "class A { set [k](v = <a></a>) {} }"},
		{"b2-208", "javascript", "class A { set [k](v = <a>{x}</a>) {} }"},
		{"b2-209", "tsx", "class A { set [k](v = <a>{x}</a>) {} }"},
		{"b2-210", "javascript", "class A { set [k](v = cond ? <a/> : <b></b>) {} }"},
		{"b2-211", "tsx", "class A { set [k](v = cond ? <a/> : <b></b>) {} }"},
		{"b2-218", "javascript", "class A { set [k](v = <>x</>) {} }"},
		{"b2-219", "tsx", "class A { set [k](v = <>x</>) {} }"},
		{"b2-220", "javascript", "class A { set [k](v = <a><b /></a>) {} }"},
		{"b2-221", "tsx", "class A { set [k](v = <a><b /></a>) {} }"},
		{"b2-222", "javascript", "class A { set [k](v = <a>x</a>, w = 1) {} }"},
		{"b2-223", "tsx", "class A { set [k](v = <a>x</a>, w = 1) {} }"},
		{"b2-224", "javascript", "const o = { set [k](v = <a>x</a>) {} };"},
		{"b2-225", "tsx", "const o = { set [k](v = <a>x</a>) {} };"},
		{"b2-228", "javascript", "const el = <div>{ ({ set [k](v = <b>y</b>) {} }) }</div>;"},
		{"b2-229", "tsx", "const el = <div>{ ({ set [k](v = <b>y</b>) {} }) }</div>;"},
		{"b2-232", "javascript", "class C extends React.Component { set [k](node = <span>loading</span>) { this.n = node; } render() { return <div>{this[k]}</div>; } }"},
		{"b2-233", "tsx", "class C extends React.Component { set [k](node = <span>loading</span>) { this.n = node; } render() { return <div>{this[k]}</div>; } }"},
	}
	for _, name := range []string{"javascript", "typescript", "tsx"} {
		t.Run(name, func(t *testing.T) {
			lang := DetectLanguageByName(name).Language()
			for _, c := range cases {
				if c.language != name {
					continue
				}
				t.Run(c.name, func(t *testing.T) {
					parser := gotreesitter.NewParser(lang)
					source := []byte(c.source)
					tree, err := parser.ParseStrict(source)
					if err != nil {
						t.Fatal(err)
					}
					defer tree.Release()
					assertCleanComputedSetterTree(t, tree, lang, len(source))

					// Edit inside the computed key so contextual lookahead is revisited.
					offset := strings.IndexByte(c.source, '[') + 1
					edited := []byte(c.source[:offset] + " " + c.source[offset:])
					tree.Edit(gotreesitter.InputEdit{
						StartByte: uint32(offset), OldEndByte: uint32(offset), NewEndByte: uint32(offset + 1),
						StartPoint: computedSetterPoint(source, offset), OldEndPoint: computedSetterPoint(source, offset), NewEndPoint: computedSetterPoint(edited, offset+1),
					})
					incremental, err := parser.ParseIncremental(edited, tree)
					if err != nil {
						t.Fatal(err)
					}
					defer incremental.Release()
					fresh, err := gotreesitter.NewParser(lang).ParseStrict(edited)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					assertCleanComputedSetterTree(t, incremental, lang, len(edited))
					assertCleanComputedSetterTree(t, fresh, lang, len(edited))
					if incremental.RootNode().SExpr(lang) != fresh.RootNode().SExpr(lang) {
						t.Fatalf("incremental tree differs from fresh: incremental=%s fresh=%s", incremental.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang))
					}
					if allocations := testing.AllocsPerRun(10, func() {
						next, err := parser.ParseIncremental(edited, incremental)
						if err != nil {
							panic(err)
						}
						next.Release()
					}); allocations != 0 {
						t.Fatalf("no-edit reparse allocated %.2f times", allocations)
					}
				})
			}
		})
	}
}

func assertCleanComputedSetterTree(t *testing.T, tree *gotreesitter.Tree, lang *gotreesitter.Language, length int) {
	t.Helper()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.HasErrorOrMissing() || tree.ParseStoppedEarly() || root.StartByte() != 0 || root.EndByte() != uint32(length) {
		t.Fatalf("invalid tree: %s", root.SExpr(lang))
	}
}

func computedSetterPoint(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	for _, ch := range source[:offset] {
		if ch == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}
