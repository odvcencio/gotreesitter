package parsercorephase0

import "errors"

// ErrCSelectionComparisonBudget reports that a compact directed acyclic graph
// repeated more raw-tree comparison work than its arena can justify. A caller
// can treat this as an unsupported election shape and retain its fallback.
var ErrCSelectionComparisonBudget = errors.New("parser-core phase zero: C subtree comparison exceeded the arena work budget")

type cSelectionSubtreePair struct {
	left  SubtreeID
	right SubtreeID
}

// CompareCSelectionSubtrees ports tree-sitter's ts_subtree_compare ordering.
// It compares only the raw symbol and child count, then visits children from
// left to right. A negative result orders left first. A positive result orders
// right first. Zero means the two raw trees are equal under C's comparator.
//
// Compact children always precede their parent. The walk checks this invariant
// and fails closed if corrupt arena data could create a cycle.
func (c *Core) CompareCSelectionSubtrees(left, right SubtreeID) (int, error) {
	if c == nil {
		return 0, errors.New("parser-core phase zero: C subtree comparison requires a core")
	}
	stack := make([]cSelectionSubtreePair, 1, 64)
	stack[0] = cSelectionSubtreePair{left: left, right: right}
	workBudget := uint64(len(c.subtrees)) + uint64(len(c.children))
	if workBudget == 0 {
		workBudget = 1
	}
	var work uint64
	for len(stack) != 0 {
		work++
		if work > workBudget {
			return 0, ErrCSelectionComparisonBudget
		}
		pair := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if pair.left == pair.right {
			continue
		}
		leftRecord, err := c.subtree(pair.left)
		if err != nil {
			return 0, err
		}
		rightRecord, err := c.subtree(pair.right)
		if err != nil {
			return 0, err
		}
		leftReuse, leftOpaque := c.reusedSubtree(pair.left)
		rightReuse, rightOpaque := c.reusedSubtree(pair.right)
		if leftOpaque || rightOpaque {
			if leftOpaque && rightOpaque && leftReuse == rightReuse {
				continue
			}
			return 0, errors.New("parser-core phase zero: C selection cannot inspect an opaque reused subtree")
		}
		if leftRecord.symbol < rightRecord.symbol {
			return -1, nil
		}
		if rightRecord.symbol < leftRecord.symbol {
			return 1, nil
		}
		if leftRecord.childCount < rightRecord.childCount {
			return -1, nil
		}
		if rightRecord.childCount < leftRecord.childCount {
			return 1, nil
		}
		for index := leftRecord.childCount; index > 0; index-- {
			leftChild := c.children[leftRecord.firstChild+index-1]
			rightChild := c.children[rightRecord.firstChild+index-1]
			if leftChild >= pair.left || rightChild >= pair.right {
				return 0, errors.New("parser-core phase zero: compact subtree child identifier is out of order during C selection comparison")
			}
			stack = append(stack, cSelectionSubtreePair{left: leftChild, right: rightChild})
			if len(stack) > len(c.subtrees) {
				return 0, errors.New("parser-core phase zero: C selection comparison exceeded the subtree arena")
			}
		}
	}
	return 0, nil
}
