//go:build cgo && treesitter_c_parity

// gts_downstream measures complete downstream operations against the locked C
// oracle. Correctness and each engine's timing run in separate child processes.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"

	ts "github.com/odvcencio/gotreesitter"
	harness "github.com/odvcencio/gotreesitter/cgo_harness"
	"github.com/odvcencio/gotreesitter/grammars"
)

const corpusLockDigest = "41c744279c8b1d7c9fe7b1b8e26fba733423e77cd48efea46927309c22d163ea"

var downstreamLanguages = []string{"go", "python", "typescript", "rust", "c"}

type options struct {
	workflow, language, shape, corpus, lock, root, phase, worker, output, revision, file string
	size                                                                                 int
	timeout                                                                              time.Duration
}

type job struct {
	Query       string   `json:"query,omitempty"`
	Workflow    string   `json:"workflow"`
	Language    string   `json:"language"`
	Shape       string   `json:"shape,omitempty"`
	TargetBytes int      `json:"target_bytes,omitempty"`
	Path        string   `json:"path,omitempty"`
	Commit      string   `json:"corpus_commit,omitempty"`
	Files       []string `json:"files,omitempty"`
	SourceHash  string   `json:"source_sha256,omitempty"`
}

type capture struct {
	NameStart uint32 `json:"name_start,omitempty"`
	NameEnd   uint32 `json:"name_end,omitempty"`
	Name      string `json:"name"`
	Start     uint32 `json:"start"`
	End       uint32 `json:"end"`
	Text      string `json:"text"`
}

type witness struct {
	Kind        string        `json:"kind"`
	File        string        `json:"file"`
	Step        int           `json:"step"` // 0 is open; 1..200 are keystrokes
	Edit        *ts.InputEdit `json:"edit,omitempty"`
	GoDigest    string        `json:"go_digest,omitempty"`
	CDigest     string        `json:"c_digest,omitempty"`
	FreshDigest string        `json:"fresh_digest,omitempty"`
	Detail      string        `json:"detail,omitempty"`
}

type measurement struct {
	Symbols      int    `json:"symbols,omitempty"`
	WallNS       int64  `json:"wall_ns"`
	P95NS        int64  `json:"p95_keystroke_ns,omitempty"`
	PeakRSSKiB   int64  `json:"peak_rss_kib"`
	Operations   int    `json:"operations"`
	Captures     int    `json:"captures"`
	OutputSHA256 string `json:"output_sha256,omitempty"`
	Error        string `json:"error,omitempty"`
}

type validation struct {
	Checks     int            `json:"checks"`
	Mismatches map[string]int `json:"mismatches"`
	Witnesses  []witness      `json:"witnesses,omitempty"`
	Error      string         `json:"error,omitempty"`
}

type workerResult struct {
	Measurement measurement `json:"measurement"`
	Validation  validation  `json:"validation"`
}

type receipt struct {
	Phase                 string      `json:"phase"`
	InputFiles            int         `json:"input_files"`
	InputBytes            int64       `json:"input_bytes"`
	EditSteps             int         `json:"edit_steps"`
	GoVersion             string      `json:"go_version"`
	Platform              string      `json:"platform"`
	GOMAXPROCS            int         `json:"gomaxprocs"`
	Schema                string      `json:"schema"`
	Revision              string      `json:"revision"`
	Job                   job         `json:"input"`
	InputSHA256           string      `json:"input_manifest_sha256"`
	QuerySHA256           string      `json:"query_sha256"`
	QueryOrigin           string      `json:"query_origin"`
	CorpusLockSHA256      string      `json:"corpus_lock_sha256,omitempty"`
	RuntimeCommit         string      `json:"c_runtime_commit"`
	BindingVersion        string      `json:"c_binding_version"`
	GrammarCommit         string      `json:"c_grammar_commit,omitempty"`
	GrammarArtifactSHA256 string      `json:"c_grammar_artifact_sha256,omitempty"`
	Go                    measurement `json:"go"`
	C                     measurement `json:"c"`
	Validation            validation  `json:"validation"`
	EqualOutputs          bool        `json:"equal_outputs"`
	Pass                  bool        `json:"pass"`
}

