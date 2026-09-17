//go:build !grammar_subset

package grammars

import grammarruntime "github.com/odvcencio/gotreesitter/grammars/runtime"

func init() { grammarruntime.RegisterBuiltinScanners() }
