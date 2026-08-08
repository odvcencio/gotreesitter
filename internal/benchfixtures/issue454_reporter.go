package benchfixtures

import (
	"bytes"
	"fmt"
)

const (
	issue454ReporterPythonTemplate     = "def f%d(a, b):\n    x%d = a + b\n    print(\"f%d\", x%d)\n    return x%d\n\n"
	issue454ReporterJavaScriptTemplate = "function fn%d(a, b) {\n\tvar x%d = a + b;\n\treturn x%d;\n}\n\n"
	issue454ReporterTypeScriptTemplate = "function fn%d(a: number, b: number): number {\n\tconst x%d = a + b;\n\treturn x%d;\n}\n\n"
)

// Issue454ReporterPythonSource builds the Python generator published in issue 454.
func Issue454ReporterPythonSource(targetBytes int) []byte {
	var source bytes.Buffer
	for i := 0; source.Len() < targetBytes; i++ {
		fmt.Fprintf(&source, issue454ReporterPythonTemplate, i, i, i, i, i)
	}
	return source.Bytes()
}

// Issue454ReporterJavaScriptSource builds the JavaScript generator published in issue 454.
func Issue454ReporterJavaScriptSource(targetBytes int) []byte {
	var source bytes.Buffer
	for i := 0; source.Len() < targetBytes; i++ {
		fmt.Fprintf(&source, issue454ReporterJavaScriptTemplate, i, i, i)
	}
	return source.Bytes()
}

// Issue454ReporterTypeScriptSource builds the TypeScript generator published in issue 454.
func Issue454ReporterTypeScriptSource(targetBytes int) []byte {
	var source bytes.Buffer
	for i := 0; source.Len() < targetBytes; i++ {
		fmt.Fprintf(&source, issue454ReporterTypeScriptTemplate, i, i, i)
	}
	return source.Bytes()
}

// Issue454ReporterEditClass names one reporter edit operation.
type Issue454ReporterEditClass string

const (
	Issue454ReporterReplace Issue454ReporterEditClass = "replace"
	Issue454ReporterInsert  Issue454ReporterEditClass = "insert"
	Issue454ReporterDelete  Issue454ReporterEditClass = "delete"
)

// Issue454ReporterPoint stores a zero-based source point.
type Issue454ReporterPoint struct {
	Row    uint32
	Column uint32
}

// Issue454ReporterEdit records one exact reporter edit and its edited source.
type Issue454ReporterEdit struct {
	Class       Issue454ReporterEditClass
	Source      []byte
	StartByte   uint32
	OldEndByte  uint32
	NewEndByte  uint32
	StartPoint  Issue454ReporterPoint
	OldEndPoint Issue454ReporterPoint
	NewEndPoint Issue454ReporterPoint
}

// BuildIssue454ReporterEdit applies one reporter edit to the first marker byte.
func BuildIssue454ReporterEdit(
	source []byte,
	marker string,
	class Issue454ReporterEditClass,
) (Issue454ReporterEdit, error) {
	if marker == "" {
		return Issue454ReporterEdit{}, fmt.Errorf("issue 454 marker is empty")
	}
	site := bytes.Index(source, []byte(marker))
	if site < 0 {
		return Issue454ReporterEdit{}, fmt.Errorf("issue 454 marker %q is absent", marker)
	}
	point := issue454ReporterPointAt(source, site)
	result := Issue454ReporterEdit{
		Class:      class,
		StartByte:  uint32(site),
		StartPoint: point,
	}
	switch class {
	case Issue454ReporterReplace:
		result.Source = append([]byte(nil), source...)
		result.Source[site]++
		result.OldEndByte = uint32(site + 1)
		result.NewEndByte = uint32(site + 1)
		result.OldEndPoint = Issue454ReporterPoint{Row: point.Row, Column: point.Column + 1}
		result.NewEndPoint = result.OldEndPoint
	case Issue454ReporterInsert:
		result.Source = make([]byte, 0, len(source)+1)
		result.Source = append(result.Source, source[:site]...)
		result.Source = append(result.Source, source[site])
		result.Source = append(result.Source, source[site:]...)
		result.OldEndByte = uint32(site)
		result.NewEndByte = uint32(site + 1)
		result.OldEndPoint = point
		result.NewEndPoint = Issue454ReporterPoint{Row: point.Row, Column: point.Column + 1}
	case Issue454ReporterDelete:
		result.Source = make([]byte, 0, len(source)-1)
		result.Source = append(result.Source, source[:site]...)
		result.Source = append(result.Source, source[site+1:]...)
		result.OldEndByte = uint32(site + 1)
		result.NewEndByte = uint32(site)
		result.OldEndPoint = Issue454ReporterPoint{Row: point.Row, Column: point.Column + 1}
		result.NewEndPoint = point
	default:
		return Issue454ReporterEdit{}, fmt.Errorf("unknown issue 454 edit class %q", class)
	}
	return result, nil
}

func issue454ReporterPointAt(source []byte, offset int) Issue454ReporterPoint {
	var point Issue454ReporterPoint
	for _, value := range source[:offset] {
		if value == '\n' {
			point.Row++
			point.Column = 0
			continue
		}
		point.Column++
	}
	return point
}
