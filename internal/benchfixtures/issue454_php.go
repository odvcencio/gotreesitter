package benchfixtures

import (
	"fmt"
	"strings"
)

// Issue454PHPFixtureBytes is the exact size of the downstream PHP edit
// workload reported in issue #454.
const Issue454PHPFixtureBytes = 137 * 1024

// Issue454PHPSource builds the deterministic issue #454 PHP workload. The
// first local variable contains the edit site for the $x0 to $0 deletion.
func Issue454PHPSource() []byte {
	var source strings.Builder
	source.Grow(Issue454PHPFixtureBytes)
	source.WriteString("<?php\n")
	for i := 0; ; i++ {
		line := fmt.Sprintf(
			"function f%d() { $x%d = %d; return $x%d; }\n",
			i,
			i,
			i,
			i,
		)
		if source.Len()+len(line) > Issue454PHPFixtureBytes {
			break
		}
		source.WriteString(line)
	}
	source.WriteString(strings.Repeat(" ", Issue454PHPFixtureBytes-source.Len()))
	return []byte(source.String())
}
