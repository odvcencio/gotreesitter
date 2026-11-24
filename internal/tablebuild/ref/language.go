package ref

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
	"sort"

	gts "github.com/odvcencio/gotreesitter"
)

// L6Ignored lists the exported Language fields that the L6 records leave out,
// each with the reason. Everything else is compared.
var L6Ignored = map[string]string{
	"ExternalScanner": "a Go value the grammar registry attaches; not table data and not in a blob",
}

// l6Skipped holds the struct indices of the Language fields that L6Ignored
// names, so the records skip a field by its index.
var l6Skipped = func() map[int]bool {
	t := reflect.TypeOf((*gts.Language)(nil)).Elem()
	skipped := map[int]bool{}
	for field := range L6Ignored {
		sf, ok := t.FieldByName(field)
		if !ok {
			panic("ref: L6Ignored names no Language field " + field)
		}
		skipped[sf.Index[0]] = true
	}
	return skipped
}()

// languageRecords writes one L6 record per exported field of the Language, in
// struct order: the field name and the digest of a canonical text of its
// value (see writeValue).
func languageRecords(lang *gts.Language) []string {
	v := reflect.ValueOf(lang).Elem()
	t := v.Type()
	var out []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() || l6Skipped[i] {
			continue
		}
		h := sha256.New()
		writeValue(h, v.Field(i))
		out = append(out, line("L6", "field", f.Name, hex.EncodeToString(h.Sum(nil)[:8])))
	}
	return out
}

// writeValue writes a canonical text of v: a nil slice or map equals an empty
// one (gob decodes empty slices as nil), map keys are sorted, pointers are
// followed, and an interface is written by its dynamic type only.
func writeValue(w io.Writer, v reflect.Value) {
	switch v.Kind() {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		fmt.Fprintf(w, "%v,", v.Interface())
	case reflect.String:
		fmt.Fprintf(w, "%q,", v.String())
	case reflect.Slice, reflect.Array:
		fmt.Fprintf(w, "[%d:", v.Len())
		for i := 0; i < v.Len(); i++ {
			writeValue(w, v.Index(i))
		}
		io.WriteString(w, "]")
	case reflect.Map:
		keys := v.MapKeys()
		sort.Slice(keys, func(a, b int) bool { return fmt.Sprint(keys[a].Interface()) < fmt.Sprint(keys[b].Interface()) })
		fmt.Fprintf(w, "{%d:", len(keys))
		for _, k := range keys {
			writeValue(w, k)
			writeValue(w, v.MapIndex(k))
		}
		io.WriteString(w, "}")
	case reflect.Pointer:
		if v.IsNil() {
			io.WriteString(w, "nil,")
			return
		}
		io.WriteString(w, "&")
		writeValue(w, v.Elem())
	case reflect.Interface:
		if v.IsNil() {
			io.WriteString(w, "nil,")
			return
		}
		fmt.Fprintf(w, "<%s>,", v.Elem().Type())
	case reflect.Struct:
		io.WriteString(w, "(")
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			if t.Field(i).IsExported() {
				writeValue(w, v.Field(i))
			}
		}
		io.WriteString(w, ")")
	default:
		fmt.Fprintf(w, "<%s>,", v.Kind())
	}
}
