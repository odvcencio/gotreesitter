package cgoharness

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Odin corpus keeps its vendored Windows libraries in Git LFS. A checkout
// made with git-lfs replaces each pointer with the real object. The harness
// image has no git-lfs, so its git cannot run the clean filter that maps the
// object back to the pointer, and it reports the file as modified.

const (
	forestLFSTestPath   = "vendor/lib/blob.lib"
	forestLFSTestObject = "bytes that git-lfs stored instead of the blob\n"
)

func forestLFSTestPointer(object string) string {
	return fmt.Sprintf("version https://git-lfs.github.com/spec/v1\noid sha256:%x\nsize %d\n", sha256.Sum256([]byte(object)), len(object))
}

// initForestLFSTestCheckout commits a corpus in which every path in lfsPaths
// carries the LFS filter attribute and holds the pointer for
// forestLFSTestObject. Entries in files replace or add blobs. The real object
// is not written, so a test writes it to show a smudged checkout. The helper
// hides the developer's git config so that no LFS filter runs, the same as in
// the harness image.
func initForestLFSTestCheckout(t *testing.T, lfsPaths []string, files map[string]string) (string, forestCorpusLockEntry) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	var attributes strings.Builder
	committed := map[string]string{"src/main.odin": "package main\n"}
	for _, lfsPath := range lfsPaths {
		fmt.Fprintf(&attributes, "%s filter=lfs diff=lfs merge=lfs -text\n", lfsPath)
		committed[lfsPath] = forestLFSTestPointer(forestLFSTestObject)
	}
	committed[".gitattributes"] = attributes.String()
	for name, content := range files {
		committed[name] = content
	}
	root := t.TempDir()
	revision := initForestManifestTestRepo(t, root, "odin", committed)
	return root, forestCorpusLockEntry{
		Language: "odin", Repository: "https://example.invalid/odin",
		Revision: revision, Subdir: ".", Matchers: []string{".odin"},
	}
}

func TestVerifyForestCorpusCheckoutAcceptsSmudgedGitLFSObject(t *testing.T) {
	root, entry := initForestLFSTestCheckout(t, []string{forestLFSTestPath}, nil)
	checkout := filepath.Join(root, "odin")
	writeForestManifestTestFile(t, checkout, forestLFSTestPath, forestLFSTestObject)
	if status := runForestManifestGit(t, checkout, "status", "--porcelain=v1"); !strings.Contains(status, " M "+forestLFSTestPath) {
		t.Fatalf("git does not report the smudged object as modified, so this test shows nothing: %q", status)
	}
	if err := verifyForestCorpusCheckout(root, entry); err != nil {
		t.Fatalf("verify smudged LFS checkout: %v", err)
	}
}

