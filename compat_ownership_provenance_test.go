package gotreesitter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const resultCompatAuthoritativeMainCommit = "2c5ddcc9a566d2df4eed8544df0a022636d9977b"

// These are zero-based registry indexes. Keep this map explicit so a later
// ledger regeneration cannot silently restore a parallel retirement commit.
var resultCompatCorrectedRetiredCommits = map[int]string{
	1:  "2ef4802f2dbb853486d852cc9c1909e2a26f0199",
	5:  "2ef4802f2dbb853486d852cc9c1909e2a26f0199",
	6:  "2509e61d8cceeeaa017ce46e059fee1a014933c2",
	8:  "2ef4802f2dbb853486d852cc9c1909e2a26f0199",
	12: "84aee737500e7b9e0365725fb7af18e059e9d9e5",
	15: "16d4873e62d7eeff43cf61b552465e27eda87536",
	18: "05cd86a0bb925078f833b69bac21effa3fcca657",
	21: "e074c5b22fafc12d587612404bd0b626a2ec628c",
	23: "ff3acd87af32d98ed0d05ddfe16d5430abbbcb4a",
	24: "062fb1a130c15c030192bc23cdd0dd7eed6af18f",
	25: "7c07270b3e98b691df5e9c8c6d10151cb8918b62",
	26: "2ef4802f2dbb853486d852cc9c1909e2a26f0199",
	28: "9e2a7d8668f99e6f3eb43f6a74f4d449451943b2",
	29: "3f2a9256d72a838bb62a39ccf4af2e7cc1724dbb",
	30: "e12abef4da826db0bef291127ef4ab62ae4071e2",
	37: "2427094ae0629d0bd5c6a2fe2c2e801cbc709d3a",
	39: "a5c853591fd84b1d004692b9f96651ad433d699d",
	40: "2427094ae0629d0bd5c6a2fe2c2e801cbc709d3a",
	44: "84aee737500e7b9e0365725fb7af18e059e9d9e5",
	45: "cca7215444b29aea8a7046406a928a263fddde0f",
	49: "db34ebb495d1cd0c6540428c7045ac032ac41a22",
	50: "3f2a9256d72a838bb62a39ccf4af2e7cc1724dbb",
	51: "db34ebb495d1cd0c6540428c7045ac032ac41a22",
	52: "29b6e302701058e2ed101941c6da5f52a98dfe9a",
	53: "84aee737500e7b9e0365725fb7af18e059e9d9e5",
	54: "c3e734a527ed95327cf4fc010167a222cea1900c",
	56: "84aee737500e7b9e0365725fb7af18e059e9d9e5",
	64: "0d84d2eb5e597420de90d7d275b945e9b15ba7df",
	68: "0d84d2eb5e597420de90d7d275b945e9b15ba7df",
	78: "af6dce4cf9dc323fba10155fdcab6a84a25e3927",
	80: "db34ebb495d1cd0c6540428c7045ac032ac41a22",
	82: "27b0f624032680f6246bb4ee307883a2b874afb9",
	84: "d212e7c52fffc56182b138b08bb29ee7c490aa11",
	85: "8ced58fd8591aa18490d2720cf579e107f472711",
	87: "93ab17820fd007b5808f2ab6b62e22f79df179d1",
}

type resultCompatRetirementFixture struct {
	AncestorOfMain bool
	EntryIDs       []string
}

