package graduation

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SourceFingerprint authenticates runtime sources and embedded grammar blobs,
// or the measurement harness. Tests and the receipt checker are excluded.
// A source change therefore requires new measurements before graduation can
// keep controlling the defaults. Paths in the receipt stay repository-relative.
func SourceFingerprint(root string, harness bool) (string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if entry.IsDir() {
			if relative == "." {
				return nil
			}
			if harness {
				if relative == "cgo_harness" {
					return nil
				}
				return filepath.SkipDir
			}
			if strings.HasPrefix(relative, "internal/graduation") || strings.HasPrefix(relative, "internal/benchfixtures") {
				return filepath.SkipDir
			}
			if relative == "internal" || relative == "grammars" || strings.HasPrefix(relative, "internal/") || strings.HasPrefix(relative, "grammars/") {
				return nil
			}
			return filepath.SkipDir
		}
		if harness {
			if relative == "cgo_harness/engine_ceiling_test.go" || relative == "cgo_harness/engine_ceiling_cgo.go" || relative == "cgo_harness/compact_graduation_test.go" || relative == "cgo_harness/compact_edits_work_test.go" || relative == "cgo_harness/compact_edits_rss_test.go" || relative == "cgo_harness/parity_c_loader_cgo.go" || relative == "cgo_harness/go.mod" || relative == "cgo_harness/go.sum" {
				paths = append(paths, relative)
			}
		} else if strings.HasSuffix(relative, ".go") && !strings.HasSuffix(relative, "_test.go") || strings.HasSuffix(relative, ".bin") {
			paths = append(paths, relative)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			return "", err
		}
		hash.Write([]byte(path))
		hash.Write([]byte{0})
		sum := sha256.Sum256(data)
		hash.Write(sum[:])
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}
