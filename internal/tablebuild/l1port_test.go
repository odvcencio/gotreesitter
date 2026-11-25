package tablebuild

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// This file turns the prepared grammar into the records of layer L1. It holds
// test-only ports of the generator functions that read the prepared grammar
// to write parser.c and node-types.json: parts of render.rs (symbol
// identifiers, names, metadata, alias identifiers, the non-terminal alias map
// and the supertype map), of build_parse_table.rs (fn get_production_id) and
// of node_types.rs (fn generate_node_types, without the root and extra flags).
// The table builder will port render.rs and build_parse_table.rs in full;
// these copies exist so that L1 checks the prepared grammar alone.

// l1Port is the prepared grammar of one grammar with what L1 derives from it.
type l1Port struct {
	g     *PreparedGrammar
	info  []VariableInfo
	super []SupertypeSubtypes

	// symbols are the port's symbols in the order of the reference's symbol
	// ids (parse_table.symbols), found by alignSymbols; ids holds the C
	// identifier of each (render.rs: symbol_ids).
	symbols []Symbol
	ids     map[Symbol]string
	unused  []Symbol // port symbols with no reference symbol id

	symbolMap     map[Symbol]Symbol // render.rs: symbol_map
	aliasIDs      map[Alias]string  // render.rs: alias_ids (Set aliases only)
	uniqueAliases []Alias           // render.rs: unique_aliases
	infos         []productionInfo  // production infos of the reduced productions
}

func newL1Port(grammarJSON []byte) (*l1Port, error) {
	g, err := Prepare(grammarJSON)
	if err != nil {
		return nil, err
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		return nil, err
	}
	return &l1Port{g: g, info: info, super: GetSupertypeSymbolMap(g.Syntax, g.DefaultAliases, info)}, nil
}

// metadataForSymbol is render.rs: fn metadata_for_symbol.
func (p *l1Port) metadataForSymbol(s Symbol) (string, lexbuild.VariableType) {
	switch s.Kind {
	case lexbuild.NonTerminal:
		v := p.g.Syntax.Variables[s.Index]
		return v.Ident, v.Kind
	case lexbuild.Terminal:
		v := p.g.Lex.Variables[s.Index]
		return v.Ident, v.Kind
	case lexbuild.External:
		v := p.g.Syntax.ExternalTokens[s.Index]
		return v.Ident, v.Kind
	}
	return "end", lexbuild.VarHidden
}

// symbolID is render.rs: fn assign_symbol_id, without the insertion: the
// identifier the symbol gets when the identifiers in used are taken.
func (p *l1Port) symbolID(s Symbol, used map[string]bool) string {
	if s == lexbuild.EndSym() {
		return "ts_builtin_sym_end"
	}
	name, kind := p.metadataForSymbol(s)
	var id string
	switch kind {
	case lexbuild.VarAuxiliary:
		id = "aux_sym_" + sanitizeIdentifier(name)
	case lexbuild.VarAnonymous:
		id = "anon_sym_" + sanitizeIdentifier(name)
	default:
		id = "sym_" + sanitizeIdentifier(name)
	}
	suffixNumber, suffix := 1, ""
	for used[id] {
		id = id[:len(id)-len(suffix)]
		suffixNumber++
		suffix = fmt.Sprint(suffixNumber)
		id += suffix
	}
	return id
}

