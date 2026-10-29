//go:build !grammar_subset

package grammarruntime

import (
	"os"
	"os/exec"
	"reflect"
	"sync"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

// Run each case in a fresh process so other tests cannot initialize the
// process-wide registry first or observe the test's custom registration.
func runFactoryTestInFreshProcess(t *testing.T) bool {
	t.Helper()
	if os.Getenv("GTS_FACTORY_TEST_CHILD") == t.Name() {
		return true
	}
	cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$")
	cmd.Env = append(os.Environ(), "GTS_FACTORY_TEST_CHILD="+t.Name())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("factory subprocess: %v\n%s", err, output)
	}
	return false
}

func TestTokenSourceFactoryConcurrentFirstLookup(t *testing.T) {
	if !runFactoryTestInFreshProcess(t) {
		return
	}
	const workers = 32
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for _, name := range []string{"authzed", "c", "cpp", "java"} {
				first, second := TokenSourceFactory(name), TokenSourceFactory(name)
				if first == nil || second == nil || reflect.ValueOf(first).Pointer() != reflect.ValueOf(second).Pointer() {
					t.Errorf("unstable factory for %s", name)
				}
			}
			if TokenSourceFactory("missing") != nil {
				t.Error("unexpected factory for unknown name")
			}
		}()
	}
	close(start)
	wg.Wait()
}

func TestTokenSourceFactoryPreservesEarlyRegistration(t *testing.T) {
	if !runFactoryTestInFreshProcess(t) {
		return
	}
	custom := func([]byte, *gotreesitter.Language) gotreesitter.TokenSource { return nil }
	registerTokenSourceFactory("c", custom)
	got := TokenSourceFactory("c")
	if got == nil || reflect.ValueOf(got).Pointer() != reflect.ValueOf(custom).Pointer() {
		t.Fatal("first lookup replaced the registered factory")
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				registerTokenSourceFactory("custom", custom)
				if TokenSourceFactory("custom") == nil {
					t.Error("missing custom factory")
				}
			}
		}()
	}
	wg.Wait()
}
