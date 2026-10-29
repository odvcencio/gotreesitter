//go:build !grammar_subset || grammar_subset_vhdl

package grammarruntime

import (
	"sync"
	"testing"
)

func TestVhdlTrieConcurrentInitialization(t *testing.T) {
	const callers = 32
	tries := make([]*vhdlTrie, callers)
	var ready sync.WaitGroup
	ready.Add(callers)
	start := make(chan struct{})
	var done sync.WaitGroup
	done.Add(callers)
	for i := range tries {
		go func(i int) {
			defer done.Done()
			ready.Done()
			<-start
			tries[i] = vhdlTrieForScanner()
		}(i)
	}
	ready.Wait()
	close(start)
	done.Wait()
	if tries[0] == nil || tries[0].root == nil {
		t.Fatal("VHDL trie was not initialized")
	}
	for i, trie := range tries {
		if trie != tries[0] {
			t.Fatalf("caller %d received a different trie", i)
		}
	}
}
