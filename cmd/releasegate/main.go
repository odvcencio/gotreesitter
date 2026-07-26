package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const losAngelesZone = "America/Los_Angeles"

var (
	releaseVersionPattern = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)
	shaPattern            = regexp.MustCompile(`^[0-9a-f]{40}$`)
)

type releaseVersion struct {
	major int
	minor int
	patch int
}

type validationInput struct {
	Version            string
	CandidateSHA       string
	ReleaseKind        string
	ReceiptURI         string
	Incident           string
	WhyWaitingIsWorse  string
	LatestTag          string
	CIRunID            string
	CIRunURL           string
	CIEvent            string
	CISHA              string
	CIConclusion       string
	CICompletedAt      string
	MainExactCandidate bool
	TagAbsent          bool
	ReleaseAbsent      bool
	RepositoryRoot     string
	Now                time.Time
}

type releasePolicyFacts struct {
	Kind                  string `json:"kind"`
	VersionIncrement      string `json:"version_increment"`
	LosAngelesThursday    bool   `json:"los_angeles_thursday"`
	SoakSeconds           int64  `json:"soak_seconds"`
	IncidentPresent       bool   `json:"incident_present"`
	DelayRationalePresent bool   `json:"delay_rationale_present"`
}

type evidencePolicyFacts struct {
	MainExactCandidate      bool `json:"main_exact_candidate"`
	CIExactCandidate        bool `json:"ci_exact_candidate"`
	CIFullDispatch          bool `json:"ci_full_dispatch"`
	CISuccessful            bool `json:"ci_successful"`
	CICompletionNotFuture   bool `json:"ci_completion_not_future"`
	TagAbsent               bool `json:"tag_absent"`
	ReleaseAbsent           bool `json:"release_absent"`
	ReceiptValid            bool `json:"receipt_valid"`
	ChangelogSectionPresent bool `json:"changelog_section_present"`
	ChangelogNotesPresent   bool `json:"changelog_notes_present"`
	ChangelogDateMatches    bool `json:"changelog_date_matches"`
	ReadmeMatches           bool `json:"readme_matches"`
	UnreleasedLinkMatches   bool `json:"unreleased_link_matches"`
	VersionLinkMatches      bool `json:"version_link_matches"`
}

type policyFacts struct {
	Release  releasePolicyFacts  `json:"release"`
	Evidence evidencePolicyFacts `json:"evidence"`
}

type ciRunEvidence struct {
	RunID       string `json:"run_id"`
	URL         string `json:"url"`
	Event       string `json:"event"`
	SHA         string `json:"sha"`
	Conclusion  string `json:"conclusion"`
	CompletedAt string `json:"completed_at"`
}

type validationResult struct {
	Title       string
	Notes       string
	Summary     string
	PolicyFacts policyFacts
	CIRun       ciRunEvidence
}

func main() {
	var input validationInput
	var outputDirectory string

	flag.StringVar(&input.Version, "version", "", "release version")
	flag.StringVar(&input.CandidateSHA, "candidate-sha", "", "release candidate commit")
	flag.StringVar(&input.ReleaseKind, "release-kind", "", "release route")
	flag.StringVar(&input.ReceiptURI, "receipt-uri", "", "Hyphae release receipt")
	flag.StringVar(&input.Incident, "incident", "", "urgent patch incident")
	flag.StringVar(&input.WhyWaitingIsWorse, "why-waiting-is-worse", "", "urgent patch delay rationale")
	flag.StringVar(&input.LatestTag, "latest-tag", "", "latest stable tag")
	flag.StringVar(&input.CIRunID, "ci-run-id", "", "hosted check run identifier")
	flag.StringVar(&input.CIRunURL, "ci-run-url", "", "hosted check run URL")
	flag.StringVar(&input.CIEvent, "ci-event", "", "hosted check run event")
	flag.StringVar(&input.CISHA, "ci-sha", "", "hosted check commit")
	flag.StringVar(&input.CIConclusion, "ci-conclusion", "", "hosted check conclusion")
	flag.StringVar(&input.CICompletedAt, "ci-completed-at", "", "hosted check completion time")
	flag.BoolVar(&input.MainExactCandidate, "main-exact-candidate", false, "current main matches the candidate")
	flag.BoolVar(&input.TagAbsent, "tag-absent", false, "remote tag is absent")
	flag.BoolVar(&input.ReleaseAbsent, "release-absent", false, "GitHub release is absent")
	flag.StringVar(&input.RepositoryRoot, "repository-root", ".", "repository root")
	flag.StringVar(&outputDirectory, "output-directory", "", "artifact output directory")
	flag.Parse()

	input.Now = time.Now()
	result, err := collectReleaseEvidence(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "release evidence failed: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(outputDirectory) == "" {
		fmt.Fprintln(os.Stderr, "release evidence failed: output directory is required")
		os.Exit(1)
	}
	if err := writeResult(outputDirectory, result); err != nil {
		fmt.Fprintf(os.Stderr, "release evidence failed: %v\n", err)
		os.Exit(1)
	}
}

