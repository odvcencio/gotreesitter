package benchfixtures

import "fmt"

// CliffFrontier describes one parser route on one exact source file.
type CliffFrontier struct {
	MaxLive    uint64  `json:"max_live"`
	MultiShare float64 `json:"multi_share"`
	Measured   bool    `json:"measured"`
}

// CliffFailures applies the cliff rule in the v1 design, Workstream P3 and Gates.
// MultiShare is a fraction in [0, 1]. A declined route has no frontier verdict.
func CliffFailures(goRoute, cOracle CliffFrontier) []string {
	if !goRoute.Measured || !cOracle.Measured {
		return nil
	}
	var failures []string
	if goRoute.MaxLive > 2*cOracle.MaxLive+2 {
		failures = append(failures, fmt.Sprintf("Go live stacks %d exceed 2*C versions+2 (%d)", goRoute.MaxLive, 2*cOracle.MaxLive+2))
	}
	if goRoute.MultiShare > cOracle.MultiShare+0.15 {
		failures = append(failures, fmt.Sprintf("Go multi-stack share %.2f%% exceeds C %.2f%% by more than 15 points", 100*goRoute.MultiShare, 100*cOracle.MultiShare))
	}
	return failures
}