// alignSymbols maps the reference's symbol ids to the port's symbols. The
// generator lists the end token, then the used terminals in index order with
// the word token moved to slot 1, then the used externals, then the used
// non-terminals (build_tables.rs: fn populate_used_symbols), and names them in
// that order (render.rs: fn init). Which symbols are used depends on the parse
// table, which L2 checks; here each reference identifier takes the next port
// symbol, in that order, whose identifier (given the identifiers taken so
// far) equals it. Port symbols passed over are unused.
func (p *l1Port) alignSymbols(refIdents []string) error {
	sg, lg := p.g.Syntax, p.g.Lex
	var cands []Symbol
	if sg.HasWord && sg.WordToken.IsTerminal() {
		cands = append(cands, sg.WordToken)
	}
	for i := range lg.Variables {
		if s := lexbuild.TerminalSym(i); !(sg.HasWord && sg.WordToken == s) {
			cands = append(cands, s)
		}
	}
	for i := range sg.ExternalTokens {
		cands = append(cands, lexbuild.ExternalSym(i))
	}
	for i := range sg.Variables {
		cands = append(cands, lexbuild.NonTerminalSym(i))
	}
	used := map[string]bool{"ts_builtin_sym_end": true}
	p.ids = map[Symbol]string{lexbuild.EndSym(): "ts_builtin_sym_end"}
	p.symbols = []Symbol{lexbuild.EndSym()}
	ptr := 0
	for id := 1; id < len(refIdents); id++ {
		found := -1
		for j := ptr; j < len(cands); j++ {
			if p.symbolID(cands[j], used) == refIdents[id] {
				found = j
				break
			}
		}
		if found < 0 {
			return fmt.Errorf("reference symbol %d %s: no port symbol has that identifier", id, refIdents[id])
		}
		p.unused = append(p.unused, cands[ptr:found]...)
		s := cands[found]
		used[refIdents[id]] = true
		p.ids[s] = refIdents[id]
		p.symbols = append(p.symbols, s)
		ptr = found + 1
	}
	p.unused = append(p.unused, cands[ptr:]...)
	p.ids[lexbuild.EndOfNonTerminalExtraSym()] = p.ids[lexbuild.EndSym()]
	p.computeSymbolMap()
	p.computeProductionInfos()
	p.computeAliasIDs()
	return nil
}

// computeSymbolMap is the symbol_map part of render.rs: fn init.
func (p *l1Port) computeSymbolMap() {
	p.symbolMap = map[Symbol]Symbol{}
	for _, symbol := range p.symbols {
		mapping := symbol
		if alias, ok := p.g.DefaultAliases.get(symbol); ok {
			kind := lexbuild.VarAnonymous
			if alias.Named {
				kind = lexbuild.VarNamed
			}
			for _, other := range p.symbols {
				if oa, ok := p.g.DefaultAliases.get(other); ok {
					if lexbuild.SymbolLess(other, mapping) && aliasEq(oa, alias) {
						mapping = other
					}
				} else if n, k := p.metadataForSymbol(other); n == alias.Value && k == kind {
					mapping = other
					break
				}
			}
		} else if symbol.IsTerminal() {
			name, kind := p.metadataForSymbol(symbol)
			for _, other := range p.symbols {
				if otherName, otherKind := p.metadataForSymbol(other); otherName == name && otherKind == kind {
					if m, ok := p.symbolMap[other]; ok && m == symbol {
						break
					}
					mapping = other
					break
				}
			}
		}
		p.symbolMap[symbol] = mapping
	}
}

// symbolsForAlias is render.rs: fn symbols_for_alias.
func (p *l1Port) symbolsForAlias(alias Alias) []Symbol {
	kind := lexbuild.VarAnonymous
	if alias.Named {
		kind = lexbuild.VarNamed
	}
	var out []Symbol
	for _, s := range p.symbols {
		if d, ok := p.g.DefaultAliases.get(s); ok {
			if aliasEq(d, alias) {
				out = append(out, s)
			}
		} else if n, k := p.metadataForSymbol(s); n == alias.Value && k == kind {
			out = append(out, s)
		}
	}
	return out
}

// computeAliasIDs is the alias part of render.rs: fn init.
func (p *l1Port) computeAliasIDs() {
	p.aliasIDs = map[Alias]string{}
	for _, info := range p.infos {
		for _, a := range info.aliases {
			if !a.Set {
				continue
			}
			var id string
			if existing := p.symbolsForAlias(a); len(existing) > 0 {
				id = p.ids[p.symbolMap[existing[0]]]
			} else {
				i := sort.Search(len(p.uniqueAliases), func(k int) bool { return !aliasLessByName(p.uniqueAliases[k], a) })
				if i == len(p.uniqueAliases) || !aliasEq(p.uniqueAliases[i], a) {
					p.uniqueAliases = append(p.uniqueAliases, Alias{})
					copy(p.uniqueAliases[i+1:], p.uniqueAliases[i:])
					p.uniqueAliases[i] = a
				}
				if a.Named {
					id = "alias_sym_" + sanitizeIdentifier(a.Value)
				} else {
					id = "anon_alias_sym_" + sanitizeIdentifier(a.Value)
				}
			}
			if _, ok := p.aliasIDs[a]; !ok {
				p.aliasIDs[a] = id
			}
		}
	}
}

