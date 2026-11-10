package bind

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/ccoretest"
)

func reset() {
	mu.Lock()
	adapter = nil
	mu.Unlock()
}

func golden(t *testing.T, name string) *ccoretest.Golden {
	t.Helper()
	g, err := ccoretest.Read(filepath.Join("..", "testdata", "golden"), filepath.Join("..", "testdata", "tables"), name+".json")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

// stub registers an adapter whose language is a *ccoretest.Golden.
func stub(t *testing.T) {
	t.Helper()
	reset()
	t.Cleanup(reset)
	Register(Adapter{
		Config: func(lang any) (ccore.Config, error) {
			g, ok := lang.(*ccoretest.Golden)
			if !ok {
				return ccore.Config{}, errors.New("stub: not a golden")
			}
			return ccore.Config{Tables: g.Tables}, nil
		},
		CheckTables: func(lang any) (int, int, error) { return 7, 0, nil },
	})
}

func TestNoAdapter(t *testing.T) {
	reset()
	if _, err := New(nil); !errors.Is(err, ErrNoAdapter) {
		t.Fatalf("New without adapter: %v", err)
	}
	if _, _, err := CheckTables(nil); !errors.Is(err, ErrNoAdapter) {
		t.Fatalf("CheckTables without adapter: %v", err)
	}
}

func TestRegisterTwicePanics(t *testing.T) {
	stub(t)
	defer func() {
		if recover() == nil {
			t.Fatal("second Register did not panic")
		}
	}()
	Register(Adapter{Config: func(any) (ccore.Config, error) { return ccore.Config{}, nil }, CheckTables: func(any) (int, int, error) { return 0, 0, nil }})
}

func TestRegisterNeedsBothFunctions(t *testing.T) {
	reset()
	t.Cleanup(reset)
	defer func() {
		if recover() == nil {
			t.Fatal("Register accepted an adapter with a nil function")
		}
	}()
	Register(Adapter{Config: func(any) (ccore.Config, error) { return ccore.Config{}, nil }})
}

// TestNewRefusesBadTables: ccore.NewParser's refusal comes back through New
// with the cause still testable.
func TestNewRefusesBadTables(t *testing.T) {
	stub(t)
	g := golden(t, "json-object")
	bad := *g.Tables
	bad.InitialState = 0
	if _, err := New(&ccoretest.Golden{Tables: &bad}); !errors.Is(err, ccore.ErrInitialState) {
		t.Fatalf("initial state 0: %v", err)
	}
	if _, err := New("not a language"); err == nil || !strings.Contains(err.Error(), "stub") {
		t.Fatalf("adapter error not passed on: %v", err)
	}
}

func TestFuncScanner(t *testing.T) {
	closed := false
	var s ccore.ExtScanner = &FuncScanner{
		ScanFn:  func([]byte, int, uint32, uint32, []byte, []bool, []byte, *ccore.ExtScan) bool { return true },
		CloseFn: func() { closed = true },
	}
	if !s.Scan(nil, 0, 0, 0, nil, nil, nil, &ccore.ExtScan{}) {
		t.Fatal("ScanFn result not passed on")
	}
	s.Close()
	if !closed {
		t.Fatal("CloseFn not called")
	}
}
