package grammars

import (
	"strings"
	"testing"

	ts "github.com/odvcencio/gotreesitter"
)

func TestBashRetryFullParseOnChainedIfs(t *testing.T) {
	src := []byte(strings.ReplaceAll(`a=1
if foo; then
  :
fi
b=x
c=x
tar=x
if [ -z "$tar" ]; then
  tar=x
fi
if [ -z "$tar" ]; then
  tar=\x06foo\x06
fi
if [ 1 -eq 0 ] && [ -x "$tar" ]; then
  :
fi
	`, `\x06`, "`"))
	p := ts.NewParser(BashLanguage())
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("unexpected error tree: %s", root.SExpr(BashLanguage()))
	}
}

func TestBashTrailingAndClauseSurvivesMixedElseBody(t *testing.T) {
	src := []byte(`x=1

if foo; then
  (exit 0)
else
  :
fi

true

node=x
ret=0
if [ $ret -eq 0 ] && [ -x "$node" ]; then
  (exit 0)
else
  true
  echo ""
  exit $ret
fi

cd "$TMP"   && (ret=0
      true
      if [ $ret -ne 0 ]; then
        echo "Aborted 0.x cleanup.  Exiting." >&2
        exit $ret
      fi)   && :
`)
	p := ts.NewParser(BashLanguage())
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("unexpected error tree: %s", root.SExpr(BashLanguage()))
	}
}

func TestBashElifCommandAssignmentDoesNotInheritRedirectField(t *testing.T) {
	src := []byte(`#!/bin/bash
(
  if [ $isnpm10 -eq 1 ]; then
    if [ "x$clean" = "xno" ] \
        || [ "x$clean" = "xn" ]; then
      echo "Skipping 0.x cruft clean" >&2
    elif [ "x$clean" = "xy" ] || [ "x$clean" = "xyes" ]; then
      NODE="$node" /bin/bash "scripts/clean-old.sh" "-y"
    else
      NODE="$node" /bin/bash "scripts/clean-old.sh" </dev/tty
    fi
  fi
)
`)
	p := ts.NewParser(BashLanguage())
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("unexpected error tree: %s", root.SExpr(BashLanguage()))
	}
	var found bool
	var walk func(*ts.Node)
	walk = func(n *ts.Node) {
		if n == nil || found {
			return
		}
		if n.Type(BashLanguage()) == "command" {
			text := string(src[n.StartByte():n.EndByte()])
			if strings.Contains(text, `"scripts/clean-old.sh" "-y"`) {
				if got := n.FieldNameForChild(0, BashLanguage()); got != "" {
					t.Fatalf("field on leading variable_assignment = %q, want empty; tree=%s", got, root.SExpr(BashLanguage()))
				}
				found = true
				return
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if !found {
		t.Fatalf("missing target command: %s", root.SExpr(BashLanguage()))
	}
}

func TestBashExpansionVariableNameDoesNotInheritOperatorField(t *testing.T) {
	src := []byte("t=\"${npm_install}\"\n")
	p := ts.NewParser(BashLanguage())
	tree, err := p.Parse(src)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	root := tree.RootNode()
	if root == nil {
		t.Fatal("missing root node")
	}
	if tree.ParseStopReason() != ts.ParseStopAccepted {
		t.Fatalf("stop=%s runtime=%s", tree.ParseStopReason(), tree.ParseRuntime().Summary())
	}
	if root.HasError() {
		t.Fatalf("unexpected error tree: %s", root.SExpr(BashLanguage()))
	}
	var found bool
	var walk func(*ts.Node)
	walk = func(n *ts.Node) {
		if n == nil || found {
			return
		}
		if n.Type(BashLanguage()) == "expansion" && n.ChildCount() >= 3 {
			if got := n.FieldNameForChild(1, BashLanguage()); got != "" {
				t.Fatalf("field on expansion variable_name = %q, want empty; tree=%s", got, root.SExpr(BashLanguage()))
			}
			found = true
			return
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	if !found {
		t.Fatalf("missing expansion: %s", root.SExpr(BashLanguage()))
	}
}
