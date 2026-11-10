#!/usr/bin/env python3
"""Track fuzz-nightly failures as GitHub issues, one issue per fuzz target.

Two subcommands:

  collect  Run in each fuzz matrix leg. Reads the go test log and the new
           crasher files, then writes result.json.
  sync     Run once after the matrix. Reads every result.json and opens,
           updates, comments on, or closes issues through the gh CLI.

Rules for sync:
  * A failing target gets one open issue titled "fuzz-nightly: <pkg>#<Target>".
    A second failure edits the same issue and resets the green streak.
  * A passing target with an open issue gets a comment and a streak of N
    green nights. The issue closes at --close-after nights (default 3).
  * A leg that neither passed nor produced a crash (build error, cancel,
    runner loss) changes nothing.

The streak lives in a hidden marker in the issue body:
  <!-- fuzz-nightly target=<pkg>#<Target> green=<n> -->

--dry-run prints the planned gh calls and reads existing issues from
--issues-json instead of calling gh.
"""
import argparse
import base64
import json
import os
import re
import subprocess
import sys

LABEL = "fuzz-nightly"
TITLE_PREFIX = "fuzz-nightly:"
MARKER_RE = re.compile(r"<!-- fuzz-nightly target=(\S+) green=(\d+) -->")
MAX_FILE = 16 * 1024
MAX_TOTAL = 48 * 1024
CRASH_RE = re.compile(r"Failing input written to (\S+)")
FAIL_RE = re.compile(r"^--- FAIL|^panic:|Failing input written", re.M)


def title_for(target):
    return f"{TITLE_PREFIX} {target}"


def marker(target, green):
    return f"<!-- fuzz-nightly target={target} green={green} -->"


def classify(log, exit_code):
    """Return pass, fail, or error. Only pass and fail change issues."""
    if exit_code == 0:
        return "pass"
    return "fail" if FAIL_RE.search(log) else "error"


def failure_message(log):
    lines = log.splitlines()
    start = next((i for i, l in enumerate(lines) if l.startswith("--- FAIL") or l.startswith("panic:")), 0)
    end = next((i for i, l in enumerate(lines) if "Failing input written" in l), len(lines) - 1)
    return "\n".join(lines[start : max(end, start) + 1][:60])


def find_crasher(path):
    """go test prints the path relative to the package directory."""
    if os.path.isfile(path):
        return path
    suffix = os.sep + os.path.normpath(path)
    for root, dirs, names in os.walk("."):
        dirs[:] = [d for d in dirs if d != ".git"]
        for n in names:
            full = os.path.join(root, n)
            if full.endswith(suffix):
                return full
    return None


def cmd_collect(a):
    log = open(a.log, errors="replace").read() if os.path.exists(a.log) else ""
    files = []
    for m in CRASH_RE.finditer(log):
        path = find_crasher(m.group(1))
        if path is None:
            continue
        raw = open(path, "rb").read()
        entry = {"name": path}
        try:
            entry["text"] = raw.decode("utf-8")
        except UnicodeDecodeError:
            entry["base64"] = base64.b64encode(raw).decode()
        entry["bytes"] = len(raw)
        files.append(entry)
    result = {
        "package": a.package,
        "target": a.target,
        "outcome": classify(log, a.exit_code),
        "exit_code": a.exit_code,
        "message": failure_message(log),
        "files": files,
    }
    os.makedirs(os.path.dirname(os.path.abspath(a.out)), exist_ok=True)
    json.dump(result, open(a.out, "w"), indent=1)
    print(f"{a.package}#{a.target}: {result['outcome']} ({len(files)} crasher files)")


def render_inputs(files):
    out, total = [], 0
    for f in files:
        name = os.path.basename(os.path.dirname(f["name"])) + "/" + os.path.basename(f["name"])
        if "text" in f:
            body, kind = f["text"], ""
        else:
            body, kind = f["base64"], "base64 "
        if len(body) > MAX_FILE or total + len(body) > MAX_TOTAL:
            out.append(f"`{name}` ({f['bytes']} bytes) is too large to inline. Download it from the run artifacts.")
            continue
        total += len(body)
        longest = max((len(m) for m in re.findall(r"`+", body)), default=0)
        fence = "`" * max(3, longest + 1)
        out.append(f"{kind}`{name}`:\n\n{fence}\n{body.rstrip(chr(10))}\n{fence}")
    return "\n\n".join(out) or "No crasher file was written. See the log excerpt."


def render_body(r, green, sha, run_url):
    target = f"{r['package']}#{r['target']}"
    name = r["target"]
    repro = f"go test -run='^$' -fuzz='^{name}$' -fuzztime=30s {r['package']}"
    replay = f"go test -run='^{name}$' {r['package']}"
    return (
        f"{marker(target, green)}\n"
        f"Fuzz target `{target}` failed in the nightly run.\n\n"
        f"- Run: {run_url}\n- Commit: {sha}\n- Green nights since last failure: {green}\n\n"
        f"Reproduce (copy the input below into `testdata/fuzz/{name}/` in the package first):\n\n"
        f"```\n{replay}\n```\n\nFind it again by fuzzing:\n\n```\n{repro}\n```\n\n"
        f"Failure:\n\n```\n{r['message']}\n```\n\nInput:\n\n{render_inputs(r['files'])}\n"
    )


