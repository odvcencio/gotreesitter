package parsercorephase0

import "errors"

// C joins reductions that pop to the same predecessor before choosing a
// subtree. Keep the winner's entire row so reusable child buffers stay unique.
func (c *Core) selectCRecoveryJoinedReductionPaths(paths []popPath, cost ReductionOutputCostFunc) ([]popPath, error) {
	if !c.diagnostics.cSubtreeSelectionCertified || len(paths) < 2 {
		return paths, nil
	}
	selected := paths[:0]
	for candidateIndex := range paths {
		candidate := paths[candidateIndex]
		incumbent := -1
		if !candidate.recoveryDiscontinuity {
			for i := range selected {
				if !selected[i].recoveryDiscontinuity && selected[i].prev == candidate.prev {
					incumbent = i
					break
				}
			}
		}
		if incumbent < 0 {
			position := len(selected)
			paths[position], paths[candidateIndex] = paths[candidateIndex], paths[position]
			selected = paths[:position+1]
			continue
		}
		left := selected[incumbent]
		leftCost, err := c.recoveryPopChildrenCost(left, cost)
		if err != nil {
			return nil, err
		}
		rightCost, err := c.recoveryPopChildrenCost(candidate, cost)
		if err != nil {
			return nil, err
		}
		replace := rightCost < leftCost || (rightCost == leftCost && candidate.score > left.score)
		if rightCost == leftCost && candidate.score == left.score {
			if rightCost > 0 {
				replace = true
			} else {
				comparison, err := c.compareCReductionChildren(left.children, candidate.children)
				if err != nil {
					return nil, err
				}
				replace = comparison > 0
			}
		}
		if replace {
			paths[incumbent], paths[candidateIndex] = paths[candidateIndex], paths[incumbent]
		}
	}
	return selected, nil
}

func (c *Core) recoveryPopChildrenCost(path popPath, cost ReductionOutputCostFunc) (uint32, error) {
	prefix, err := c.RecoveryStoredErrorCost(Head{Node: path.prev})
	if err != nil {
		return 0, err
	}
	total := uint64(0)
	for _, child := range path.children {
		cumulative, err := cost(path.prev, child)
		if err != nil {
			return 0, err
		}
		if cumulative < prefix {
			return 0, errors.New("parser-core phase zero: child recovery cost is below predecessor cost")
		}
		total += uint64(cumulative - prefix)
		if total > uint64(^uint32(0)) {
			return 0, errors.New("parser-core phase zero: child recovery cost overflow")
		}
	}
	return uint32(total), nil
}

func (c *Core) compareCReductionChildren(left, right []SubtreeID) (int, error) {
	if len(left) < len(right) {
		return -1, nil
	}
	if len(left) > len(right) {
		return 1, nil
	}
	for i := range left {
		comparison, err := c.CompareCSelectionSubtrees(left[i], right[i])
		if err != nil || comparison != 0 {
			return comparison, err
		}
	}
	return 0, nil
}
