package parsercorephase0

import "testing"

func TestNativeProjectionDirectFieldProfile(t *testing.T) {
	for _, test := range []struct {
		name   string
		fields []FieldMapEntry
		want   bool
	}{
		{"empty", nil, true},
		{"distinct", []FieldMapEntry{{FieldID: 1, ChildIndex: 0}, {FieldID: 2, ChildIndex: 64}, {FieldID: 1, ChildIndex: 128}, {FieldID: 2, ChildIndex: 192}, {FieldID: 3, ChildIndex: 255}}, true},
		{"inherited", []FieldMapEntry{{FieldID: 1, Inherited: true}}, false},
		{"multiple_fields", []FieldMapEntry{{FieldID: 1, ChildIndex: 128}, {FieldID: 2, ChildIndex: 128}}, false},
		{"duplicate", []FieldMapEntry{{FieldID: 1, ChildIndex: 255}, {FieldID: 1, ChildIndex: 255}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := nativeProjectionDirectSingleFields(test.fields); got != test.want {
				t.Fatalf("supported=%t want %t", got, test.want)
			}
		})
	}
}

func TestNativeProjectionFieldAdmissionResetAndRollback(t *testing.T) {
	c := newTinyCore(t, 0)
	if !c.NativeProjectionDirectFieldsSupported() || (*Core)(nil).NativeProjectionDirectFieldsSupported() {
		t.Fatal("initial admission")
	}
	leaf, err := c.appendSubtree(subtreeRecord{terminal: true, symbol: 1, startByte: 0, endByte: 1}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.appendSubtree(subtreeRecord{symbol: 2, startByte: 0, endByte: 1}, []SubtreeID{leaf}, []FieldMapEntry{{FieldID: 1}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !c.NativeProjectionDirectFieldsSupported() {
		t.Fatal("direct field lost admission")
	}
	mark := c.mark()
	_, err = c.appendSubtree(subtreeRecord{symbol: 2, startByte: 0, endByte: 1}, []SubtreeID{leaf}, []FieldMapEntry{{FieldID: 1, Inherited: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.NativeProjectionDirectFieldsSupported() {
		t.Fatal("inherited field authorized streaming")
	}
	c.restore(mark)
	if c.NativeProjectionDirectFieldsSupported() {
		t.Fatal("rollback removed conservative admission boundary")
	}
	if err := c.Reset(); err != nil {
		t.Fatal(err)
	}
	if !c.NativeProjectionDirectFieldsSupported() {
		t.Fatal("reset retained a prior generation boundary")
	}
}
