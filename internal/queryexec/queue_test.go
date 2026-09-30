package queryexec

import "testing"

func TestCaptureStreamOrder(t *testing.T) {
	var queue Queue[string]
	for _, e := range []Entry[string]{
		{Start: 10, Pattern: 0, Sequence: 3, Value: "later"},
		{Start: 1, Pattern: 1, Sequence: 0, Value: "other pattern"},
		{Start: 1, Pattern: 0, Sequence: 2, Value: "second match"},
		{Start: 1, Pattern: 0, Sequence: 1, Value: "first match"},
	} {
		queue.Push(e)
	}
	for _, want := range []string{"first match", "second match", "other pattern", "later"} {
		if got := queue.Pop().Value; got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	}
	for _, e := range queue.Entries[:cap(queue.Entries)] {
		if e.Value != "" {
			t.Fatal("queue retained a consumed stream")
		}
	}
}