// Docker worktrees can lack the host repository metadata. This fixture keeps
// the provenance gate deterministic when local Git history is unavailable.
var resultCompatRetirementFixtures = map[string]resultCompatRetirementFixture{
	"05cd86a0bb925078f833b69bac21effa3fcca657": {true, []string{"dispatch.d"}},
	"062fb1a130c15c030192bc23cdd0dd7eed6af18f": {true, []string{"dispatch.enforce"}},
	"0d84d2eb5e597420de90d7d275b945e9b15ba7df": {true, []string{"dispatch.robot", "dispatch.scheme"}},
	"144b30c9ee085406335f4549272e1ae843427993": {true, []string{"dispatch.erlang"}},
	"16d4873e62d7eeff43cf61b552465e27eda87536": {true, []string{"dispatch.crystal"}},
	"2427094ae0629d0bd5c6a2fe2c2e801cbc709d3a": {true, []string{"dispatch.hurl", "dispatch.ini"}},
	"27b0f624032680f6246bb4ee307883a2b874afb9": {true, []string{"generic.trailing-extra-trivia"}},
	"29b6e302701058e2ed101941c6da5f52a98dfe9a": {true, []string{"dispatch.objc"}},
	"2509e61d8cceeeaa017ce46e059fee1a014933c2": {true, []string{"dispatch.bash"}},
	"2c5ddcc9a566d2df4eed8544df0a022636d9977b": {true, []string{"dispatch.julia"}},
	"cca7215444b29aea8a7046406a928a263fddde0f": {true, []string{"dispatch.ledger"}},
	"c3e734a527ed95327cf4fc010167a222cea1900c": {true, []string{"dispatch.ninja"}},
	"2ef4802f2dbb853486d852cc9c1909e2a26f0199": {true, []string{"dispatch.angular", "dispatch.bibtex", "dispatch.chatito", "dispatch.eds"}},
	"305e32b0bd796a47cfbd566ec596cd8d01ceb124": {true, []string{"dispatch.ql"}},
	"31bc9f1ed88bc930d22d0c2eaedc84195604cce1": {true, []string{"generic.terminal-leaf"}},
	"3f2a9256d72a838bb62a39ccf4af2e7cc1724dbb": {true, []string{"dispatch.forth", "dispatch.luau"}},
	"49d776674b2f599fa162874bbf74dc119fa9e7d4": {true, []string{"dispatch.hcl"}},
	"4ac19d4446b4b41daa236465a5316d624214343c": {true, []string{"dispatch.linkerscript"}},
	"22f506d9b633b9b83f405ca1d7d2770527b9a8cd": {true, []string{"fixpoint.returned-tree-second-pass.html"}},
	"6f9047d75b4406e2aadf8eeb1a47ebca752734b5": {true, []string{"dispatch.cpon"}},
	"7c07270b3e98b691df5e9c8c6d10151cb8918b62": {true, []string{"dispatch.ebnf"}},
	"84aee737500e7b9e0365725fb7af18e059e9d9e5": {true, []string{"dispatch.shared_trailing_span", "dispatch.just", "dispatch.nginx", "dispatch.pascal"}},
	"8ced58fd8591aa18490d2720cf579e107f472711": {true, []string{"fixpoint.returned-tree-second-pass"}},
	"8d0648f5e97fb75e4934aab90f0de4b0e3c7e821": {true, []string{"dispatch.javascript.dynamic-import"}},
	"9e2a7d8668f99e6f3eb43f6a74f4d449451943b2": {true, []string{"dispatch.fsharp"}},
	"93ab17820fd007b5808f2ab6b62e22f79df179d1": {true, []string{"fixpoint.returned-tree-second-pass.javascript"}},
	"a0e65e32a892ac33fe7481cf0837cb033e562a1c": {true, []string{"dispatch.http"}},
	"a5c853591fd84b1d004692b9f96651ad433d699d": {true, []string{"dispatch.hyprlang"}},
	"aadc2fed64f072499f8cc9485f7cd86db2a274c3": {true, []string{"dispatch.haskell"}},
	"af6dce4cf9dc323fba10155fdcab6a84a25e3927": {true, []string{"dispatch.typst"}},
	"af90246a860bf2c6a1632b7b4f5573e6b0b57339": {true, []string{"dispatch.cue", "dispatch.gitcommit", "dispatch.r"}},
	"c73670f0a4af1c6d9ba9572fb04eef2795bdfd26": {true, []string{"dispatch.rescript"}},
	"c79ee1ca40a332739374652beb02df9df5280e83": {true, []string{"dispatch.html", "dispatch.ocaml", "dispatch.ruby"}},
	"d212e7c52fffc56182b138b08bb29ee7c490aa11": {true, []string{"generic.collapsed-named-leaf"}},
	"d9c712131e152529b2e21e048c792db0a5b358c1": {true, []string{"dispatch.arduino"}},
	"dcb14a971284eedf064a08963455e940427f7dcc": {true, []string{"dispatch.kotlin.interpolated-call-expressions"}},
	"db34ebb495d1cd0c6540428c7045ac032ac41a22": {true, []string{"dispatch.lua", "dispatch.make", "dispatch.zig"}},
	"ddaed36e558d60d0e8e96bb9f6c59c0fb63c3b97": {true, []string{"dispatch.swift.ternary"}},
	"e074c5b22fafc12d587612404bd0b626a2ec628c": {true, []string{"dispatch.jsdoc"}},
	"e12abef4da826db0bef291127ef4ab62ae4071e2": {true, []string{"dispatch.fidl"}},
	"ff3acd87af32d98ed0d05ddfe16d5430abbbcb4a": {true, []string{"dispatch.elixir"}},
	"252a2513688945de445f02addfc6ce9680196577": {true, []string{"dispatch.squirrel"}},
}