func collectReleaseEvidence(input validationInput) (validationResult, error) {
	version, err := parseReleaseVersion(input.Version)
	if err != nil {
		return validationResult{}, err
	}
	latest, err := parseReleaseVersion(input.LatestTag)
	if err != nil {
		return validationResult{}, fmt.Errorf("latest stable tag: %w", err)
	}
	if !shaPattern.MatchString(input.CandidateSHA) {
		return validationResult{}, errors.New("candidate commit must contain 40 lowercase hexadecimal characters")
	}

	location, err := time.LoadLocation(losAngelesZone)
	if err != nil {
		return validationResult{}, fmt.Errorf("load release time zone: %w", err)
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	localNow := now.In(location)

	completedAt, completionErr := time.Parse(time.RFC3339, input.CICompletedAt)
	actualSoak := time.Duration(0)
	if completionErr == nil {
		actualSoak = now.Sub(completedAt)
	}

	changelog, err := os.ReadFile(filepath.Join(input.RepositoryRoot, "CHANGELOG.md"))
	if err != nil {
		return validationResult{}, fmt.Errorf("read changelog: %w", err)
	}
	readme, err := os.ReadFile(filepath.Join(input.RepositoryRoot, "README.md"))
	if err != nil {
		return validationResult{}, fmt.Errorf("read README: %w", err)
	}

	notes, date, sectionPresent, notesPresent := extractVersionNotes(string(changelog), input.Version)
	unreleasedLink := "[Unreleased]: https://github.com/odvcencio/gotreesitter/compare/" + input.Version + "...HEAD"
	versionLink := "[" + strings.TrimPrefix(input.Version, "v") + "]: https://github.com/odvcencio/gotreesitter/compare/" +
		input.LatestTag + "..." + input.Version
	receipt := strings.TrimSpace(input.ReceiptURI)

	facts := policyFacts{
		Release: releasePolicyFacts{
			Kind:                  input.ReleaseKind,
			VersionIncrement:      classifyVersionIncrement(version, latest),
			LosAngelesThursday:    localNow.Weekday() == time.Thursday,
			SoakSeconds:           int64(actualSoak / time.Second),
			IncidentPresent:       strings.TrimSpace(input.Incident) != "",
			DelayRationalePresent: strings.TrimSpace(input.WhyWaitingIsWorse) != "",
		},
		Evidence: evidencePolicyFacts{
			MainExactCandidate:      input.MainExactCandidate,
			CIExactCandidate:        input.CISHA == input.CandidateSHA,
			CIFullDispatch:          input.CIEvent == "workflow_dispatch",
			CISuccessful:            input.CIConclusion == "success",
			CICompletionNotFuture:   completionErr == nil && !now.Before(completedAt),
			TagAbsent:               input.TagAbsent,
			ReleaseAbsent:           input.ReleaseAbsent,
			ReceiptValid:            strings.HasPrefix(receipt, "hypha://") && len(receipt) > len("hypha://"),
			ChangelogSectionPresent: sectionPresent,
			ChangelogNotesPresent:   notesPresent,
			ChangelogDateMatches:    sectionPresent && date == localNow.Format("2006-01-02"),
			ReadmeMatches:           strings.Contains(string(readme), "The current release is **"+input.Version+"**."),
			UnreleasedLinkMatches:   containsLine(string(changelog), unreleasedLink),
			VersionLinkMatches:      containsLine(string(changelog), versionLink),
		},
	}

	return validationResult{
		Title:       "gotreesitter " + input.Version,
		Notes:       notes,
		Summary:     buildSummary(input, localNow, completedAt, completionErr == nil, actualSoak, facts),
		PolicyFacts: facts,
		CIRun: ciRunEvidence{
			RunID:       input.CIRunID,
			URL:         input.CIRunURL,
			Event:       input.CIEvent,
			SHA:         input.CISHA,
			Conclusion:  input.CIConclusion,
			CompletedAt: input.CICompletedAt,
		},
	}, nil
}

func classifyVersionIncrement(version, latest releaseVersion) string {
	if version.major == latest.major && version.minor == latest.minor+1 && version.patch == 0 {
		return "next-minor"
	}
	if version.major == latest.major && version.minor == latest.minor && version.patch == latest.patch+1 {
		return "next-patch"
	}
	return "other"
}

func parseReleaseVersion(value string) (releaseVersion, error) {
	match := releaseVersionPattern.FindStringSubmatch(value)
	if match == nil {
		return releaseVersion{}, fmt.Errorf("version %q must use vX.Y.Z", value)
	}
	var version releaseVersion
	if _, err := fmt.Sscanf(value, "v%d.%d.%d", &version.major, &version.minor, &version.patch); err != nil {
		return releaseVersion{}, fmt.Errorf("parse version %q: %w", value, err)
	}
	return version, nil
}

func extractVersionNotes(changelog, version string) (notes, date string, sectionPresent, notesPresent bool) {
	number := strings.TrimPrefix(version, "v")
	header := regexp.MustCompile(`(?m)^## \[` + regexp.QuoteMeta(number) + `\] - ([0-9]{4}-[0-9]{2}-[0-9]{2})[ \t]*$`)
	match := header.FindStringSubmatchIndex(changelog)
	if match == nil {
		return "", "", false, false
	}
	notesStart := match[1]
	notesEnd := len(changelog)
	if next := strings.Index(changelog[notesStart:], "\n## "); next >= 0 {
		notesEnd = notesStart + next
	}
	notes = strings.TrimSpace(changelog[notesStart:notesEnd])
	if notes != "" {
		notes += "\n"
	}
	date = changelog[match[2]:match[3]]
	return notes, date, true, notes != ""
}

func containsLine(text, expected string) bool {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == expected {
			return true
		}
	}
	return false
}

