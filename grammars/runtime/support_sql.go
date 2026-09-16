//go:build !grammar_subset || grammar_subset_sql

package grammarruntime

// RegisterSqlSupport registers the scanner support for sql.
func RegisterSqlSupport() {
	RegisterExternalScanner("sql", SqlExternalScanner{})
}
