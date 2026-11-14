package bind_test

import (
	"reflect"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
)

func TestScannerHookCannotBeOverwritten(t *testing.T) {
	newHook := func(value bool) ccore.ExtScanner {
		return bind.NewFuncScanner(func([]byte, int, uint32, uint32, []byte, []bool, []byte, *ccore.ExtScan) bool { return value }, func() {})
	}
	hook, other := newHook(true), newHook(false)
	v := reflect.ValueOf(hook)
	if !v.Comparable() || hook == other {
		t.Fatal("distinct hooks must be comparable and distinct")
	}
	if v.Kind() != reflect.Struct || v.CanSet() {
		t.Fatalf("settable hook: %v", v.Kind())
	}
	ptr := v.Field(0)
	if ptr.Kind() != reflect.Pointer || ptr.CanSet() || ptr.Elem().CanSet() {
		t.Fatal("private state can be overwritten")
	}
	for i := 0; i < ptr.Elem().NumField(); i++ {
		if ptr.Elem().Field(i).CanSet() {
			t.Fatal("callback can be overwritten")
		}
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("reflection unexpectedly replaced hook")
			}
		}()
		v.Set(reflect.ValueOf(other))
	}()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("reflection unexpectedly replaced private state")
			}
		}()
		ptr.Elem().Set(reflect.ValueOf(other).Field(0).Elem())
	}()
	func() {
		defer func() {
			if recover() == nil {
				t.Error("reflection unexpectedly replaced callback")
			}
		}()
		ptr.Elem().Field(0).Set(reflect.ValueOf(other).Field(0).Elem().Field(0))
	}()
	if !hook.Scan(nil, 0, 0, 0, nil, nil, nil, &ccore.ExtScan{}) {
		t.Fatal("callback was replaced")
	}
}