def load_results(d):
    results = []
    for root, _, names in os.walk(d):
        for n in sorted(names):
            if n == "result.json":
                results.append(json.load(open(os.path.join(root, n))))
    return results


class Gh:
    def __init__(self, repo, dry, issues_json):
        self.repo, self.dry, self.issues_json = repo, dry, issues_json

    def run(self, *args, stdin=None):
        cmd = ["gh", *args, "--repo", self.repo]
        if self.dry:
            print("DRY gh " + " ".join(args[:3]) + (f"  [{len(stdin)} byte body]" if stdin else ""))
            return ""
        return subprocess.run(cmd, input=stdin, text=True, check=True, capture_output=True).stdout

    def open_issues(self):
        if self.dry:
            return json.load(open(self.issues_json)) if self.issues_json else []
        out = subprocess.run(
            ["gh", "issue", "list", "--repo", self.repo, "--state", "open", "--label", LABEL,
             "--limit", "500", "--json", "number,title,body"],
            text=True, check=True, capture_output=True).stdout
        return json.loads(out)

    def ensure_label(self):
        if self.dry:
            print(f"DRY gh label create {LABEL} (if missing)")
            return
        subprocess.run(["gh", "label", "create", LABEL, "--repo", self.repo, "--color", "B60205",
                        "--description", "Tracked by the nightly fuzz workflow"], capture_output=True)


def cmd_sync(a):
    gh = Gh(a.repo, a.dry_run, a.issues_json)
    results = load_results(a.results)
    existing = {}
    for issue in gh.open_issues():
        m = MARKER_RE.search(issue.get("body") or "")
        if m and issue["title"].startswith(TITLE_PREFIX):
            existing[m.group(1)] = (issue["number"], int(m.group(2)))
    labeled = False
    for r in results:
        try:
            target = f"{r['package']}#{r['target']}"
            cur = existing.get(target)
            if r["outcome"] == "fail":
                body = render_body(r, 0, a.sha, a.run_url)
                if cur is None:
                    if not labeled:
                        gh.ensure_label()
                        labeled = True
                    print(f"{target}: open new issue")
                    gh.run("issue", "create", "--title", title_for(target), "--label", LABEL, "--body-file", "-", stdin=body)
                else:
                    print(f"{target}: update issue #{cur[0]}, streak reset")
                    gh.run("issue", "edit", str(cur[0]), "--body-file", "-", stdin=body)
                    gh.run("issue", "comment", str(cur[0]), "--body-file", "-",
                           stdin=f"Failed again. Run: {a.run_url} (commit {a.sha}). Green streak reset to 0.")
            elif r["outcome"] == "pass" and cur is not None:
                green = cur[1] + 1
                number, _ = cur
                if green >= a.close_after:
                    print(f"{target}: green {green}/{a.close_after}, close #{number}")
                    gh.run("issue", "comment", str(number), "--body-file", "-",
                           stdin=f"Passed {green} nights in a row ({a.run_url}). Closing. A new failure opens a new issue.")
                    gh.run("issue", "close", str(number), "--reason", "completed")
                else:
                    print(f"{target}: green {green}/{a.close_after}, comment on #{number}")
                    old = next(i for i in gh.open_issues() if i["number"] == number) if not a.dry_run else None
                    body = (old["body"] if old else "")
                    if a.dry_run:
                        body = marker(target, cur[1])
                    body = MARKER_RE.sub(marker(target, green), body, count=1)
                    gh.run("issue", "edit", str(number), "--body-file", "-", stdin=body)
                    gh.run("issue", "comment", str(number), "--body-file", "-",
                           stdin=f"Passed ({a.run_url}). Green nights: {green} of {a.close_after} needed to close.")
            else:
                print(f"{target}: {r['outcome']}, no change")
        except subprocess.CalledProcessError as e:
            print(f"{r['package']}#{r['target']}: gh failed: {(e.stderr or '').strip()}", file=sys.stderr)
    return 0  # gh errors are logged above; the red fuzz leg already fails the run


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = p.add_subparsers(dest="cmd", required=True)
    c = sub.add_parser("collect")
    c.add_argument("--log", required=True)
    c.add_argument("--package", required=True)
    c.add_argument("--target", required=True)
    c.add_argument("--exit-code", type=int, required=True)
    c.add_argument("--out", required=True)
    c.set_defaults(fn=cmd_collect)
    s = sub.add_parser("sync")
    s.add_argument("--results", required=True)
    s.add_argument("--repo", default=os.environ.get("GITHUB_REPOSITORY", ""))
    s.add_argument("--sha", default=os.environ.get("GITHUB_SHA", ""))
    s.add_argument("--run-url", default="")
    s.add_argument("--close-after", type=int, default=3)
    s.add_argument("--dry-run", action="store_true")
    s.add_argument("--issues-json", help="dry-run only: open issues as gh would list them")
    s.set_defaults(fn=cmd_sync)
    a = p.parse_args()
    a.fn(a)


if __name__ == "__main__":
    sys.exit(main())
