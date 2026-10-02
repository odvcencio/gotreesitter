package graduation

import "fmt"

// RequestedGrammars names the complete fleet covered by the lane. An explicit
// per-language increment must still carry every gate for each selected grammar.
var RequestedGrammars = []string{"go", "java", "javascript", "typescript", "python", "rust", "c", "cpp", "c_sharp", "ruby", "php", "bash"}

// VerifyCoverage checks explicit measurement scope, including every enabled
// default. Scoping an increment cannot omit evidence for a promoted route.
func VerifyCoverage(matrix *Matrix, requested []string) error {
	catalog := map[string]bool{}
	for _, name := range RequestedGrammars {
		catalog[name] = true
	}
	wanted := map[string]bool{}
	for _, name := range requested {
		if !catalog[name] || wanted[name] {
			return fmt.Errorf("invalid or duplicate graduation scope %q", name)
		}
		wanted[name] = true
	}
	if len(wanted) == 0 {
		return fmt.Errorf("graduation scope is empty")
	}
	for name := range Allowlist() {
		if !wanted[name] {
			return fmt.Errorf("graduation scope omits enabled default %s", name)
		}
	}
	for _, language := range matrix.Languages {
		if !wanted[language.Grammar] {
			return fmt.Errorf("unexpected graduation grammar %s", language.Grammar)
		}
		delete(wanted, language.Grammar)
	}
	if len(wanted) != 0 {
		return fmt.Errorf("graduation matrix does not cover the requested scope")
	}
	return nil
}
