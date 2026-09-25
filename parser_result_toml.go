package gotreesitter

// C keeps a terminal "key = " pair inside its table and inserts a missing
// integer. The Go recovery can instead publish that pair as a root ERROR.
func normalizeTOMLTrailingUnfinishedPair(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || root.ownerArena == nil || root.Type(lang) != "document" {
		return
	}
	count := resultChildCount(root)
	if count < 2 {
		return
	}
	table := resultChildAt(root, count-2)
	err := resultChildAt(root, count-1)
	if table == nil || err == nil || table.Type(lang) != "table" || !err.IsError() || resultChildCount(err) != 2 {
		return
	}
	key, equals := resultChildAt(err, 0), resultChildAt(err, 1)
	if key == nil || equals == nil || key.Type(lang) != "bare_key" || equals.Type(lang) != "=" ||
		err.startByte != key.startByte || err.endByte != equals.endByte ||
		int(equals.endByte)+1 != len(source) || source[equals.endByte] != ' ' {
		return
	}
	pairSym, pairOK := symbolByName(lang, "pair")
	integerSym, integerOK := symbolByName(lang, "integer")
	if !pairOK || !integerOK {
		return
	}
	arena := root.ownerArena
	integer := newLeafNodeInArena(arena, integerSym, symbolIsNamed(lang, integerSym), equals.endByte, equals.endByte, equals.endPoint, equals.endPoint)
	integer.setHasError(true)
	pair := newParentNodeInArena(arena, pairSym, symbolIsNamed(lang, pairSym), []*Node{key, equals, integer}, nil, 0)
	pair.endByte = uint32(len(source))
	pair.endPoint = root.endPoint
	pair.setHasError(true)
	completedTable := cloneNodeInArenaAppendingChildForMutation(arena, table, pair)
	if completedTable == nil {
		return
	}
	completedTable.endByte = pair.endByte
	completedTable.endPoint = pair.endPoint
	completedTable.setHasError(true)
	replaceChildRangeWithSingleNode(root, count-2, count, completedTable)
	root.setHasError(true)
}

// C's visible integer alias contains a hidden missing token. Restore its
// error flags after the generic stale-flag pass checks visible nodes.
func restoreTOMLTrailingUnfinishedPairErrorFlags(tree *Tree, source []byte, lang *Language) {
	if tree == nil || lang == nil || lang.Name != "toml" || tree.root == nil || tree.root.Type(lang) != "document" {
		return
	}
	root := tree.root
	if resultChildCount(root) == 0 {
		return
	}
	table := resultChildAt(root, resultChildCount(root)-1)
	if table == nil || table.Type(lang) != "table" || resultChildCount(table) == 0 {
		return
	}
	pair := resultChildAt(table, resultChildCount(table)-1)
	if pair == nil || pair.Type(lang) != "pair" || resultChildCount(pair) != 3 {
		return
	}
	equals, integer := resultChildAt(pair, 1), resultChildAt(pair, 2)
	if equals == nil || integer == nil || equals.Type(lang) != "=" || integer.Type(lang) != "integer" ||
		integer.startByte != integer.endByte || integer.startByte != equals.endByte ||
		int(equals.endByte)+1 != len(source) || source[equals.endByte] != ' ' {
		return
	}
	integer.setHasError(true)
	pair.setHasError(true)
	table.setHasError(true)
	root.setHasError(true)
	tree.resultErrorSummary = resultErrorSummaryPresent
}
