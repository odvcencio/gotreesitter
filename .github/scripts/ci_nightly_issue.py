#!/usr/bin/env python3
"""Keep one GitHub issue for the nightly heavy-tier ci run on main.

Pull requests run the light tier, so the nightly heavy run is where a
heavy-only regression shows first. This script runs after the nightly build
gate:

  build failure  open the issue "ci-nightly: heavy tier failed on main", or
                 update its body and add a comment, with the failed job names
                 and the run link
  build success  comment on the open issue and close it
  other result   change nothing (a cancelled run says nothing about main)

gh errors are printed and never fail the run: the build gate is the verdict.

--dry-run prints the planned gh calls. It reads the run's jobs from
--jobs-json ({"jobs": [{"name": ..., "conclusion": ...}]}) and the open issues
from --issues-json ([{"number": ..., "title": ...}]) instead of calling gh.
"""
import argparse
import json
import subprocess
import sys

TITLE = "ci-nightly: heavy tier failed on main"
FAILED = {"failure", "cancelled", "timed_out", "startup_failure"}


def gh(a, args, body=None):
    if a.dry_run:
        print("DRY gh " + " ".join(args) + (f"  [{len(body)} byte body]" if body else ""))
        return ""
    return subprocess.run(["gh", *args], input=body, text=True, check=True, capture_output=True).stdout


def failed_jobs(a):
    if a.dry_run:
        jobs = json.load(open(a.jobs_json))["jobs"] if a.jobs_json else []
        rows = [(j.get("conclusion") or "", j["name"]) for j in jobs]
    else:
        out = gh(a, ["api", "--paginate", f"repos/{a.repo}/actions/runs/{a.run_id}/attempts/{a.attempt}/jobs?per_page=30",
                     "--jq", '.jobs[] | "\\(.conclusion)\\t\\(.name)"'])
        rows = [tuple(line.split("\t", 1)) for line in out.splitlines() if "\t" in line]
    return sorted({name for conclusion, name in rows if conclusion in FAILED})


def open_issue(a):
    if a.dry_run:
        issues = json.load(open(a.issues_json)) if a.issues_json else []
    else:
        issues = json.loads(gh(a, ["issue", "list", "--repo", a.repo, "--state", "open", "--search",
                                   f'in:title "{TITLE}"', "--limit", "50", "--json", "number,title"]))
    return next((i["number"] for i in issues if i["title"] == TITLE), None)


def body_for(a, failed):
    names = "\n".join(f"- `{n}`" for n in failed) or "- (the job list was empty; open the run)"
    return (
        f"The nightly heavy-tier `ci` run on `main` failed. Pull requests run the light tier, "
        f"so this run is where a heavy-only regression shows first.\n\n"
        f"- Run: {a.run_url}\n- Commit: {a.sha}\n\nFailed jobs ({len(failed)}):\n\n{names}\n\n"
        f"The next nightly run whose `build` gate passes closes this issue. To run the heavy tier on a "
        f"branch: `gh workflow run ci.yml --ref <branch> -f tier=heavy`.\n"
    )


def sync(a):
    if a.build_result not in ("success", "failure"):
        print(f"build result {a.build_result!r}: no change")
        return
    number = open_issue(a)
    if a.build_result == "success":
        if number is None:
            print("build passed; no open issue")
            return
        print(f"build passed: close #{number}")
        gh(a, ["issue", "comment", str(number), "--repo", a.repo, "--body-file", "-"],
           f"The nightly heavy-tier run passed: {a.run_url} (commit {a.sha}). Closing.")
        gh(a, ["issue", "close", str(number), "--repo", a.repo, "--reason", "completed"])
        return
    failed = failed_jobs(a)
    body = body_for(a, failed)
    if number is None:
        print(f"build failed ({len(failed)} jobs): open the issue")
        gh(a, ["issue", "create", "--repo", a.repo, "--title", TITLE, "--body-file", "-"], body)
    else:
        print(f"build failed ({len(failed)} jobs): update #{number}")
        gh(a, ["issue", "edit", str(number), "--repo", a.repo, "--body-file", "-"], body)
        gh(a, ["issue", "comment", str(number), "--repo", a.repo, "--body-file", "-"],
           f"Failed again: {a.run_url} (commit {a.sha}). Failed jobs: " + ", ".join(f"`{n}`" for n in failed))


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--repo", required=True)
    p.add_argument("--run-id", required=True)
    p.add_argument("--attempt", required=True)
    p.add_argument("--sha", required=True)
    p.add_argument("--run-url", required=True)
    p.add_argument("--build-result", required=True)
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--jobs-json", help="dry-run only: the run's jobs as the API lists them")
    p.add_argument("--issues-json", help="dry-run only: open issues as gh lists them")
    a = p.parse_args()
    try:
        sync(a)
    except (subprocess.CalledProcessError, OSError, ValueError, KeyError) as e:
        print(f"ci-nightly issue sync failed: {(getattr(e, 'stderr', None) or str(e)).strip()}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
