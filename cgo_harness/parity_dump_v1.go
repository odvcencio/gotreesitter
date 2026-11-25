//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"

	sitter "github.com/tree-sitter/go-tree-sitter"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

const DumpV1Version = "dump.v1"

type DumpV1Point struct {
	Row    uint32 `json:"row"`
	Column uint32 `json:"column"`
}

type DumpV1Node struct {
	Path       string      `json:"path"`
	Type       string      `json:"type"`
	Field      string      `json:"field,omitempty"`
	IsNamed    bool        `json:"isNamed"`
	IsExtra    bool        `json:"isExtra"`
	HasError   bool        `json:"hasError"`
	StartByte  uint32      `json:"startByte"`
	EndByte    uint32      `json:"endByte"`
	StartPoint DumpV1Point `json:"startPoint"`
	EndPoint   DumpV1Point `json:"endPoint"`
	ChildCount int         `json:"childCount"`
}

type DumpV1Tree struct {
	Version string       `json:"version"`
	Nodes   []DumpV1Node `json:"nodes"`
}

type DumpV1Divergence struct {
	Path     string `json:"path"`
	Category string `json:"category"`
	GoValue  string `json:"goValue"`
	CValue   string `json:"cValue"`
}

func DumpV1FromGo(root *gotreesitter.Node, lang *gotreesitter.Language) DumpV1Tree {
	out := DumpV1Tree{Version: DumpV1Version}
	if root == nil || lang == nil {
		return out
	}
	walkDumpV1Go(root, lang, rootPath(dumpV1GoType(root, lang)), "", &out.Nodes)
	return out
}

func DumpV1FromC(root *sitter.Node) DumpV1Tree {
	out := DumpV1Tree{Version: DumpV1Version}
	if root == nil {
		return out
	}
	walkDumpV1C(root, rootPath(root.Kind()), "", &out.Nodes)
	return out
}

func FirstDivergenceDumpV1(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node) *DumpV1Divergence {
	if goNode == nil && cNode == nil {
		return nil
	}
	path := rootPath("?")
	if goNode != nil && goLang != nil {
		path = rootPath(goNode.Type(goLang))
	} else if cNode != nil {
		path = rootPath(cNode.Kind())
	}
	return firstDivergenceDumpV1(goNode, goLang, cNode, path)
}

func walkDumpV1Go(node *gotreesitter.Node, lang *gotreesitter.Language, path, field string, out *[]DumpV1Node) {
	if node == nil || lang == nil {
		return
	}
	start := node.StartPoint()
	end := node.EndPoint()
	typ := dumpV1GoType(node, lang)
	*out = append(*out, DumpV1Node{
		Path:      path,
		Type:      typ,
		Field:     field,
		IsNamed:   node.IsNamed(),
		IsExtra:   node.IsExtra(),
		HasError:  node.HasError(),
		StartByte: node.StartByte(),
		EndByte:   node.EndByte(),
		StartPoint: DumpV1Point{
			Row:    start.Row,
			Column: start.Column,
		},
		EndPoint: DumpV1Point{
			Row:    end.Row,
			Column: end.Column,
		},
		ChildCount: node.ChildCount(),
	})
	for i := 0; i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		childPath := childPath(path, dumpV1GoType(child, lang), i)
		walkDumpV1Go(child, lang, childPath, node.FieldNameForChild(i, lang), out)
	}
}

func walkDumpV1C(node *sitter.Node, path, field string, out *[]DumpV1Node) {
	if node == nil {
		return
	}
	start := node.StartPosition()
	end := node.EndPosition()
	*out = append(*out, DumpV1Node{
		Path:      path,
		Type:      node.Kind(),
		Field:     field,
		IsNamed:   node.IsNamed(),
		IsExtra:   node.IsExtra(),
		HasError:  node.HasError(),
		StartByte: uint32(node.StartByte()),
		EndByte:   uint32(node.EndByte()),
		StartPoint: DumpV1Point{
			Row:    uint32(start.Row),
			Column: uint32(start.Column),
		},
		EndPoint: DumpV1Point{
			Row:    uint32(end.Row),
			Column: uint32(end.Column),
		},
		ChildCount: int(node.ChildCount()),
	})
	count := int(node.ChildCount())
	for i := 0; i < count; i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}
		childPath := childPath(path, child.Kind(), i)
		walkDumpV1C(child, childPath, node.FieldNameForChild(uint32(i)), out)
	}
}