func aliasLessByName(a, b Alias) bool {
	if a.Value != b.Value {
		return a.Value < b.Value
	}
	return !a.Named && b.Named
}

// productionInfo is tables.rs: struct ProductionInfo. fields are in the order
// of render.rs: fn add_field_sequences (stably sorted by field name).
type productionInfo struct {
	aliases []Alias
	fields  []fieldLocation
}

// fieldLocation is tables.rs: struct FieldLocation, with its field name.
type fieldLocation struct {
	name      string
	index     int
	inherited bool
}

// getProductionInfo is the ProductionInfo part of build_parse_table.rs: fn
// get_production_id.
func (p *l1Port) getProductionInfo(prodID uint32) productionInfo {
	sg := p.g.Syntax
	var info productionInfo
	for i, step := range sg.Productions[prodID].Steps {
		info.aliases = append(info.aliases, step.Alias)
		if step.HasField {
			info.fields = append(info.fields, fieldLocation{step.Field, i, false})
		}
		if step.Symbol.IsNonTerminal() {
			if k := sg.Variables[step.Symbol.Index].Kind; k != lexbuild.VarNamed && k != lexbuild.VarAnonymous {
				for _, name := range p.info[step.Symbol.Index].FieldNames() {
					info.fields = append(info.fields, fieldLocation{name, i, true})
				}
			}
		}
	}
	for len(info.aliases) > 0 && !info.aliases[len(info.aliases)-1].Set {
		info.aliases = info.aliases[:len(info.aliases)-1]
	}
	sort.SliceStable(info.fields, func(a, b int) bool { return info.fields[a].name < info.fields[b].name })
	return info
}

// computeProductionInfos collects the production infos of the productions
// that the parse table reduces: those of the variables that are not inlined,
// followed through the inline map until no step holds an inlined symbol
// (the closure of item_set_builder.rs replaces an item whose next symbol is
// inlined by the items of the map). The infos are listed once each, in the
// order first met; their ids are L2's business.
func (p *l1Port) computeProductionInfos() {
	sg := p.g.Syntax
	seen := map[string]bool{}
	visited := map[inlineKey]bool{}
	var expand func(prodID, from uint32)
	expand = func(prodID, from uint32) {
		if visited[inlineKey{prodID, from}] {
			return
		}
		visited[inlineKey{prodID, from}] = true
		steps := sg.Productions[prodID].Steps
		for si := int(from); si < len(steps); si++ {
			if containsSymbol(sg.VariablesToInline, steps[si].Symbol) {
				ids, ok := p.g.Inlines.InlinedProdIDs(prodID, uint32(si))
				if !ok {
					panic(fmt.Sprintf("no inline map entry for production %d step %d", prodID, si))
				}
				for _, id := range ids {
					expand(id, uint32(si))
				}
				return
			}
		}
		info := p.getProductionInfo(prodID)
		if k := info.key(); !seen[k] {
			seen[k] = true
			p.infos = append(p.infos, info)
		}
	}
	for i := range sg.Variables {
		if containsSymbol(sg.VariablesToInline, lexbuild.NonTerminalSym(i)) {
			continue
		}
		vp := sg.VarProds[i]
		for prodID := vp[0]; prodID < vp[1]; prodID++ {
			expand(uint32(prodID), 0)
		}
	}
}

// key is the record value of a production info.
func (pi productionInfo) key() string {
	var b strings.Builder
	b.WriteString("aliases=[")
	first := true
	for j, a := range pi.aliases {
		if !a.Set {
			continue
		}
		if !first {
			b.WriteByte(' ')
		}
		first = false
		fmt.Fprintf(&b, "%d:%s", j, aliasRef(a.Value, a.Named))
	}
	b.WriteString("] fields=[")
	for k, f := range pi.fields {
		if k > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s:%d", f.name, f.index)
		if f.inherited {
			b.WriteString("*")
		}
	}
	b.WriteString("]")
	return b.String()
}

