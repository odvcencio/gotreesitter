//go:build cgo && treesitter_c_parity && gts_derivation_set_census && gts_eof_history_census

package cgoharness

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const eofHistoryRuntimeParserSHA256 = "97a49276242294bc1b1a22908671a202f92d2a213a872edbe467a9e121822615"

type eofHistoryCVersion struct {
	AcceptIndex int
	Version     int
	Precedence  int
	ErrorCost   uint32
	Shape       string
}

type eofHistoryCReceipt struct {
	Versions  []eofHistoryCVersion
	Published string
	Summary   string
	Raw       string
}

func runEOFAcceptHistoryCOracle(t *testing.T, language string, source []byte) eofHistoryCReceipt {
	t.Helper()
	identity, err := COracleIdentity(language)
	if err != nil {
		t.Fatalf("load %s C oracle identity: %v", language, err)
	}
	if identity.BindingVersion != COracleBindingVersion || identity.BindingCommit != COracleBindingCommit ||
		identity.RuntimeCommit != COracleRuntimeCommit {
		t.Fatalf("%s C oracle identity is not locked: %+v", language, identity)
	}

	parityCRefState.mu.Lock()
	entry, ok := parityCRefState.lock[language]
	parityCRefState.mu.Unlock()
	if !ok {
		t.Fatalf("parity lock has no %s entry", language)
	}
	symbol, err := eofHistorySharedLanguageSymbol(identity.GrammarArtifactPath, parityLanguageSymbols(entry))
	if err != nil {
		t.Fatalf("resolve %s C language symbol: %v", language, err)
	}

	artifact := buildEOFAcceptHistoryRuntime(t)
	command := exec.Command(artifact, identity.GrammarArtifactPath, symbol)
	command.Stdin = bytes.NewReader(source)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run %s EOF-history C oracle: %v: %s", language, err, strings.TrimSpace(string(output)))
	}
	receipt, err := parseEOFAcceptHistoryCOutput(string(output))
	if err != nil {
		t.Fatalf("parse %s EOF-history C receipt: %v\n%s", language, err, output)
	}
	return receipt
}

