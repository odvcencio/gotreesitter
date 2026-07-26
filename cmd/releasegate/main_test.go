package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlannedMinorFactsUseLosAngelesThursdayAcrossUTCBoundary(t *testing.T) {
	input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
	result := collect(t, input)
	if !result.PolicyFacts.Release.LosAngelesThursday {
		t.Fatal("Los Angeles Thursday fact is false")
	}
	if result.PolicyFacts.Release.VersionIncrement != "next-minor" {
		t.Fatalf("version increment = %q", result.PolicyFacts.Release.VersionIncrement)
	}
}

func TestPlannedMinorFactsMarkOtherWeekdays(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
	}{
		{name: "Wednesday", now: time.Date(2026, 7, 30, 2, 0, 0, 0, time.UTC)},
		{name: "Friday", now: time.Date(2026, 8, 1, 2, 0, 0, 0, time.UTC)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := plannedInput(t, test.now)
			writeReleaseFiles(t, input.RepositoryRoot, input.Version, localDate(t, test.now), input.LatestTag, "### Changed\n\n- Release notes.\n")
			result := collect(t, input)
			if result.PolicyFacts.Release.LosAngelesThursday {
				t.Fatal("Los Angeles Thursday fact is true")
			}
		})
	}
}

func TestUrgentPatchFactsOutsideThursday(t *testing.T) {
	input := urgentInput(t, time.Date(2026, 7, 29, 19, 0, 0, 0, time.UTC))
	result := collect(t, input)
	facts := result.PolicyFacts.Release
	if facts.LosAngelesThursday {
		t.Fatal("Los Angeles Thursday fact is true")
	}
	if facts.VersionIncrement != "next-patch" {
		t.Fatalf("version increment = %q", facts.VersionIncrement)
	}
	if !facts.IncidentPresent || !facts.DelayRationalePresent {
		t.Fatal("urgent patch exception facts are false")
	}
}

func TestUrgentPatchReasonFactsAreIndependent(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*validationInput)
		check  func(releasePolicyFacts) bool
	}{
		{
			name: "missing incident",
			mutate: func(input *validationInput) {
				input.Incident = ""
			},
			check: func(facts releasePolicyFacts) bool {
				return !facts.IncidentPresent && facts.DelayRationalePresent
			},
		},
		{
			name: "missing delay rationale",
			mutate: func(input *validationInput) {
				input.WhyWaitingIsWorse = ""
			},
			check: func(facts releasePolicyFacts) bool {
				return facts.IncidentPresent && !facts.DelayRationalePresent
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := urgentInput(t, time.Date(2026, 7, 29, 19, 0, 0, 0, time.UTC))
			test.mutate(&input)
			result := collect(t, input)
			if !test.check(result.PolicyFacts.Release) {
				t.Fatalf("unexpected exception facts: %+v", result.PolicyFacts.Release)
			}
		})
	}
}

func TestVersionIncrementFactsDoNotChooseARoute(t *testing.T) {
	t.Run("minor increment on urgent input", func(t *testing.T) {
		input := urgentInput(t, time.Date(2026, 7, 29, 19, 0, 0, 0, time.UTC))
		input.Version = "v0.48.0"
		writeReleaseFiles(t, input.RepositoryRoot, input.Version, localDate(t, input.Now), input.LatestTag, "### Changed\n\n- Release notes.\n")
		result := collect(t, input)
		if result.PolicyFacts.Release.VersionIncrement != "next-minor" {
			t.Fatalf("version increment = %q", result.PolicyFacts.Release.VersionIncrement)
		}
	})

	t.Run("patch increment on planned input", func(t *testing.T) {
		input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
		input.Version = "v0.47.1"
		writeReleaseFiles(t, input.RepositoryRoot, input.Version, localDate(t, input.Now), input.LatestTag, "### Fixed\n\n- Release notes.\n")
		result := collect(t, input)
		if result.PolicyFacts.Release.VersionIncrement != "next-patch" {
			t.Fatalf("version increment = %q", result.PolicyFacts.Release.VersionIncrement)
		}
	})
}

func TestSoakFactsPreserveBoundary(t *testing.T) {
	tests := []struct {
		name        string
		soak        time.Duration
		wantSeconds int64
	}{
		{name: "47 hours 59 minutes", soak: 47*time.Hour + 59*time.Minute, wantSeconds: 172740},
		{name: "exactly 48 hours", soak: 48 * time.Hour, wantSeconds: 172800},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
			input.CICompletedAt = input.Now.Add(-test.soak).Format(time.RFC3339)
			result := collect(t, input)
			if got := result.PolicyFacts.Release.SoakSeconds; got != test.wantSeconds {
				t.Fatalf("soak seconds = %d, want %d", got, test.wantSeconds)
			}
		})
	}
}

