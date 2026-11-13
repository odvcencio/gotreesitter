package ccore

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestChangedRangesInvisibleRootTerminates(t *testing.T) {
	if order := os.Getenv("CCORE_HIDDEN_ROOT_CHILD"); order != "" {
		leaf := func(visible bool) *Tree {
			h := Handle(inlInline | (uint64(1) << 8) | (uint64(1) << 56))
			if visible {
				h |= Handle(inlVisible | inlNamed)
			}
			c := &cell{}
			c.root.Store(uint64(h))
			return &Tree{c: c}
		}
		hidden, visible := leaf(false), leaf(true)
		a, b := hidden, visible
		if order == "visible-first" {
			a, b = b, a
		}
		if ranges, ok := ChangedRanges(a, b, &Tables{}, nil, nil); ok || len(ranges) != 0 {
			t.Fatalf("hidden root accepted: ok=%t ranges=%v", ok, ranges)
		}
		if ranges, ok := ChangedRanges(visible, visible, &Tables{}, nil, nil); !ok || len(ranges) != 0 {
			t.Fatalf("visible control: ok=%t ranges=%v", ok, ranges)
		}
		return
	}
	for _, order := range []string{"hidden-first", "visible-first"} {
		t.Run(order, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestChangedRangesInvisibleRootTerminates$", "-test.count=1")
			cmd.Env = append(os.Environ(), "CCORE_HIDDEN_ROOT_CHILD="+order)
			out, err := cmd.CombinedOutput()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Fatalf("comparison hung for %s", order)
			}
			if err != nil {
				t.Fatalf("child failed: %v\n%s", err, out)
			}
		})
	}
}
