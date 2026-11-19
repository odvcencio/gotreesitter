package ccore_test

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

func TestForeignBindingReleasedWhileWaiting(t *testing.T) {
	g := goldenNamed(t, "json-object")
	other := *g.Tables
	p := mustParser(t, ccore.Config{Tables: g.Tables})
	q := mustParser(t, ccore.Config{Tables: &other})
	old, stop := p.Parse(g.Src)
	if stop != "" || old == nil {
		t.Fatal(stop)
	}
	ccore.LockLease(old)
	waiting := make(chan struct{}, 1)
	ccore.SetTestHook(old, func(point string) {
		if point == "parse-wait" {
			select {
			case waiting <- struct{}{}:
			default:
			}
		}
	})
	done := make(chan string, 1)
	go func() {
		n, stop := q.ParseWith(g.Src, old)
		if n != nil {
			n.Release()
			stop = "returned tree"
		}
		done <- stop
	}()
	<-waiting
	old.Release()
	ccore.UnlockLease(old)
	if stop := <-done; stop != "old tree released" {
		t.Fatalf("released while waiting: %q", stop)
	}
	waitFor(t, "store ended", func() bool { return ccore.Ended(old) })
}