func TestHostedCheckFactsAreIndependent(t *testing.T) {
	input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
	input.CIEvent = "push"
	input.CISHA = ""
	input.CIConclusion = "failure"
	input.CICompletedAt = input.Now.Add(time.Hour).Format(time.RFC3339)
	result := collect(t, input)
	facts := result.PolicyFacts.Evidence
	if facts.CIExactCandidate || facts.CIFullDispatch || facts.CISuccessful || facts.CICompletionNotFuture {
		t.Fatalf("unexpected hosted check facts: %+v", facts)
	}
}

func TestHostedCheckFullDispatchFactRequiresManualEvent(t *testing.T) {
	input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
	if facts := collect(t, input).PolicyFacts.Evidence; !facts.CIFullDispatch {
		t.Fatalf("manual event facts = %+v", facts)
	}

	input.CIEvent = "push"
	if facts := collect(t, input).PolicyFacts.Evidence; facts.CIFullDispatch {
		t.Fatalf("push event facts = %+v", facts)
	}
}

func TestSummaryIncludesExactHostedRunIdentity(t *testing.T) {
	input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
	summary := collect(t, input).Summary
	for _, expected := range []string{
		input.CIRunID,
		input.CIRunURL,
		input.CIEvent,
		input.CISHA,
		input.CIConclusion,
		input.CICompletedAt,
	} {
		if !strings.Contains(summary, expected) {
			t.Fatalf("summary does not contain %q:\n%s", expected, summary)
		}
	}
}

func TestMutableGitHubFactsReachThePolicy(t *testing.T) {
	input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
	input.MainExactCandidate = false
	input.TagAbsent = false
	input.ReleaseAbsent = false
	result := collect(t, input)
	facts := result.PolicyFacts.Evidence
	if facts.MainExactCandidate || facts.TagAbsent || facts.ReleaseAbsent {
		t.Fatalf("unexpected mutable GitHub facts: %+v", facts)
	}
}

func TestReleaseDocumentFacts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*validationInput)
		check  func(validationResult) bool
	}{
		{
			name: "README mismatch",
			mutate: func(input *validationInput) {
				writeFile(t, filepath.Join(input.RepositoryRoot, "README.md"), "The current release is **v0.47.0**.\n")
			},
			check: func(result validationResult) bool {
				return !result.PolicyFacts.Evidence.ReadmeMatches
			},
		},
		{
			name: "missing changelog section",
			mutate: func(input *validationInput) {
				writeFile(t, filepath.Join(input.RepositoryRoot, "CHANGELOG.md"),
					"[Unreleased]: https://github.com/odvcencio/gotreesitter/compare/v0.48.0...HEAD\n")
			},
			check: func(result validationResult) bool {
				facts := result.PolicyFacts.Evidence
				return !facts.ChangelogSectionPresent && !facts.ChangelogNotesPresent && result.Notes == ""
			},
		},
		{
			name: "bad comparison base",
			mutate: func(input *validationInput) {
				path := filepath.Join(input.RepositoryRoot, "CHANGELOG.md")
				content, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, path, strings.ReplaceAll(string(content), "compare/v0.48.0...HEAD", "compare/v0.47.0...HEAD"))
			},
			check: func(result validationResult) bool {
				return !result.PolicyFacts.Evidence.UnreleasedLinkMatches
			},
		},
		{
			name: "empty notes",
			mutate: func(input *validationInput) {
				writeReleaseFiles(t, input.RepositoryRoot, input.Version, localDate(t, input.Now), input.LatestTag, "")
			},
			check: func(result validationResult) bool {
				facts := result.PolicyFacts.Evidence
				return facts.ChangelogSectionPresent && !facts.ChangelogNotesPresent
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
			test.mutate(&input)
			result := collect(t, input)
			if !test.check(result) {
				t.Fatalf("unexpected release facts: %+v", result.PolicyFacts)
			}
		})
	}
}

func TestMalformedEvidenceStopsFactCollection(t *testing.T) {
	t.Run("invalid version", func(t *testing.T) {
		input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
		input.Version = "0.48.0"
		if _, err := collectReleaseEvidence(input); err == nil {
			t.Fatal("fact collection succeeded with an invalid version")
		}
	})

	t.Run("invalid completion time becomes deny evidence", func(t *testing.T) {
		input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
		input.CICompletedAt = "not-a-time"
		result := collect(t, input)
		if result.PolicyFacts.Evidence.CICompletionNotFuture {
			t.Fatal("invalid completion time produced valid evidence")
		}
	})
}

