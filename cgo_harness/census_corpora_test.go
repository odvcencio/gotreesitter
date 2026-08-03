//go:build cgo && treesitter_c_parity && (gts_derivation_set_census || gts_merge_census)

package cgoharness

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// Shared census corpora.
//
// Stage D0 of spec.derivation-set-equivalence.v1 (the derivation-set
// differential) and stage M0 of spec.merge-time-election.v1 (the merge-event
// census) measure the SAME sources, so the two censuses share one
// denominator. This file holds that corpus definition, and each census
// compiles it behind its own build tag.

// derivationSetCensusLanguage is one language's differential corpus.
type derivationSetCensusLanguage struct {
	Name        string
	CName       string
	Language    func() *gotreesitter.Language
	Constructed func() []a3CertificationSweepSource
	RealDir     string
}

// derivationSetCensusLanguages mirrors the five A3 certification sweeps
// (apex, perl, ada, kotlin, python) source for source, so the baseline
// census denominator equals the sweeps' own denominator on the same host.
func derivationSetCensusLanguages() []derivationSetCensusLanguage {
	return []derivationSetCensusLanguage{
		{
			Name: "apex", CName: "apex", Language: grammars.ApexLanguage,
			Constructed: func() []a3CertificationSweepSource {
				sources := []a3CertificationSweepSource{{Name: "smoke_sample", Source: []byte(grammars.ParseSmokeSample("apex"))}}
				sources = append(sources, apexA3TiedElectionWitnesses()...)
				return append(sources, apexA3AdversarialSources()...)
			},
		},
		{
			Name: "perl", CName: "perl", Language: grammars.PerlLanguage,
			Constructed: func() []a3CertificationSweepSource {
				sources := []a3CertificationSweepSource{{Name: "smoke_sample", Source: []byte(grammars.ParseSmokeSample("perl"))}}
				return append(sources, perlA3AdversarialSources()...)
			},
			RealDir: filepath.Join("corpus_real", "perl"),
		},
		{
			Name: "ada", CName: "ada", Language: grammars.AdaLanguage,
			Constructed: func() []a3CertificationSweepSource {
				sources := []a3CertificationSweepSource{{Name: "smoke_sample", Source: []byte(grammars.ParseSmokeSample("ada"))}}
				sources = append(sources, adaA3TiedElectionWitnesses()...)
				return append(sources, adaA3AdversarialSources()...)
			},
		},
		{
			Name: "kotlin", CName: "kotlin", Language: grammars.KotlinLanguage,
			Constructed: func() []a3CertificationSweepSource {
				sources := []a3CertificationSweepSource{{Name: "smoke_sample", Source: []byte(grammars.ParseSmokeSample("kotlin"))}}
				return append(sources, kotlinA3AdversarialSources()...)
			},
			RealDir: filepath.Join("corpus_real", "kotlin"),
		},
		{
			Name: "python", CName: "python", Language: grammars.PythonLanguage,
			Constructed: func() []a3CertificationSweepSource {
				sources := []a3CertificationSweepSource{{Name: "smoke_sample", Source: []byte(grammars.ParseSmokeSample("python"))}}
				return append(sources, pythonA3AdversarialSources()...)
			},
			RealDir: filepath.Join("corpus_real", "python"),
		},
	}
}

// derivationSetLoadRealCorpus reads a real-corpus directory when it exists.
// Unlike a3LoadRealCorpusDir it never skips the caller: the baseline census
// pins its constructed-source numbers, which every host reproduces, and
// reports the real-corpus numbers separately with their file count.
func derivationSetLoadRealCorpus(t *testing.T, dir string) []a3CertificationSweepSource {
	t.Helper()
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read corpus dir %s: %v", dir, err)
	}
	var out []a3CertificationSweepSource
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read corpus file %s: %v", path, err)
		}
		out = append(out, a3CertificationSweepSource{Name: entry.Name(), Source: data})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
