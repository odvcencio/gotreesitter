//go:build docker_parity

package cgoharness

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnershipRepairPipelineReportsFindFailure(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"find":  "#!/bin/sh\nprintf '%s\\0' '/root-owned path'\nif [ \"$MOCK_FIND_FAIL\" = 1 ]; then exit 71; fi\n",
		"chown": "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MOCK_CHOWN_LOG\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0755); err != nil {
			t.Fatal(err)
		}
	}
	args := ownershipRepairArgs("/repo", "img", 1000, 1001)
	var i int
	for i = 0; i < len(args) && args[i] != "bash"; i++ {
	}
	if i == len(args) {
		t.Fatal("missing bash command")
	}
	for _, tc := range []struct {
		name, fail  string
		wantSuccess bool
	}{{"valid", "0", true}, {"producer failure after output", "1", false}} {
		t.Run(tc.name, func(t *testing.T) {
			log := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_"))
			cmd := exec.Command(args[i], args[i+1:]...)
			cmd.Env = []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"), "MOCK_FIND_FAIL=" + tc.fail, "MOCK_CHOWN_LOG=" + log, "GTS_HOST_UID=1000", "GTS_HOST_GID=1001"}
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.wantSuccess {
				t.Fatalf("command err=%v output=%s", err, out)
			}
			got, e := os.ReadFile(log)
			if e != nil {
				t.Fatalf("consumer did not run: %v", e)
			}
			if string(got) != "-h\n1000:1001\n/root-owned path\n" {
				t.Fatalf("consumer args=%q", got)
			}
		})
	}
}