func TestWriteResultEmitsDeterministicPolicyFactsAndCIRun(t *testing.T) {
	directory := t.TempDir()
	input := plannedInput(t, time.Date(2026, 7, 31, 2, 0, 0, 0, time.UTC))
	result := collect(t, input)
	if err := writeResult(directory, result); err != nil {
		t.Fatalf("write result: %v", err)
	}
	for _, name := range []string{"title.txt", "notes.md", "summary.md", "policy-facts.json", "ci-run.json"} {
		content, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(content) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
	content, err := os.ReadFile(filepath.Join(directory, "policy-facts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded policyFacts
	if err := json.Unmarshal(content, &decoded); err != nil {
		t.Fatalf("decode policy facts: %v", err)
	}
	if decoded.Release.Kind != "planned-minor" || !decoded.Evidence.CIExactCandidate || !decoded.Evidence.CIFullDispatch {
		t.Fatalf("decoded policy facts = %+v", decoded)
	}
	content, err = os.ReadFile(filepath.Join(directory, "ci-run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var ciRun ciRunEvidence
	if err := json.Unmarshal(content, &ciRun); err != nil {
		t.Fatalf("decode CI run evidence: %v", err)
	}
	if ciRun != result.CIRun {
		t.Fatalf("decoded CI run evidence = %+v", ciRun)
	}
}

func collect(t *testing.T, input validationInput) validationResult {
	t.Helper()
	result, err := collectReleaseEvidence(input)
	if err != nil {
		t.Fatalf("collect release evidence: %v", err)
	}
	return result
}

func plannedInput(t *testing.T, now time.Time) validationInput {
	t.Helper()
	root := t.TempDir()
	input := validationInput{
		Version:            "v0.48.0",
		CandidateSHA:       strings.Repeat("a", 40),
		ReleaseKind:        "planned-minor",
		ReceiptURI:         "hypha://m31labs/gotreesitter/release/v0.48.0",
		LatestTag:          "v0.47.0",
		CIRunID:            "29892922471",
		CIRunURL:           "https://github.com/odvcencio/gotreesitter/actions/runs/29892922471",
		CIEvent:            "workflow_dispatch",
		CISHA:              strings.Repeat("a", 40),
		CIConclusion:       "success",
		CICompletedAt:      now.Add(-48 * time.Hour).Format(time.RFC3339),
		MainExactCandidate: true,
		TagAbsent:          true,
		ReleaseAbsent:      true,
		RepositoryRoot:     root,
		Now:                now,
	}
	writeReleaseFiles(t, root, input.Version, localDate(t, now), input.LatestTag, "### Changed\n\n- Release notes.\n")
	return input
}

func urgentInput(t *testing.T, now time.Time) validationInput {
	t.Helper()
	root := t.TempDir()
	input := validationInput{
		Version:            "v0.47.1",
		CandidateSHA:       strings.Repeat("b", 40),
		ReleaseKind:        "urgent-patch",
		ReceiptURI:         "hypha://m31labs/gotreesitter/release/v0.47.1",
		Incident:           "The parser returns a wrong tree.",
		WhyWaitingIsWorse:  "Users receive incorrect parse results.",
		LatestTag:          "v0.47.0",
		CIRunID:            "29892922471",
		CIRunURL:           "https://github.com/odvcencio/gotreesitter/actions/runs/29892922471",
		CIEvent:            "workflow_dispatch",
		CISHA:              strings.Repeat("b", 40),
		CIConclusion:       "success",
		CICompletedAt:      now.Add(-time.Hour).Format(time.RFC3339),
		MainExactCandidate: true,
		TagAbsent:          true,
		ReleaseAbsent:      true,
		RepositoryRoot:     root,
		Now:                now,
	}
	writeReleaseFiles(t, root, input.Version, localDate(t, now), input.LatestTag, "### Fixed\n\n- Urgent correction.\n")
	return input
}

func writeReleaseFiles(t *testing.T, root, version, date, previousTag, notes string) {
	t.Helper()
	number := strings.TrimPrefix(version, "v")
	changelog := "# Changelog\n\n## [Unreleased]\n\n" +
		"## [" + number + "] - " + date + "\n\n" + notes + "\n" +
		"## [" + strings.TrimPrefix(previousTag, "v") + "] - 2026-07-23\n\n- Previous release.\n\n" +
		"[Unreleased]: https://github.com/odvcencio/gotreesitter/compare/" + version + "...HEAD\n" +
		"[" + number + "]: https://github.com/odvcencio/gotreesitter/compare/" + previousTag + "..." + version + "\n"
	writeFile(t, filepath.Join(root, "CHANGELOG.md"), changelog)
	readme := "# gotreesitter\n\nThe current release is **" + version + "**.\n"
	writeFile(t, filepath.Join(root, "README.md"), readme)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func localDate(t *testing.T, value time.Time) string {
	t.Helper()
	location, err := time.LoadLocation(losAngelesZone)
	if err != nil {
		t.Fatal(err)
	}
	return value.In(location).Format("2006-01-02")
}
