package benchfixtures

import (
	"fmt"
	"strings"
)

// Issue454CFixtureBytes is the internal C regression size.
// The reporter generator grows past its target and does not pad to this size.
const Issue454CFixtureBytes = 137 * 1024

// Issue454CSource builds an internal approximation of the issue 454 C workload.
// The first local variable contains the edit site for the x0 to 0 deletion (a
// transient-error edit: the declarator "x0" loses its leading letter and
// becomes the bare integer literal "0").
func Issue454CSource() []byte {
	var source strings.Builder
	source.Grow(Issue454CFixtureBytes)
	for i := 0; ; i++ {
		line := fmt.Sprintf(
			"int f%d(void) { int x%d = %d; return x%d; }\n",
			i,
			i,
			i,
			i,
		)
		if source.Len()+len(line) > Issue454CFixtureBytes {
			break
		}
		source.WriteString(line)
	}
	source.WriteString(strings.Repeat(" ", Issue454CFixtureBytes-source.Len()))
	return []byte(source.String())
}