func main() {
	o := options{}
	flag.StringVar(&o.workflow, "workflow", "fixtures", "fixtures, editor, or index")
	flag.StringVar(&o.language, "language", "", "one grammar; default is the whole workflow")
	flag.StringVar(&o.file, "file", "", "one tracked relative source path for indexing")
	flag.StringVar(&o.shape, "shape", "", "one fixture shape; default is all 59 shapes")
	flag.IntVar(&o.size, "size", 0, "one fixture target byte size; default is all pinned sizes")
	flag.StringVar(&o.corpus, "corpus", "/corpus_sources", "pinned upstream checkouts")
	flag.StringVar(&o.lock, "corpus-lock", "", "external corpus lock; otherwise fetch GTS_CORPUS_LOCK_URL")
	flag.StringVar(&o.root, "root", "..", "repository root (run from cgo_harness)")
	flag.StringVar(&o.phase, "phase", "all", "all, check, or time; all is the correctness gate")
	flag.StringVar(&o.worker, "worker", "", "internal child process mode")
	flag.StringVar(&o.revision, "revision", "", "tested engine revision; needed when worktree git metadata is not mounted")
	flag.StringVar(&o.output, "output", "", "JSONL receipt path; default stdout")
	flag.DurationVar(&o.timeout, "timeout", 30*time.Minute, "wall limit per child; failures remain in receipts")
	flag.Parse()
	runtime.GOMAXPROCS(1)
	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(o options) error {
	if o.worker != "" {
		var j job
		if err := json.NewDecoder(os.Stdin).Decode(&j); err != nil {
			return err
		}
		r := workerResult{}
		if o.worker == "check" {
			r.Validation = validate(o, j)
		} else {
			r.Measurement = measure(o, j, o.worker)
		}
		return json.NewEncoder(os.Stdout).Encode(r)
	}
	if o.phase != "all" && o.phase != "check" && o.phase != "time" {
		return errors.New("phase must be all, check, or time")
	}
	if o.timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	jobs, err := plan(o)
	if err != nil {
		return err
	}
	out := io.Writer(os.Stdout)
	if o.output != "" {
		f, err := os.Create(o.output)
		if err != nil {
			return err
		}
		defer f.Close()
		out = f
	}
	enc := json.NewEncoder(out)
	revision := []byte(o.revision)
	if len(revision) == 0 {
		revision, err = git(o.root, "rev-parse", "HEAD")
		if err != nil {
			return err
		}
	}
	failures := 0
	for i, j := range jobs {
		fmt.Fprintf(os.Stderr, "%s %d/%d %s %s %d\n", o.workflow, i+1, len(jobs), j.Language, j.Shape, j.TargetBytes)
		r := receipt{Schema: "gts-downstream-v1", Revision: strings.TrimSpace(string(revision)), Job: j,
			RuntimeCommit: harness.COracleRuntimeCommit, BindingVersion: harness.COracleBindingVersion}
		if j.Workflow != "fixtures" {
			r.CorpusLockSHA256 = corpusLockDigest
		}
		r.Phase = o.phase
		r.GoVersion = runtime.Version()
		r.Platform = runtime.GOOS + "/" + runtime.GOARCH
		r.GOMAXPROCS = 1
		r.InputFiles = len(j.Files)
		if j.Workflow == "fixtures" {
			r.InputFiles = 1
			r.EditSteps = 3
		}
		if j.Workflow == "editor" {
			r.EditSteps = 200
		}
		r.InputSHA256, r.InputBytes, err = inputDigest(o, j)
		if err != nil {
			r.Validation.Error = err.Error()
		} else {
			entry := grammars.DetectLanguageByName(j.Language)
			if entry == nil {
				r.Validation.Error = "unregistered grammar"
			} else {
				q, origin := querySource(j, *entry)
				j.Query = string(q)
				r.QuerySHA256 = sha(q)
				r.QueryOrigin = origin
				identity, idErr := harness.COracleIdentity(j.Language)
				if idErr != nil {
					r.Validation.Error = idErr.Error()
				} else {
					r.GrammarCommit = identity.GrammarCommit
					r.GrammarArtifactSHA256 = identity.GrammarArtifactSHA256
					if o.phase != "time" {
						w := child(o, j, "check")
						r.Validation = w.Validation
					}
					if o.phase != "check" {
						// Alternate engine order to avoid a fixed warm-cache advantage.
						order := []string{"go", "c"}
						if i%2 == 1 {
							order = []string{"c", "go"}
						}
						for _, engine := range order {
							w := child(o, j, engine)
							if engine == "go" {
								r.Go = w.Measurement
							} else {
								r.C = w.Measurement
							}
						}
					}
				}
			}
		}
		r.EqualOutputs = r.Go.OutputSHA256 != "" && r.Go.OutputSHA256 == r.C.OutputSHA256
		r.Pass = o.phase == "all" && r.Validation.Error == "" && len(r.Validation.Mismatches) == 0 && r.Go.Error == "" && r.C.Error == "" && r.EqualOutputs
		failed := r.Validation.Error != ""
		if o.phase != "time" {
			failed = failed || len(r.Validation.Mismatches) > 0
		}
		if o.phase != "check" {
			failed = failed || r.Go.Error != "" || r.C.Error != "" || !r.EqualOutputs
		}
		if failed {
			failures++
		}
		// The file list is authenticated by InputSHA256; avoid repeating tens of
		// thousands of paths in each receipt. Witnesses retain their relative path.
		r.Job.Files = nil
		if err := enc.Encode(r); err != nil {
			return err
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d/%d workflows failed; see JSONL witnesses", failures, len(jobs))
	}
	return nil
}

func child(o options, j job, mode string) workerResult {
	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		return failedWorker(mode, err)
	}
	input, err := json.Marshal(j)
	if err != nil {
		return failedWorker(mode, err)
	}
	cmd := exec.CommandContext(ctx, exe, "-worker", mode, "-root", o.root, "-corpus", o.corpus)
	cmd.Stdin = bytes.NewReader(input)
	stderr := stderrTail{}
	cmd.Stderr = &stderr
	started := time.Now()
	data, err := cmd.Output()
	var r workerResult
	if err == nil {
		err = json.Unmarshal(data, &r)
	}
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("worker %s exceeded %s: %s", mode, o.timeout, stderr.String())
		} else {
			err = fmt.Errorf("worker %s: %w: %s", mode, err, stderr.String())
		}
		r = failedWorker(mode, err)
		if mode != "check" {
			r.Measurement.WallNS = time.Since(started).Nanoseconds()
		}
	}
	if cmd.ProcessState != nil {
		if usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
			r.Measurement.PeakRSSKiB = usage.Maxrss
		}
	}
	return r
}

func failedWorker(mode string, err error) workerResult {
	r := workerResult{}
	if mode == "check" {
		r.Validation.Error = err.Error()
	} else {
		r.Measurement.Error = err.Error()
	}
	return r
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// Keep the last input location on crashes without buffering every indexed path.
type stderrTail struct{ data []byte }

func (s *stderrTail) Write(b []byte) (int, error) {
	n := len(b)
	const limit = 4096
	if len(b) >= limit {
		s.data = append(s.data[:0], b[len(b)-limit:]...)
	} else {
		if len(s.data)+len(b) > limit {
			keep := limit - len(b)
			copy(s.data, s.data[len(s.data)-keep:])
			s.data = s.data[:keep]
		}
		s.data = append(s.data, b...)
	}
	return n, nil
}
func (s *stderrTail) String() string { return string(s.data) }