func TestMaterializeForestCorpusManifestSelectsFromSmudgedGitLFSCheckout(t *testing.T) {
	root, entry := initForestLFSTestCheckout(t, []string{forestLFSTestPath}, nil)
	writeForestManifestTestFile(t, filepath.Join(root, "odin"), forestLFSTestPath, forestLFSTestObject)
	lockPath := filepath.Join(root, "corpus_sources.lock")
	lock := fmt.Sprintf("odin %s %s . .odin\n", entry.Repository, entry.Revision)
	if err := os.WriteFile(lockPath, []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest, err := MaterializeForestCorpusManifest(ForestCorpusMaterializeOptions{
		GotreesitterRevision: strings.Repeat("a", 40), CorpusLockPath: lockPath, CorpusRoot: root,
		Languages: []string{"odin"}, Selection: ForestCorpusSelection{Order: "path"},
	})
	if err != nil {
		t.Fatalf("materialize smudged LFS checkout: %v", err)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].Path != "src/main.odin" {
		t.Fatalf("files = %#v, want only src/main.odin", manifest.Files)
	}
}

func TestVerifyForestCorpusCheckoutRejectsGitLFSPathWithOtherContent(t *testing.T) {
	tests := map[string]string{
		"same size, other bytes": strings.Repeat("x", len(forestLFSTestObject)),
		"shorter":                forestLFSTestObject[:len(forestLFSTestObject)-1],
		"longer":                 forestLFSTestObject + "tampered\n",
		"empty":                  "",
		"pointer plus a line":    forestLFSTestPointer(forestLFSTestObject) + "tampered\n",
	}
	for name, content := range tests {
		t.Run(name, func(t *testing.T) {
			root, entry := initForestLFSTestCheckout(t, []string{forestLFSTestPath}, nil)
			writeForestManifestTestFile(t, filepath.Join(root, "odin"), forestLFSTestPath, content)
			err := verifyForestCorpusCheckout(root, entry)
			if err == nil || !strings.Contains(err.Error(), "checkout is dirty ( M "+forestLFSTestPath+")") {
				t.Fatalf("verify error = %v", err)
			}
		})
	}
}

func TestVerifyForestCorpusCheckoutRejectsSelectedSourceNextToGitLFSObject(t *testing.T) {
	// git lists changes by path. One case sorts the verified LFS file before
	// the modified source and one after, so neither order can hide the source.
	for name, lfsPath := range map[string]string{
		"LFS path first": "assets/blob.lib",
		"LFS path last":  forestLFSTestPath,
	} {
		t.Run(name, func(t *testing.T) {
			root, entry := initForestLFSTestCheckout(t, []string{lfsPath}, nil)
			checkout := filepath.Join(root, "odin")
			writeForestManifestTestFile(t, checkout, lfsPath, forestLFSTestObject)
			writeForestManifestTestFile(t, checkout, "src/main.odin", "package tampered\n")
			err := verifyForestCorpusCheckout(root, entry)
			if err == nil || !strings.Contains(err.Error(), "checkout is dirty ( M src/main.odin)") {
				t.Fatalf("verify error = %v", err)
			}
		})
	}
}

func TestVerifyForestCorpusCheckoutOnlyForgivesWorktreeChangeToGitLFSPath(t *testing.T) {
	tests := map[string]struct {
		lfsPaths []string
		files    map[string]string
		mutate   func(t *testing.T, checkout string)
	}{
		"staged object": {
			lfsPaths: []string{forestLFSTestPath},
			mutate: func(t *testing.T, checkout string) {
				t.Helper()
				writeForestManifestTestFile(t, checkout, forestLFSTestPath, forestLFSTestObject)
				runForestManifestGit(t, checkout, "add", forestLFSTestPath)
			},
		},
		"staged object and a later edit": {
			lfsPaths: []string{forestLFSTestPath},
			mutate: func(t *testing.T, checkout string) {
				t.Helper()
				writeForestManifestTestFile(t, checkout, forestLFSTestPath, forestLFSTestObject)
				runForestManifestGit(t, checkout, "add", forestLFSTestPath)
				writeForestManifestTestFile(t, checkout, forestLFSTestPath, forestLFSTestObject+"more\n")
			},
		},
		"deleted": {
			lfsPaths: []string{forestLFSTestPath},
			mutate: func(t *testing.T, checkout string) {
				t.Helper()
				if err := os.Remove(filepath.Join(checkout, filepath.FromSlash(forestLFSTestPath))); err != nil {
					t.Fatal(err)
				}
			},
		},
		"replaced by a symbolic link": {
			lfsPaths: []string{forestLFSTestPath},
			mutate: func(t *testing.T, checkout string) {
				t.Helper()
				writeForestManifestTestFile(t, checkout, "object", forestLFSTestObject)
				link := filepath.Join(checkout, filepath.FromSlash(forestLFSTestPath))
				if err := os.Remove(link); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(checkout, "object"), link); err != nil {
					t.Skipf("symbolic links are unavailable: %v", err)
				}
			},
		},
		"no filter attribute on the path": {
			// The blob at docs/pointer.txt is a valid pointer, but nothing marks
			// the path as LFS, so git reports the change in any environment.
			files: map[string]string{"docs/pointer.txt": forestLFSTestPointer(forestLFSTestObject)},
			mutate: func(t *testing.T, checkout string) {
				t.Helper()
				writeForestManifestTestFile(t, checkout, "docs/pointer.txt", forestLFSTestObject)
			},
		},
		"blob is not a pointer": {
			// The path carries the LFS attribute but HEAD holds raw bytes, as for a
			// file committed before the move to LFS. Nothing names the right object.
			lfsPaths: []string{"legacy/raw.lib"},
			files:    map[string]string{"legacy/raw.lib": "raw bytes committed before LFS\n"},
			mutate: func(t *testing.T, checkout string) {
				t.Helper()
				writeForestManifestTestFile(t, checkout, "legacy/raw.lib", forestLFSTestObject)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root, entry := initForestLFSTestCheckout(t, test.lfsPaths, test.files)
			test.mutate(t, filepath.Join(root, "odin"))
			if err := verifyForestCorpusCheckout(root, entry); err == nil || !strings.Contains(err.Error(), "checkout is dirty") {
				t.Fatalf("verify error = %v", err)
			}
		})
	}
}

func TestParseForestGitLFSPointer(t *testing.T) {
	oid := fmt.Sprintf("%x", sha256.Sum256([]byte("object")))
	valid := "version https://git-lfs.github.com/spec/v1\noid sha256:" + oid + "\nsize 6\n"
	tests := []struct {
		name     string
		pointer  string
		wantSize int64
		wantOK   bool
	}{
		{name: "pointer", pointer: valid, wantSize: 6, wantOK: true},
		{name: "empty object", pointer: "version https://git-lfs.github.com/spec/v1\noid sha256:" + oid + "\nsize 0\n", wantOK: true},
		{name: "empty file", pointer: ""},
		{name: "no final newline", pointer: strings.TrimSuffix(valid, "\n")},
		{name: "carriage returns", pointer: strings.ReplaceAll(valid, "\n", "\r\n")},
		{name: "older specification", pointer: strings.Replace(valid, "git-lfs.github.com", "hawser.github.com", 1)},
		{name: "extension line", pointer: strings.Replace(valid, "oid ", "ext-0-foo sha256:"+oid+"\noid ", 1)},
		{name: "trailing line", pointer: valid + "note x\n"},
		{name: "lines out of order", pointer: "version https://git-lfs.github.com/spec/v1\nsize 6\noid sha256:" + oid + "\n"},
		{name: "other hash", pointer: strings.Replace(valid, "sha256:", "sha1:", 1)},
		{name: "upper case digest", pointer: strings.Replace(valid, oid, strings.ToUpper(oid), 1)},
		{name: "short digest", pointer: strings.Replace(valid, oid, oid[:63], 1)},
		{name: "digest is not hex", pointer: strings.Replace(valid, oid, strings.Repeat("g", 64), 1)},
		{name: "signed size", pointer: strings.Replace(valid, "size 6", "size +6", 1)},
		{name: "padded size", pointer: strings.Replace(valid, "size 6", "size 06", 1)},
		{name: "negative size", pointer: strings.Replace(valid, "size 6", "size -6", 1)},
		{name: "size overflow", pointer: strings.Replace(valid, "size 6", "size 9223372036854775808", 1)},
		{name: "size is not a number", pointer: strings.Replace(valid, "size 6", "size six", 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotOID, gotSize, ok := parseForestGitLFSPointer([]byte(test.pointer))
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if ok && (gotOID != oid || gotSize != test.wantSize) {
				t.Fatalf("oid, size = %s, %d, want %s, %d", gotOID, gotSize, oid, test.wantSize)
			}
		})
	}
}
