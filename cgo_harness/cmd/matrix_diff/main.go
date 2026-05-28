// matrix_diff compares two REAL_CORPUS_BENCH_REPORT.md outputs from the
// pinned bench matrix and prints a per-language ratio drift table.
// Useful for verifying session-over-session perf changes against the
// honest matrix harness (commit d43d58a — --cpuset-cpus pinning).
//
// Usage:
//   go run ./cgo_harness/cmd/matrix_diff <baseline.md> <current.md>
//
// Parses the markdown summary table — looking for lines like:
//   | python | 25.440ms | 10.524ms | 2.42x | ... |
package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type entry struct {
	name    string
	goWall  float64 // nanoseconds
	cWall   float64
	ratio   float64
}

var rowRe = regexp.MustCompile(`^\|\s*([a-zA-Z_][a-zA-Z_0-9]*)\s*\|\s*([0-9.]+)(ms|us|s|ns)\s*\|\s*([0-9.]+)(ms|us|s|ns)\s*\|\s*([0-9.]+)x\s*\|`)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: matrix_diff <baseline.md> <current.md>")
		os.Exit(2)
	}

	base, err := parseReport(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "load baseline: %v\n", err)
		os.Exit(1)
	}
	cur, err := parseReport(os.Args[2])
	if err != nil {
		fmt.Fprintf(os.Stderr, "load current: %v\n", err)
		os.Exit(1)
	}

	all := allLangs(base, cur)
	fmt.Printf("%-12s %12s %12s %10s   %12s %12s %10s   %10s %10s\n",
		"language", "base Go", "cur Go", "Go drift", "base C", "cur C", "C drift", "base/cur ratio", "drift")
	fmt.Println(strings.Repeat("-", 124))

	for _, name := range all {
		b, hasB := base[name]
		c, hasC := cur[name]

		switch {
		case !hasB && hasC:
			fmt.Printf("%-12s %12s %12s %10s   %12s %12s %10s   %10s %10s\n",
				name, "(new)", dur(c.goWall), "—",
				"(new)", dur(c.cWall), "—",
				fmt.Sprintf("(new)→%.2fx", c.ratio), "—")
		case hasB && !hasC:
			fmt.Printf("%-12s %12s %12s %10s   %12s %12s %10s   %10s %10s\n",
				name, dur(b.goWall), "(missing)", "—",
				dur(b.cWall), "(missing)", "—",
				fmt.Sprintf("%.2fx→(?)", b.ratio), "—")
		default:
			fmt.Printf("%-12s %12s %12s %+9.1f%%   %12s %12s %+9.1f%%   %4.2fx → %.2fx %+9.1f%%\n",
				name,
				dur(b.goWall), dur(c.goWall), pctDiff(b.goWall, c.goWall),
				dur(b.cWall), dur(c.cWall), pctDiff(b.cWall, c.cWall),
				b.ratio, c.ratio, pctDiff(b.ratio, c.ratio),
			)
		}
	}
}

func parseReport(path string) (map[string]entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]entry)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		m := rowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]
		// Skip table header row that has "Language" as text — case-sensitive
		// won't match because regex is lowercase, but be defensive.
		if name == "language" || name == "Language" {
			continue
		}

		goVal, err := parseDur(m[2], m[3])
		if err != nil {
			continue
		}
		cVal, err := parseDur(m[4], m[5])
		if err != nil {
			continue
		}
		ratio, err := strconv.ParseFloat(m[6], 64)
		if err != nil {
			continue
		}
		out[name] = entry{name: name, goWall: goVal, cWall: cVal, ratio: ratio}
	}
	return out, scanner.Err()
}

func parseDur(numStr, unit string) (float64, error) {
	n, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return 0, err
	}
	switch unit {
	case "ns":
		return n, nil
	case "us":
		return n * 1e3, nil
	case "ms":
		return n * 1e6, nil
	case "s":
		return n * 1e9, nil
	}
	return 0, fmt.Errorf("unknown unit %q", unit)
}

func allLangs(base, cur map[string]entry) []string {
	seen := make(map[string]struct{})
	for k := range base {
		seen[k] = struct{}{}
	}
	for k := range cur {
		seen[k] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		bi, hi := base[out[i]]
		bj, hj := base[out[j]]
		if hi && hj {
			return bi.goWall < bj.goWall
		}
		if !hi && hj {
			return false
		}
		if hi && !hj {
			return true
		}
		return out[i] < out[j]
	})
	return out
}

func dur(ns float64) string {
	if ns == 0 || math.IsNaN(ns) {
		return "—"
	}
	switch {
	case ns >= 1e9:
		return fmt.Sprintf("%.3fs", ns/1e9)
	case ns >= 1e6:
		return fmt.Sprintf("%.2fms", ns/1e6)
	case ns >= 1e3:
		return fmt.Sprintf("%.2fus", ns/1e3)
	default:
		return fmt.Sprintf("%.0fns", ns)
	}
}

func pctDiff(base, cur float64) float64 {
	if base == 0 || math.IsNaN(base) {
		return 0
	}
	return 100.0 * (cur - base) / base
}