func buildEOFAcceptHistoryRuntime(t *testing.T) string {
	t.Helper()
	var module struct {
		Path    string
		Version string
		Dir     string
	}
	command := exec.Command("go", "list", "-m", "-json", "github.com/tree-sitter/go-tree-sitter")
	output, err := command.Output()
	if err != nil {
		t.Fatalf("locate locked C binding source: %v", err)
	}
	if err := json.Unmarshal(output, &module); err != nil {
		t.Fatalf("decode locked C binding module: %v", err)
	}
	if module.Path != COracleBindingModule || module.Version != COracleBindingVersion || module.Dir == "" {
		t.Fatalf("unexpected locked C binding source: %+v", module)
	}

	buildRoot := t.TempDir()
	runtimeRoot := filepath.Join(buildRoot, "runtime")
	if err := eofHistoryCopyTree(filepath.Join(module.Dir, "src"), filepath.Join(runtimeRoot, "src")); err != nil {
		t.Fatalf("copy runtime src: %v", err)
	}
	if err := eofHistoryCopyTree(filepath.Join(module.Dir, "include"), filepath.Join(runtimeRoot, "include")); err != nil {
		t.Fatalf("copy runtime include: %v", err)
	}

	parserPath := filepath.Join(runtimeRoot, "src", "parser.c")
	parserSource, err := os.ReadFile(parserPath)
	if err != nil {
		t.Fatalf("read runtime parser.c: %v", err)
	}
	parserHash := sha256.Sum256(parserSource)
	if got := hex.EncodeToString(parserHash[:]); got != eofHistoryRuntimeParserSHA256 {
		t.Fatalf("runtime parser.c sha256=%s, want %s", got, eofHistoryRuntimeParserSHA256)
	}
	patched, err := eofHistoryPatchRuntimeParser(string(parserSource))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parserPath, 0o644); err != nil {
		t.Fatalf("make copied parser.c writable: %v", err)
	}
	if err := os.WriteFile(parserPath, []byte(patched), 0o644); err != nil {
		t.Fatalf("write diagnostic parser.c: %v", err)
	}

	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	driver := filepath.Join(repoRoot, "cgo_harness", "pure_c", "eof_history_oracle.c")
	artifact := filepath.Join(buildRoot, "eof_history_oracle")
	compile := exec.Command(
		"cc",
		"-O2", "-DNDEBUG", "-std=c11", "-D_POSIX_C_SOURCE=200112L", "-D_DEFAULT_SOURCE",
		"-DGTS_EOF_HISTORY_CENSUS",
		"-I", filepath.Join(runtimeRoot, "include"),
		"-I", filepath.Join(runtimeRoot, "src"),
		filepath.Join(runtimeRoot, "src", "lib.c"), driver,
		"-Wl,--export-dynamic", "-ldl", "-o", artifact,
	)
	if output, err := compile.CombinedOutput(); err != nil {
		t.Fatalf("compile diagnostic C runtime: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return artifact
}

func eofHistoryPatchRuntimeParser(source string) (string, error) {
	includeNeedle := "#include \"./wasm_store.h\"\n"
	includeReplacement := includeNeedle + "\n#ifdef GTS_EOF_HISTORY_CENSUS\n" +
		"void gts_eof_history_capture_root(const TSLanguage *, Subtree, uint32_t, uint32_t);\n" +
		"#endif\n"
	if strings.Count(source, includeNeedle) != 1 {
		return "", fmt.Errorf("runtime parser.c has %d wasm_store include seams, want 1", strings.Count(source, includeNeedle))
	}
	source = strings.Replace(source, includeNeedle, includeReplacement, 1)

	acceptNeedle := "    self->accept_count++;\n\n    if (self->finished_tree.ptr) {"
	acceptReplacement := "    self->accept_count++;\n" +
		"#ifdef GTS_EOF_HISTORY_CENSUS\n" +
		"    gts_eof_history_capture_root(self->language, root, self->accept_count - 1, version);\n" +
		"#endif\n\n    if (self->finished_tree.ptr) {"
	if strings.Count(source, acceptNeedle) != 1 {
		return "", fmt.Errorf("runtime parser.c has %d accept capture seams, want 1", strings.Count(source, acceptNeedle))
	}
	return strings.Replace(source, acceptNeedle, acceptReplacement, 1), nil
}

func eofHistoryCopyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("runtime source contains unsupported symlink %s", path)
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o444)
		if err != nil {
			_ = input.Close()
			return err
		}
		_, copyErr := io.Copy(output, input)
		inputCloseErr := input.Close()
		outputCloseErr := output.Close()
		if copyErr != nil {
			return copyErr
		}
		if inputCloseErr != nil {
			return inputCloseErr
		}
		return outputCloseErr
	})
}

func eofHistorySharedLanguageSymbol(sharedObject string, candidates []string) (string, error) {
	command := exec.Command("nm", "-D", "--defined-only", sharedObject)
	output, err := command.Output()
	if err != nil {
		return "", err
	}
	defined := make(map[string]bool)
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 0 {
			defined[fields[len(fields)-1]] = true
		}
	}
	for _, candidate := range candidates {
		if defined[candidate] {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no accepted language symbol in %v", candidates)
}

func parseEOFAcceptHistoryCOutput(raw string) (eofHistoryCReceipt, error) {
	receipt := eofHistoryCReceipt{Raw: raw}
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		switch {
		case strings.HasPrefix(line, "GTS_C_EOF_HISTORY "):
			shapeAt := strings.Index(line, " shape=")
			if shapeAt < 0 {
				return receipt, fmt.Errorf("history line lacks shape: %q", line)
			}
			var version eofHistoryCVersion
			if _, err := fmt.Sscanf(
				line[:shapeAt],
				"GTS_C_EOF_HISTORY accept=%d version=%d precedence=%d error_cost=%d",
				&version.AcceptIndex, &version.Version, &version.Precedence, &version.ErrorCost,
			); err != nil {
				return receipt, fmt.Errorf("decode history line: %w", err)
			}
			version.Shape = line[shapeAt+len(" shape="):]
			receipt.Versions = append(receipt.Versions, version)
		case strings.HasPrefix(line, "GTS_C_EOF_PUBLISHED shape="):
			receipt.Published = strings.TrimPrefix(line, "GTS_C_EOF_PUBLISHED shape=")
		case strings.HasPrefix(line, "GTS_C_EOF_SUMMARY "):
			receipt.Summary = line
		}
	}
	if len(receipt.Versions) == 0 || receipt.Published == "" || receipt.Summary == "" {
		return receipt, fmt.Errorf("incomplete C receipt: versions=%d published=%v summary=%v", len(receipt.Versions), receipt.Published != "", receipt.Summary != "")
	}
	return receipt, nil
}
