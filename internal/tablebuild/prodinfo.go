package tablebuild

import "github.com/odvcencio/gotreesitter/internal/lexbuild"

// This file ports build_tables/build_parse_table.rs: fn
// ParseTableBuilder::get_production_id.

// getProductionID is build_parse_table.rs: fn
// ParseTableBuilder::get_production_id. It interns the production info of
// the item's production (its alias sequence without trailing blanks and its
// field map, with the fields that hidden children pass up marked inherited)
// and returns its id. The id of a production is computed once, from the
// first item that reduces it; ids are given in the order add_actions first
// reduces each distinct info, and the first equal info is found by a linear
// search, as in the generator.
func (b *parseTableBuilder) getProductionID(item *ParseItem) uint32 {
	sg := b.sg
	if item.ProdID == startProductionID {
		panicLikeGenerator("get_production_id of the start production")
	}
	if id := b.productionInfoIDsByProdID[item.ProdID]; id >= 0 {
		return uint32(id)
	}
	info := ProductionInfo{FieldMap: map[string][]FieldLocation{}}

	steps := item.production(sg).steps
	for i, step := range steps {
		info.AliasSequence = append(info.AliasSequence, step.Alias)
		if step.HasField {
			info.FieldMap[step.Field] = append(info.FieldMap[step.Field], FieldLocation{Index: uint32(i)})
		}

		if step.Symbol.Kind == lexbuild.NonTerminal && !isVisible(sg.Variables[step.Symbol.Index].Kind) {
			// The generator iterates the field names of a hash map; each name
			// gets its own list, so the order reaches no result.
			for fieldName := range b.variableInfo[step.Symbol.Index].Fields {
				info.FieldMap[fieldName] = append(info.FieldMap[fieldName], FieldLocation{Index: uint32(i), Inherited: true})
			}
		}
	}

	for n := len(info.AliasSequence); n > 0 && !info.AliasSequence[n-1].Set; n-- {
		info.AliasSequence = info.AliasSequence[:n-1]
	}

	if len(steps) > b.parseTable.MaxAliasedProductionLength {
		b.parseTable.MaxAliasedProductionLength = len(steps)
	}

	id := -1
	for k := range b.parseTable.ProductionInfos {
		if b.parseTable.ProductionInfos[k].equal(&info) {
			id = k
			break
		}
	}
	if id < 0 {
		b.parseTable.ProductionInfos = append(b.parseTable.ProductionInfos, info)
		id = len(b.parseTable.ProductionInfos) - 1
	}
	b.productionInfoIDsByProdID[item.ProdID] = int32(id)
	return uint32(id)
}
