//go:build cgo && treesitter_c_parity

package editorlatency

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCampaignRoleBinding(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "grammars"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "grammars", "languages.lock")
	if err := os.WriteFile(path, []byte("lock"), 0600); err != nil {
		t.Fatal(err)
	}
	old := compiledCampaignRole
	t.Cleanup(func() { compiledCampaignRole = old })
	revision := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	compiledCampaignRole = &campaignRole{Role: "base", Root: root, Revision: revision, LockSHA256: digest([]byte("lock"))}
	t.Setenv("GTS_EDIT_REPO_ROOT", "")
	t.Setenv("GTS_EDIT_REVISION", "")
	if err := applyCampaignRole(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GTS_EDIT_REVISION", "other")
	if err := applyCampaignRole(); err == nil {
		t.Fatal("accepted conflicting revision")
	}
	t.Setenv("GTS_EDIT_REVISION", revision)
	t.Setenv("GTS_EDIT_REPO_ROOT", "/other")
	if err := applyCampaignRole(); err == nil {
		t.Fatal("accepted conflicting checkout")
	}
	t.Setenv("GTS_EDIT_REPO_ROOT", root)
	if err := os.WriteFile(path, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := applyCampaignRole(); err == nil {
		t.Fatal("accepted changed lock")
	}
}