func buildSummary(
	input validationInput,
	localNow time.Time,
	completedAt time.Time,
	completionValid bool,
	actualSoak time.Duration,
	facts policyFacts,
) string {
	var summary strings.Builder
	fmt.Fprintln(&summary, "# Release evidence summary")
	fmt.Fprintln(&summary)
	fmt.Fprintf(&summary, "- Version: `%s`\n", input.Version)
	fmt.Fprintf(&summary, "- Candidate commit: `%s`\n", input.CandidateSHA)
	fmt.Fprintf(&summary, "- Release route: `%s`\n", input.ReleaseKind)
	fmt.Fprintf(&summary, "- Version increment: `%s`\n", facts.Release.VersionIncrement)
	fmt.Fprintf(&summary, "- Release receipt: `%s`\n", input.ReceiptURI)
	fmt.Fprintf(&summary, "- Release time: `%s`\n", localNow.Format(time.RFC3339))
	fmt.Fprintf(&summary, "- Hosted CI run ID: `%s`\n", input.CIRunID)
	fmt.Fprintf(&summary, "- Hosted CI run URL: `%s`\n", input.CIRunURL)
	fmt.Fprintf(&summary, "- Hosted CI event: `%s`\n", input.CIEvent)
	fmt.Fprintf(&summary, "- Hosted CI commit: `%s`\n", input.CISHA)
	fmt.Fprintf(&summary, "- Hosted CI conclusion: `%s`\n", input.CIConclusion)
	if completionValid {
		fmt.Fprintf(&summary, "- Hosted CI completion: `%s`\n", completedAt.Format(time.RFC3339))
	} else {
		fmt.Fprintln(&summary, "- Hosted CI completion: `missing`")
	}
	fmt.Fprintf(&summary, "- Soak duration: `%s`\n", actualSoak.Round(time.Second))
	fmt.Fprintln(&summary, "- Policy facts: `policy-facts.json`")
	if input.ReleaseKind == "urgent-patch" {
		fmt.Fprintf(&summary, "- Incident: %s\n", strings.TrimSpace(input.Incident))
		fmt.Fprintf(&summary, "- Delay rationale: %s\n", strings.TrimSpace(input.WhyWaitingIsWorse))
	}
	return summary.String()
}

func writeResult(directory string, result validationResult) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	policyJSON, err := json.MarshalIndent(result.PolicyFacts, "", "  ")
	if err != nil {
		return fmt.Errorf("encode policy facts: %w", err)
	}
	ciRunJSON, err := json.MarshalIndent(result.CIRun, "", "  ")
	if err != nil {
		return fmt.Errorf("encode CI run evidence: %w", err)
	}
	files := []struct {
		name    string
		content []byte
	}{
		{name: "title.txt", content: []byte(result.Title + "\n")},
		{name: "notes.md", content: []byte(result.Notes)},
		{name: "summary.md", content: []byte(result.Summary)},
		{name: "policy-facts.json", content: append(policyJSON, '\n')},
		{name: "ci-run.json", content: append(ciRunJSON, '\n')},
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(directory, file.name), file.content, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", file.name, err)
		}
	}
	return nil
}