func TestResultCompatibilityRetiredCommitProvenance(t *testing.T) {
	registry := loadResultCompatOwnershipRegistry(t)
	if got, want := len(registry.Entries), 88; got != want {
		t.Fatalf("registry entries = %d, want %d", got, want)
	}
	if got, want := len(resultCompatCorrectedRetiredCommits), 35; got != want {
		t.Fatalf("corrected retired commit rows = %d, want %d", got, want)
	}
	for index, wantCommit := range resultCompatCorrectedRetiredCommits {
		if index < 0 || index >= len(registry.Entries) {
			t.Fatalf("corrected registry index %d is outside the registry", index)
		}
		entry := registry.Entries[index]
		if entry.Status != "retired" {
			t.Errorf("registry index %d (%s) status = %q, want retired", index, entry.ID, entry.Status)
		}
		if entry.RetiredCommit != wantCommit {
			t.Errorf("registry index %d (%s) retired_commit = %q, want %q", index, entry.ID, entry.RetiredCommit, wantCommit)
		}
	}
	for index, entry := range registry.Entries {
		if entry.Status == "retired" && entry.ProducerFixCommit != "" && entry.ProducerFixCommit == entry.RetiredCommit {
			t.Errorf("registry index %d (%s) uses producer_fix_commit %s as retired_commit", index, entry.ID, entry.RetiredCommit)
		}
	}

	for index, entry := range registry.Entries {
		if entry.Status != "retired" {
			continue
		}
		if entry.RetiredCommit == "" {
			t.Errorf("registry index %d (%s) has no retired_commit", index, entry.ID)
			continue
		}
		fixture, ok := resultCompatRetirementFixtures[entry.RetiredCommit]
		if !ok || !fixture.AncestorOfMain || !containsString(fixture.EntryIDs, entry.ID) {
			t.Errorf("retirement fixture lacks ancestor and deletion evidence for registry index %d (%s), commit %s", index, entry.ID, entry.RetiredCommit)
		}
	}

	repo, ok := resultCompatGitRepository()
	if !ok {
		t.Log("local Git history is unavailable; used the deterministic retirement fixture")
		return
	}
	for index, entry := range registry.Entries {
		if entry.Status != "retired" {
			continue
		}
		if err := resultCompatRequireAncestor(repo, entry.RetiredCommit); err != nil {
			t.Errorf("registry index %d (%s) retired_commit %s is not an ancestor of authoritative main %s: %v", index, entry.ID, entry.RetiredCommit, resultCompatAuthoritativeMainCommit, err)
			continue
		}
		if evidence, err := resultCompatRetirementDiffEvidence(repo, entry); err != nil {
			t.Errorf("registry index %d (%s) retired_commit %s has no local retirement diff: %v", index, entry.ID, entry.RetiredCommit, err)
		} else if evidence == "" {
			t.Errorf("registry index %d (%s) retired_commit %s has no removed function, row, label, or file", index, entry.ID, entry.RetiredCommit)
		}
	}
}

func resultCompatGitRepository() (string, bool) {
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := command.Output()
	if err != nil {
		return "", false
	}
	repo := strings.TrimSpace(string(out))
	if repo == "" {
		return "", false
	}
	repo = filepath.Clean(repo)
	probe := exec.Command("git", "-C", repo, "rev-parse", "--verify", resultCompatAuthoritativeMainCommit+"^{commit}")
	probe.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if _, err := probe.Output(); err != nil {
		return "", false
	}
	return repo, true
}

func resultCompatRequireAncestor(repo, commit string) error {
	command := exec.Command("git", "-C", repo, "merge-base", "--is-ancestor", commit, resultCompatAuthoritativeMainCommit)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("git merge-base: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func resultCompatRetirementDiffEvidence(repo string, entry resultCompatOwnershipEntry) (string, error) {
	paths := append([]string{}, entry.Files...)
	paths = append(paths, resultCompatOwnershipRegistryPath, "parser_result_compat.go")
	paths = uniqueStrings(paths)
	args := []string{"-C", repo, "diff-tree", "--no-commit-id", "--no-ext-diff", "--no-renames", "--unified=0", "-p", entry.RetiredCommit + "^", entry.RetiredCommit, "--"}
	args = append(args, paths...)
	command := exec.Command("git", args...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git diff-tree: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	diff := string(out)
	for _, line := range strings.Split(diff, "\n") {
		if !strings.HasPrefix(line, "-") || strings.HasPrefix(line, "---") {
			continue
		}
		for _, function := range entry.Functions {
			if strings.Contains(line, function) {
				return "removed function " + function, nil
			}
		}
		if strings.Contains(line, entry.ID) {
			return "removed registry or census label " + entry.ID, nil
		}
		for _, language := range entry.Languages {
			if strings.Contains(line, strconv.Quote(language)) {
				return "removed language label " + language, nil
			}
		}
	}

	nameArgs := []string{"-C", repo, "diff-tree", "--no-commit-id", "--no-ext-diff", "--no-renames", "--name-status", "-r", entry.RetiredCommit + "^", entry.RetiredCommit, "--"}
	nameArgs = append(nameArgs, paths...)
	nameCommand := exec.Command("git", nameArgs...)
	nameCommand.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	nameOut, nameErr := nameCommand.CombinedOutput()
	if nameErr != nil {
		return "", fmt.Errorf("git diff-tree name-status: %w (%s)", nameErr, strings.TrimSpace(string(nameOut)))
	}
	for _, line := range strings.Split(string(nameOut), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) >= 2 && fields[0] == "D" && containsString(entry.Files, fields[1]) {
			return "removed registered file " + fields[1], nil
		}
	}
	return "", nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
