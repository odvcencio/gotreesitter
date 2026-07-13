// Command wasmassets builds the persistent gotreesitter WASM runtime and emits
// a self-contained browser asset bundle for one registered language.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/odvcencio/gotreesitter/grammars"
)

type manifest struct {
	Version  int               `json:"version"`
	Language string            `json:"language"`
	Assets   map[string]string `json:"assets"`
}

func main() {
	languageName := flag.String("language", "go", "registered grammar name")
	output := flag.String("output", "", "output directory")
	tinygo := flag.String("tinygo", "tinygo", "TinyGo executable")
	flag.Parse()
	if strings.TrimSpace(*output) == "" {
		fatal(errors.New("-output is required"))
	}
	if err := generate(*languageName, *output, *tinygo); err != nil {
		fatal(err)
	}
}

func generate(languageName, output, tinygo string) error {
	entry := grammars.DetectLanguageByName(languageName)
	if entry == nil {
		return fmt.Errorf("unknown language %q", languageName)
	}
	root, err := moduleRoot()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}

	assets := map[string]string{}
	write := func(name string, data []byte, mode os.FileMode) error {
		path := filepath.Join(output, name)
		if err := os.WriteFile(path, data, mode); err != nil {
			return err
		}
		digest := sha256.Sum256(data)
		assets[name] = "sha256:" + hex.EncodeToString(digest[:])
		return nil
	}

	runtimeName := "gotreesitter.wasm"
	runtimePath := filepath.Join(output, runtimeName)
	build := exec.Command(tinygo, "build", "-target", "wasm", "-no-debug", "-opt=z", "-o", runtimePath, "./wasm/runtime")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build runtime: %w", err)
	}
	runtime, err := os.ReadFile(runtimePath)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(runtime)
	assets[runtimeName] = "sha256:" + hex.EncodeToString(digest[:])

	tinygoRoot, err := commandOutput(tinygo, "env", "TINYGOROOT")
	if err != nil {
		return fmt.Errorf("locate TinyGo runtime: %w", err)
	}
	wasmExec, err := os.ReadFile(filepath.Join(tinygoRoot, "targets", "wasm_exec.js"))
	if err != nil {
		return err
	}
	if err := write("wasm_exec.js", wasmExec, 0o644); err != nil {
		return err
	}

	blobName := entry.Name + ".bin"
	if err := copyAsset(filepath.Join(root, "grammars", "grammar_blobs", blobName), filepath.Join(output, blobName), assets); err != nil {
		return err
	}
	if strings.TrimSpace(entry.HighlightQuery) == "" {
		return fmt.Errorf("language %q has no highlight query", entry.Name)
	}
	if err := write(entry.Name+"-highlights.scm", []byte(entry.HighlightQuery), 0o644); err != nil {
		return err
	}
	if tags := grammars.ResolveTagsQuery(*entry); strings.TrimSpace(tags) != "" {
		if err := write(entry.Name+"-tags.scm", []byte(tags), 0o644); err != nil {
			return err
		}
	}

	encoded, err := json.MarshalIndent(manifest{Version: 1, Language: entry.Name, Assets: assets}, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	return os.WriteFile(filepath.Join(output, "manifest.json"), encoded, 0o644)
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}

func commandOutput(name string, args ...string) (string, error) {
	output, err := exec.Command(name, args...).Output()
	return strings.TrimSpace(string(output)), err
}

func copyAsset(source, destination string, assets map[string]string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.Create(destination)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(output, hash), input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	assets[filepath.Base(destination)] = "sha256:" + hex.EncodeToString(hash.Sum(nil))
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "wasmassets:", err)
	os.Exit(1)
}
