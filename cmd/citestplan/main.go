// Command citestplan checks host race-build test packages against the CI workflow.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type packageInfo struct {
	ImportPath   string
	TestGoFiles  []string
	XTestGoFiles []string
}

type lane struct {
	Name          string `yaml:"name"`
	PackageFilter string `yaml:"package_filter"`
	TestFilter    string `yaml:"test_filter"`
}

type job struct {
	Strategy struct {
		Matrix struct {
			Include []lane `yaml:"include"`
		} `yaml:"matrix"`
	} `yaml:"strategy"`
	Steps []struct {
		Run string `yaml:"run"`
	} `yaml:"steps"`
}

type workflow struct {
	Jobs map[string]job `yaml:"jobs"`
}

func main() {
	workflowPath := flag.String("workflow", ".github/workflows/ci.yml", "workflow to inspect")
	selectedLane := flag.String("lane", "", "print test packages for one race lane")
	flag.Parse()
	if err := run(*workflowPath, *selectedLane, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path, selectedLane string, out io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var wf workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return err
	}
	module, err := exec.Command("go", "list", "-m").Output()
	if err != nil {
		return fmt.Errorf("read module: %w", err)
	}
	cmd := exec.Command("go", "list", "-race", "-json", "./...")
	cmd.Stderr = os.Stderr
	data, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("list packages: %w", err)
	}
	packages, err := decodePackages(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		return errors.New("package inventory contains no tests")
	}
	plan, err := buildPlan(strings.TrimSpace(string(module)), packages, wf)
	if err != nil {
		return err
	}
	if selectedLane != "" {
		packages, ok := plan[selectedLane]
		if !ok {
			return fmt.Errorf("unknown race lane %q", selectedLane)
		}
		for _, name := range packages {
			fmt.Fprintln(out, name)
		}
		return nil
	}
	names := make([]string, 0, len(plan))
	for name := range plan {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Fprintf(out, "%s: %d test packages\n", name, len(plan[name]))
	}
	fmt.Fprintln(out, "Host race-build test packages have declared execution lanes.")
	return nil
}

func decodePackages(r io.Reader) ([]packageInfo, error) {
	var packages []packageInfo
	decoder := json.NewDecoder(r)
	for {
		var p packageInfo
		if err := decoder.Decode(&p); err != nil {
			if errors.Is(err, io.EOF) {
				return packages, nil
			}
			return nil, fmt.Errorf("decode package list: %w", err)
		}
		if len(p.TestGoFiles)+len(p.XTestGoFiles) > 0 {
			packages = append(packages, p)
		}
	}
}

func buildPlan(module string, packages []packageInfo, wf workflow) (map[string][]string, error) {
	lanes := wf.Jobs["race_packages"].Strategy.Matrix.Include
	if len(lanes) == 0 {
		return nil, errors.New("race_packages has no execution lanes")
	}
	plan := make(map[string][]string)
	filters := make(map[string]*regexp.Regexp)
	for _, lane := range lanes {
		if lane.Name == "" || lane.PackageFilter == "" {
			return nil, errors.New("race lane requires a name and package_filter")
		}
		if _, exists := filters[lane.Name]; exists {
			return nil, fmt.Errorf("duplicate race lane %q", lane.Name)
		}
		filter, err := regexp.Compile(lane.PackageFilter)
		if err != nil {
			return nil, fmt.Errorf("race lane %q: %w", lane.Name, err)
		}
		if _, err := regexp.Compile(lane.TestFilter); err != nil {
			return nil, fmt.Errorf("race lane %q test filter: %w", lane.Name, err)
		}
		filters[lane.Name] = filter
		plan[lane.Name] = nil
	}
	// These packages execute outside the non-root race matrix.
	// Grammargen remains informational. Do not report it as a blocking gate.
	dedicated := map[string]struct{ job, command string }{
		module:                                {"race_root_shards", "go test . -race"},
		module + "/grammargen":                {"grammargen_visibility", "go test ./grammargen -race"},
		module + "/internal/parsercorephase0": {"selected_store_admission", "go test -race ./internal/parsercorephase0"},
	}
	var missing []string
	for _, p := range packages {
		if owner, ok := dedicated[p.ImportPath]; ok {
			found := false
			for _, step := range wf.Jobs[owner.job].Steps {
				if strings.Contains(step.Run, owner.command) {
					found = true
				}
			}
			if !found {
				return nil, fmt.Errorf("dedicated job %s does not execute %s", owner.job, p.ImportPath)
			}
			continue
		}
		matched := false
		for name, filter := range filters {
			if filter.MatchString(p.ImportPath) {
				plan[name] = append(plan[name], p.ImportPath)
				matched = true
			}
		}
		if !matched {
			missing = append(missing, p.ImportPath)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("test packages have no execution lane:\n  %s", strings.Join(missing, "\n  "))
	}
	for name := range plan {
		sort.Strings(plan[name])
	}
	return plan, nil
}