// aliasRef is the record form of a node type or alias: name and namedness.
func aliasRef(name string, named bool) string {
	if named {
		return fmt.Sprintf("%q/named", name)
	}
	return fmt.Sprintf("%q/anon", name)
}

// symbolRecord is the record value of a symbol: render.rs: fn
// add_symbol_names_list and fn add_symbol_metadata_list.
func (p *l1Port) symbolRecord(s Symbol) string {
	name, kind := p.metadataForSymbol(s)
	visible, named, supertype := false, false, false
	if a, ok := p.g.DefaultAliases.get(s); ok {
		name, visible, named = a.Value, true, a.Named
	} else {
		switch kind {
		case lexbuild.VarNamed:
			visible, named = true, true
		case lexbuild.VarAnonymous:
			visible = true
		case lexbuild.VarHidden:
			named = true
			supertype = containsSymbol(p.g.Syntax.SupertypeSymbols, s)
		}
	}
	return symbolRecordValue(name, visible, named, supertype)
}

func symbolRecordValue(name string, visible, named, supertype bool) string {
	return fmt.Sprintf("name=%q visible=%t named=%t supertype=%t", name, visible, named, supertype)
}

// nonTerminalAliasMap is render.rs: fn add_non_terminal_alias_map, with each
// alias as its name and namedness. The public symbol (the first entry of each
// row in C) is ts_symbol_map's business (L4) and is left out.
func (p *l1Port) nonTerminalAliasMap() map[string][]string {
	sg := p.g.Syntax
	bySymbol := map[Symbol]map[string]bool{}
	for i := range sg.Variables {
		vp := sg.VarProds[i]
		for prodID := vp[0]; prodID < vp[1]; prodID++ {
			for _, step := range sg.Productions[prodID].Steps {
				if !step.Alias.Set || !step.Symbol.IsNonTerminal() {
					continue
				}
				if d, ok := p.g.DefaultAliases.get(step.Symbol); ok && aliasEq(d, step.Alias) {
					continue
				}
				if _, ok := p.ids[step.Symbol]; !ok {
					continue
				}
				if _, ok := p.aliasIDs[step.Alias]; !ok {
					continue
				}
				if bySymbol[step.Symbol] == nil {
					bySymbol[step.Symbol] = map[string]bool{}
				}
				bySymbol[step.Symbol][aliasRef(step.Alias.Value, step.Alias.Named)] = true
			}
		}
	}
	out := map[string][]string{}
	for s, set := range bySymbol {
		var list []string
		for a := range set {
			list = append(list, a)
		}
		sort.Strings(list)
		out[p.ids[s]] = list
	}
	return out
}

// supertypeMap is the map of render.rs: fn add_supertype_map, keyed by the
// supertype's identifier (render.rs: fn init keeps the supertypes that have
// one).
func (p *l1Port) supertypeMap() map[string][]string {
	out := map[string][]string{}
	for _, e := range p.super {
		sid, ok := p.ids[e.Supertype]
		if !ok {
			continue
		}
		if _, dup := out[sid]; dup {
			continue
		}
		set := map[string]bool{}
		for _, ct := range e.Subtypes {
			if !ct.Aliased {
				if id, ok := p.ids[ct.Symbol]; ok {
					set[id] = true
				}
				continue
			}
			if id, ok := p.aliasIDs[ct.Alias]; ok {
				set[id] = true
				continue
			}
			for _, s := range p.symbolsForAlias(ct.Alias) {
				if id, ok := p.ids[s]; ok {
					set[id] = true
				}
			}
		}
		var list []string
		for id := range set {
			list = append(list, id)
		}
		sort.Strings(list)
		out[sid] = list
	}
	return out
}

