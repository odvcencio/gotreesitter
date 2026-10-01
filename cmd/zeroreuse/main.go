// Command zeroreuse diagnoses the pinned first edit, one grammar per process.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

func main() {
	name := flag.String("language", "", "one grammar from the pinned sample manifest")
	list := flag.Bool("list", false, "list grammar names without parsing")
	flag.Parse()
	if *list {
		var names []string
		for _, entry := range grammars.AllLanguages() {
			names = append(names, entry.Name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Println(name)
		}
		return
	}
	if err := run(*name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(name string) error {
	entry := grammars.DetectLanguageByName(name)
	if entry == nil {
		return fmt.Errorf("unknown language %q", name)
	}
	data, err := os.ReadFile("internal/benchfixtures/real_corpus.json")
	if err != nil {
		return err
	}
	var manifest struct {
		Entries []struct {
			Language      string `json:"language"`
			Role          string `json:"role"`
			Path          string `json:"committed_path"`
			SHA256        string `json:"sha256"`
			SessionSHA256 string `json:"session_sha256"`
		}
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	var source []byte
	for _, item := range manifest.Entries {
		if item.Language != name || item.Role != "sample" {
			continue
		}
		if item.Path == "" {
			source = map[string][]byte{"csv": []byte("name,value\nexample,1\n"), "enforce": []byte("class Example { int value; }\n")}[name]
		} else {
			source, err = os.ReadFile(filepath.Join("internal/benchfixtures", item.Path))
			if err != nil {
				return err
			}
			if fmt.Sprintf("%x", sha256.Sum256(source)) != item.SHA256 || benchfixtures.EditingSessionSHA256(source) != item.SessionSHA256 {
				return fmt.Errorf("%s sample or edit-session digest changed", name)
			}
		}
		break
	}
	if len(source) == 0 {
		return fmt.Errorf("%s has no sample", name)
	}
	lang := entry.Language()
	p := gts.NewParser(lang)
	p.SetAdmissionCandidateRoute(false)
	old, err := p.Parse(source)
	if err != nil {
		return err
	}
	defer old.Release()
	baseError := old.RootNode().HasError()
	baseRootError := old.RootNode().IsError()
	step := benchfixtures.EditingSession(source)[0]
	old.Edit(step.Edit)
	next, profile, err := p.ParseIncrementalProfiled(step.Source, old)
	if err != nil {
		return err
	}
	defer next.Release()
	fresh, err := p.Parse(step.Source)
	if err != nil {
		return err
	}
	defer fresh.Release()
	got, err := benchfixtures.InspectGoTree(next.RootNode(), lang)
	if err != nil {
		return err
	}
	want, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		return err
	}
	scanner := lang.ExternalScanner
	stateless, checkpointed, reusable := false, false, false
	if s, ok := scanner.(gts.StatelessExternalScanner); ok {
		stateless = s.ExternalScannerIsStateless()
	}
	if s, ok := scanner.(gts.CheckpointedExternalScanner); ok {
		checkpointed = s.UsesExternalScannerCheckpoints()
	}
	if s, ok := scanner.(gts.IncrementalReuseExternalScanner); ok {
		reusable = s.SupportsIncrementalReuse()
	}
	row := struct {
		Language          string                      `json:"language"`
		SourceSHA256      string                      `json:"source_sha256"`
		InputBytes        int                         `json:"input_bytes"`
		Edit              gts.InputEdit               `json:"edit"`
		Scanner           string                      `json:"scanner"`
		Stateless         bool                        `json:"stateless"`
		Checkpointed      bool                        `json:"checkpointed"`
		ScannerReusable   bool                        `json:"scanner_reusable"`
		BaseError         bool                        `json:"base_error"`
		BaseRootError     bool                        `json:"base_root_error"`
		IncrementalSHA256 string                      `json:"incremental_sha256"`
		FreshSHA256       string                      `json:"fresh_sha256"`
		EqualFresh        bool                        `json:"equal_fresh"`
		Profile           gts.IncrementalParseProfile `json:"profile"`
	}{name, fmt.Sprintf("%x", sha256.Sum256(source)), len(source), step.Edit, fmt.Sprintf("%T", scanner), stateless, checkpointed, reusable, baseError, baseRootError, got.SHA256, want.SHA256, got.SHA256 == want.SHA256, profile}
	if err := json.NewEncoder(os.Stdout).Encode(row); err != nil {
		return err
	}
	if !row.EqualFresh {
		return fmt.Errorf("%s incremental tree differs from fresh", name)
	}
	return nil
}
