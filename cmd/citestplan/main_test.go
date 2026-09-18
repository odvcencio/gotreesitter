package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func testWorkflow(t *testing.T) workflow {
	t.Helper()
	var wf workflow
	if err := yaml.Unmarshal([]byte(`jobs:
  race_packages:
    strategy:
      matrix:
        include:
          - name: cmd
            package_filter: '/cmd/'
          - name: query
            package_filter: '/roottest/query$'
          - name: wasm
            package_filter: '/wasm/runtime$'
  race_root_shards:
    steps:
      - run: go test . -race -run "$pattern"
  grammargen_visibility:
    steps:
      - run: go test ./grammargen -race
  selected_store_admission:
    steps:
      - run: GOMAXPROCS=1 go test -race ./internal/parsercorephase0 -count=1
`), &wf); err != nil {
		t.Fatal(err)
	}
	return wf
}

func TestPlanRejectsUnassignedAndRemovedPackages(t *testing.T) {
	const module = "example.org/parser"
	packages := []packageInfo{{ImportPath: module + "/roottest/query"}, {ImportPath: module + "/wasm/runtime"}}
	wf := testWorkflow(t)
	plan, err := buildPlan(module, packages, wf)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(plan["query"], " "); got != packages[0].ImportPath {
		t.Fatalf("query selection = %q", got)
	}
	if got := strings.Join(plan["wasm"], " "); got != packages[1].ImportPath {
		t.Fatalf("wasm selection = %q", got)
	}
	packages = append(packages, packageInfo{ImportPath: module + "/newtests"})
	if _, err := buildPlan(module, packages, wf); err == nil || !strings.Contains(err.Error(), module+"/newtests") {
		t.Fatalf("unassigned package error = %v", err)
	}
	packages = packages[:2]
	job := wf.Jobs["race_packages"]
	job.Strategy.Matrix.Include = job.Strategy.Matrix.Include[:2]
	wf.Jobs["race_packages"] = job
	if _, err := buildPlan(module, packages, wf); err == nil || !strings.Contains(err.Error(), module+"/wasm/runtime") {
		t.Fatalf("removed lane error = %v", err)
	}
}

func TestDedicatedPackagesRequireTheirCommands(t *testing.T) {
	const module = "example.org/parser"
	for _, tc := range []struct{ path, job string }{
		{module, "race_root_shards"},
		{module + "/grammargen", "grammargen_visibility"},
		{module + "/internal/parsercorephase0", "selected_store_admission"},
	} {
		t.Run(tc.job, func(t *testing.T) {
			wf := testWorkflow(t)
			packages := []packageInfo{{ImportPath: tc.path}}
			if _, err := buildPlan(module, packages, wf); err != nil {
				t.Fatal(err)
			}
			delete(wf.Jobs, tc.job)
			if _, err := buildPlan(module, packages, wf); err == nil {
				t.Fatal("missing dedicated job passed")
			}
		})
	}
}

func TestPlanRejectsMalformedLanes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lanes []lane
	}{
		{"empty", nil},
		{"missing name", []lane{{PackageFilter: "/cmd/"}}},
		{"empty filter", []lane{{Name: "all"}}},
		{"bad package filter", []lane{{Name: "cmd", PackageFilter: "["}}},
		{"bad test filter", []lane{{Name: "cmd", PackageFilter: "/cmd/", TestFilter: "["}}},
		{"duplicate name", []lane{{Name: "cmd", PackageFilter: "/cmd/"}, {Name: "cmd", PackageFilter: "/wasm/"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wf := testWorkflow(t)
			job := wf.Jobs["race_packages"]
			job.Strategy.Matrix.Include = tc.lanes
			wf.Jobs["race_packages"] = job
			if _, err := buildPlan("example.org/parser", nil, wf); err == nil {
				t.Fatal("invalid lane plan passed")
			}
		})
	}
}

func TestDecodePackagesIncludesExternalTestsAndRejectsTruncation(t *testing.T) {
	input := `{"ImportPath":"p/a","TestGoFiles":["a_test.go"]}
{"ImportPath":"p/b","XTestGoFiles":["b_test.go"]}
{"ImportPath":"p/c"}`
	packages, err := decodePackages(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 2 || packages[0].ImportPath != "p/a" || packages[1].ImportPath != "p/b" {
		t.Fatalf("test packages = %+v", packages)
	}
	if _, err := decodePackages(strings.NewReader(input + `{"ImportPath":`)); err == nil {
		t.Fatal("truncated package list passed")
	}
}