// sanitizeIdentifier is render.rs: fn sanitize_identifier.
func sanitizeIdentifier(name string) string {
	var b strings.Builder
	for _, c := range name {
		if c < utf8.RuneSelf && (c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			b.WriteRune(c)
			continue
		}
		var replacement string
		switch {
		case c == ' ' && len(name) == 1:
			replacement = "SPACE"
		case c == ' ':
			continue
		case c >= 0x80 && c <= 0xFFFF && c != 0xFEFF:
			fmt.Fprintf(&b, "u%04x", c)
			continue
		case c >= 0x10000:
			fmt.Fprintf(&b, "U%08x", c)
			continue
		default:
			replacement = specialCharNames[c]
		}
		if s := b.String(); s != "" && !strings.HasSuffix(s, "_") {
			b.WriteByte('_')
		}
		b.WriteString(replacement)
	}
	return b.String()
}

var specialCharNames = map[rune]string{
	'~': "TILDE", '`': "BQUOTE", '!': "BANG", '@': "AT", '#': "POUND", '$': "DOLLAR", '%': "PERCENT",
	'^': "CARET", '&': "AMP", '*': "STAR", '(': "LPAREN", ')': "RPAREN", '-': "DASH", '+': "PLUS",
	'=': "EQ", '{': "LBRACE", '}': "RBRACE", '[': "LBRACK", ']': "RBRACK", '\\': "BSLASH", '|': "PIPE",
	':': "COLON", ';': "SEMI", '"': "DQUOTE", '\'': "SQUOTE", '<': "LT", '>': "GT", ',': "COMMA",
	'.': "DOT", '?': "QMARK", '/': "SLASH", '\n': "LF", '\r': "CR", '\t': "TAB", 0: "NULL",
	1: "SOH", 2: "STX", 3: "ETX", 4: "EOT", 5: "ENQ", 6: "ACK", 7: "BEL", 8: "BS", 0x0b: "VTAB",
	0x0c: "FF", 0x0e: "SO", 0x0f: "SI", 0x10: "DLE", 0x11: "DC1", 0x12: "DC2", 0x13: "DC3",
	0x14: "DC4", 0x15: "NAK", 0x16: "SYN", 0x17: "ETB", 0x18: "CAN", 0x19: "EM", 0x1a: "SUB",
	0x1b: "ESC", 0x1c: "FS", 0x1d: "GS", 0x1e: "RS", 0x1f: "US", 0x7f: "DEL", 0xFEFF: "BOM",
}

// nodeTypeRef is node_types.rs: struct NodeTypeRef.
type nodeTypeRef struct {
	kind  string
	named bool
}

func (r nodeTypeRef) String() string { return aliasRef(r.kind, r.named) }

// nodeRefLess is node_types.rs: fn sort_node_type_refs (by name, then namedness).
func nodeRefLess(a, b nodeTypeRef) bool {
	if a.kind != b.kind {
		return a.kind < b.kind
	}
	return !a.named && b.named
}

// fieldInfoJSON is node_types.rs: struct FieldInfoJSON.
type fieldInfoJSON struct {
	multiple, required bool
	types              []nodeTypeRef
}

func defaultFieldInfoJSON() *fieldInfoJSON { return &fieldInfoJSON{required: true} }

// nodeInfoJSON is node_types.rs: struct NodeInfoJSON, without root and extra.
type nodeInfoJSON struct {
	ref      nodeTypeRef
	fields   map[string]*fieldInfoJSON // nil: None
	children *fieldInfoJSON            // nil: None
	subtypes []nodeTypeRef
	hasSub   bool
}

// childTypeToNodeType is node_types.rs: fn child_type_to_node_type.
func (p *l1Port) childTypeToNodeType(ct ChildType) nodeTypeRef {
	if ct.Aliased {
		return nodeTypeRef{ct.Alias.Value, ct.Alias.Named}
	}
	if a, ok := p.g.DefaultAliases.get(ct.Symbol); ok {
		return nodeTypeRef{a.Value, a.Named}
	}
	name, kind := p.metadataForSymbol(ct.Symbol)
	return nodeTypeRef{name, kind != lexbuild.VarAnonymous}
}

func sortDedupRefs(list []nodeTypeRef) []nodeTypeRef {
	sort.SliceStable(list, func(i, j int) bool { return nodeRefLess(list[i], list[j]) })
	out := list[:0]
	for i, r := range list {
		if i == 0 || r != list[i-1] {
			out = append(out, r)
		}
	}
	return out
}