func firstDivergenceDumpV1(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node, path string) *DumpV1Divergence {
	if goNode == nil || cNode == nil {
		return &DumpV1Divergence{
			Path:     path,
			Category: "shape",
			GoValue:  fmt.Sprintf("nil=%v", goNode == nil),
			CValue:   fmt.Sprintf("nil=%v", cNode == nil),
		}
	}
	goType := dumpV1GoType(goNode, goLang)
	cType := cNode.Kind()
	if goType != cType {
		return &DumpV1Divergence{Path: path, Category: "type", GoValue: goType, CValue: cType}
	}
	if goNode.IsNamed() != cNode.IsNamed() {
		return &DumpV1Divergence{
			Path:     path,
			Category: "named",
			GoValue:  fmt.Sprintf("%v", goNode.IsNamed()),
			CValue:   fmt.Sprintf("%v", cNode.IsNamed()),
		}
	}
	if goNode.IsExtra() != cNode.IsExtra() {
		return &DumpV1Divergence{
			Path:     path,
			Category: "extra",
			GoValue:  fmt.Sprintf("%v", goNode.IsExtra()),
			CValue:   fmt.Sprintf("%v", cNode.IsExtra()),
		}
	}
	if goNode.IsMissing() != cNode.IsMissing() {
		return &DumpV1Divergence{
			Path:     path,
			Category: "missing",
			GoValue:  fmt.Sprintf("%v", goNode.IsMissing()),
			CValue:   fmt.Sprintf("%v", cNode.IsMissing()),
		}
	}
	if goNode.HasError() != cNode.HasError() {
		return &DumpV1Divergence{
			Path:     path,
			Category: "error",
			GoValue:  fmt.Sprintf("%v", goNode.HasError()),
			CValue:   fmt.Sprintf("%v", cNode.HasError()),
		}
	}

	goStart := goNode.StartPoint()
	goEnd := goNode.EndPoint()
	cStart := cNode.StartPosition()
	cEnd := cNode.EndPosition()
	if goNode.StartByte() != uint32(cNode.StartByte()) ||
		goNode.EndByte() != uint32(cNode.EndByte()) ||
		goStart.Row != uint32(cStart.Row) ||
		goStart.Column != uint32(cStart.Column) ||
		goEnd.Row != uint32(cEnd.Row) ||
		goEnd.Column != uint32(cEnd.Column) {
		return &DumpV1Divergence{
			Path:     path,
			Category: "range",
			GoValue:  fmt.Sprintf("%d:%d-%d:%d @%d..%d", goStart.Row, goStart.Column, goEnd.Row, goEnd.Column, goNode.StartByte(), goNode.EndByte()),
			CValue:   fmt.Sprintf("%d:%d-%d:%d @%d..%d", cStart.Row, cStart.Column, cEnd.Row, cEnd.Column, cNode.StartByte(), cNode.EndByte()),
		}
	}

	goChildCount := goNode.ChildCount()
	cChildCount := int(cNode.ChildCount())
	if goChildCount != cChildCount {
		return &DumpV1Divergence{
			Path:     path,
			Category: "shape",
			GoValue:  fmt.Sprintf("children=%d", goChildCount),
			CValue:   fmt.Sprintf("children=%d", cChildCount),
		}
	}

	for i := 0; i < goChildCount; i++ {
		goField := goNode.FieldNameForChild(i, goLang)
		cField := cNode.FieldNameForChild(uint32(i))
		goChild := goNode.Child(i)
		cChild := cNode.Child(uint(i))
		nextPath := childPath(path, goTypeOrFallback(goChild, goLang, cChild), i)
		if goField != cField {
			return &DumpV1Divergence{
				Path:     nextPath,
				Category: "field",
				GoValue:  goField,
				CValue:   cField,
			}
		}
		if diff := firstDivergenceDumpV1(goChild, goLang, cChild, nextPath); diff != nil {
			return diff
		}
	}
	return nil
}

func childPath(parentPath, typ string, index int) string {
	name := typ
	if name == "" {
		name = "?"
	}
	return fmt.Sprintf("%s/%s[%d]", parentPath, name, index)
}

func rootPath(typ string) string {
	name := typ
	if name == "" {
		name = "?"
	}
	return "/" + name
}

func goTypeOrFallback(goNode *gotreesitter.Node, goLang *gotreesitter.Language, cNode *sitter.Node) string {
	if goNode != nil && goLang != nil {
		if typ := dumpV1GoType(goNode, goLang); typ != "" {
			return typ
		}
	}
	if cNode != nil {
		if typ := cNode.Kind(); typ != "" {
			return typ
		}
	}
	return "?"
}

func dumpV1GoType(node *gotreesitter.Node, lang *gotreesitter.Language) string {
	if node == nil || lang == nil {
		return ""
	}
	typ := node.Type(lang)
	// The NUL token: C's name for it converts to "" through cgo. Older blobs
	// spell it "\\0"; a fresh conversion carries the NUL byte.
	if typ == "\\0" || typ == "\x00" {
		return ""
	}
	return typ
}