// populateFieldInfoJSON is node_types.rs: fn populate_field_info_json.
func (p *l1Port) populateFieldInfoJSON(j *fieldInfoJSON, info *FieldInfo) {
	if len(info.Types) == 0 {
		j.required = false
		return
	}
	j.multiple = j.multiple || info.Quantity.Multiple
	j.required = j.required && info.Quantity.Required
	for _, t := range info.Types {
		j.types = append(j.types, p.childTypeToNodeType(t))
	}
	j.types = sortDedupRefs(j.types)
}

// generateNodeTypes is node_types.rs: fn generate_node_types, without the
// root and extra flags (L1 does not compare them) and without the final sort.
func (p *l1Port) generateNodeTypes() (map[nodeTypeRef]*nodeInfoJSON, error) {
	sg := p.g.Syntax
	aliasesBySymbol := getAliasesBySymbol(sg, p.g.DefaultAliases)
	nodes := map[nodeTypeRef]*nodeInfoJSON{}

	// build_supertype_entries
	type subtypeEntry struct {
		super nodeTypeRef
		subs  []nodeTypeRef
	}
	var subtypeMap []subtypeEntry
	for i := range p.info {
		symbol := lexbuild.NonTerminalSym(i)
		if !containsSymbol(sg.SupertypeSymbols, symbol) {
			continue
		}
		ref := nodeTypeRef{sg.Variables[i].Ident, true}
		n := nodes[ref]
		if n == nil {
			n = &nodeInfoJSON{ref: ref}
			nodes[ref] = n
		}
		var subs []nodeTypeRef
		for _, t := range p.info[i].Children.Types {
			subs = append(subs, p.childTypeToNodeType(t))
		}
		subs = sortDedupRefs(subs)
		if len(subs) > 0 {
			subtypeMap = append(subtypeMap, subtypeEntry{ref, append([]nodeTypeRef(nil), subs...)})
		}
		n.subtypes, n.hasSub = subs, true
	}

	// build_regular_entries
	for i := range p.info {
		symbol := lexbuild.NonTerminalSym(i)
		if containsSymbol(sg.VariablesToInline, symbol) {
			continue
		}
		isSupertype := containsSymbol(sg.SupertypeSymbols, symbol)
		variable := sg.Variables[i]
		info := &p.info[i]
		for _, alias := range aliasesBySymbol[symbol].Sorted() {
			if isSupertype && !alias.Set {
				continue
			}
			var ref nodeTypeRef
			switch {
			case alias.Set:
				ref = nodeTypeRef{alias.Value, alias.Named}
			case variable.Kind == lexbuild.VarNamed || variable.Kind == lexbuild.VarAnonymous:
				ref = nodeTypeRef{variable.Ident, variable.Kind == lexbuild.VarNamed}
			default:
				continue
			}
			existed := true
			n := nodes[ref]
			if n == nil {
				existed = false
				n = &nodeInfoJSON{ref: ref, fields: map[string]*fieldInfoJSON{}}
				nodes[ref] = n
			}
			if n.fields == nil {
				// A supertype entry takes its fields here (fields.as_mut().unwrap()
				// would panic in the generator; L1 reports it).
				return nil, fmt.Errorf("node %s: regular entry over a supertype entry", ref)
			}
			for _, name := range info.FieldNames() {
				fj := n.fields[name]
				if fj == nil {
					fj = defaultFieldInfoJSON()
					if existed {
						fj.required = false
					}
					n.fields[name] = fj
				}
				p.populateFieldInfoJSON(fj, info.Fields[name])
			}
			for name, fj := range n.fields {
				if _, ok := info.Fields[name]; !ok {
					fj.required = false
				}
			}
			if n.children == nil {
				n.children = defaultFieldInfoJSON()
			}
			p.populateFieldInfoJSON(n.children, &info.ChildrenWithoutFields)
		}
	}

	// sort_subtype_map_topologically: Kahn's algorithm, one layer at a time,
	// each layer sorted by name (the topological_sort crate's pop_all).
	deps := map[nodeTypeRef]map[nodeTypeRef]bool{} // node -> nodes it waits for
	all := map[nodeTypeRef]bool{}
	for _, e := range subtypeMap {
		all[e.super] = true
		for _, s := range e.subs {
			all[s] = true
			if deps[e.super] == nil {
				deps[e.super] = map[nodeTypeRef]bool{}
			}
			deps[e.super][s] = true
		}
	}
	var sorted []nodeTypeRef
	for len(all) > 0 {
		var layer []nodeTypeRef
		for n := range all {
			if len(deps[n]) == 0 {
				layer = append(layer, n)
			}
		}
		if len(layer) == 0 {
			return nil, fmt.Errorf("dependency cycle in node types")
		}
		sort.Slice(layer, func(i, j int) bool { return nodeRefLess(layer[i], layer[j]) })
		for _, n := range layer {
			delete(all, n)
			for _, d := range deps {
				delete(d, n)
			}
		}
		sorted = append(sorted, layer...)
	}
	pos := map[nodeTypeRef]int{}
	for i, n := range sorted {
		pos[n] = i
	}
	sort.SliceStable(subtypeMap, func(i, j int) bool { return pos[subtypeMap[i].super] < pos[subtypeMap[j].super] })

	// apply_supertype_collapsing and process_supertypes
	process := func(f *fieldInfoJSON) {
		for _, e := range subtypeMap {
			has := false
			for _, t := range f.types {
				if t == e.super {
					has = true
				}
			}
			if !has {
				continue
			}
			out := f.types[:0]
			for _, t := range f.types {
				keep := true
				for _, s := range e.subs {
					if t == s {
						keep = false
					}
				}
				if keep {
					out = append(out, t)
				}
			}
			f.types = out
		}
	}
	for _, n := range nodes {
		if n.children != nil && len(n.children.types) == 0 {
			n.children = nil
		}
		if n.children != nil {
			process(n.children)
		}
		for _, f := range n.fields {
			process(f)
		}
	}

	// build_token_entries
	type token struct {
		name string
		kind lexbuild.VariableType
	}
	var tokens []token
	add := func(sym Symbol, name string, kind lexbuild.VariableType) {
		for _, a := range aliasesBySymbol[sym].Sorted() {
			if a.Set {
				k := lexbuild.VarAnonymous
				if a.Named {
					k = lexbuild.VarNamed
				}
				tokens = append(tokens, token{a.Value, k})
			} else {
				tokens = append(tokens, token{name, kind})
			}
		}
	}
	for i, v := range p.g.Lex.Variables {
		add(lexbuild.TerminalSym(i), v.Ident, v.Kind)
	}
	for i, v := range sg.ExternalTokens {
		add(lexbuild.ExternalSym(i), v.Ident, v.Kind)
	}
	for _, tk := range tokens {
		if tk.kind != lexbuild.VarNamed && tk.kind != lexbuild.VarAnonymous {
			continue
		}
		ref := nodeTypeRef{tk.name, tk.kind == lexbuild.VarNamed}
		if n := nodes[ref]; n != nil {
			if n.children != nil {
				n.children.required = false
			}
			for _, f := range n.fields {
				f.required = false
			}
			continue
		}
		nodes[ref] = &nodeInfoJSON{ref: ref}
	}
	return nodes, nil
}

// nodeRecords turns node entries into L1 records.
func nodeRecords(nodes map[nodeTypeRef]*nodeInfoJSON, add func(key, value string)) {
	refs := func(list []nodeTypeRef) string {
		parts := make([]string, len(list))
		for i, r := range list {
			parts[i] = r.String()
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	fieldValue := func(f *fieldInfoJSON) string {
		return fmt.Sprintf("multiple=%t required=%t types=%s", f.multiple, f.required, refs(f.types))
	}
	for ref, n := range nodes {
		key := "node " + ref.String()
		kind := "leaf"
		switch {
		case n.hasSub:
			kind = "supertype"
		case n.fields != nil:
			kind = "regular"
		}
		add(key, kind)
		if n.hasSub {
			add(key+" subtypes", refs(n.subtypes))
		}
		if n.children != nil {
			add(key+" children", fieldValue(n.children))
		}
		for name, f := range n.fields {
			add(key+" field "+name, fieldValue(f))
		}
	}
}
