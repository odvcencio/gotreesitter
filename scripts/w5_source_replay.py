"""Independent campaign-side observer for the npm TypeScript grammar family."""

import base64
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tarfile
import tempfile
import urllib.request

RECIPE_REPO = "https://github.com/tree-sitter/tree-sitter-typescript"
PIN = "scripts/w5_tree_sitter_cli_linux_amd64.json"


def check(ok, reason):
    if not ok:
        raise ValueError(reason)


def sha(data):
    return hashlib.sha256(data).hexdigest()


def git(root, *args):
    return subprocess.check_output(["git", "-C", str(root), *args])


def role_recipe(root, revision, name):
    # Read literal declarations only. Never run a role's Go package to discover its own authority.
    rows = [
        s.split()
        for s in git(root, "show", revision + ":grammars/languages.lock").decode().splitlines()
        if s.split()[:1] == [name]
    ]
    check(len(rows) == 1 and len(rows[0]) >= 4, "missing role lock")
    lang, repo, commit, subdir = rows[0][:4]
    check(
        repo == RECIPE_REPO
        and subdir in ("tsx/src", "typescript/src")
        and re.fullmatch("[0-9a-f]{40}", commit),
        "unsupported upstream generation recipe",
    )
    source = git(root, "show", revision + ":internal/grammarpatch/patch.go").decode()
    blocks = re.findall(r'^\s*"' + re.escape(lang) + r'": \{\n([^{}]+?)^\s*\},$', source, re.M)
    check(len(blocks) == 1, "ambiguous role patch declaration")
    fields = {}
    for line in blocks[0].strip().splitlines():
        match = re.fullmatch(r'\s*(File|RegenerateParser|ExtraFile):\s*(true|"[\w.-]+"),', line)
        check(match and match[1] not in fields, "unsupported role patch expression")
        fields[match[1]] = json.loads(match[2])
    check(fields.get("RegenerateParser") is True and "File" in fields, "unsupported generator recipe")
    config = dict(File=fields["File"], RegenerateParser=True, GeneratorVersion="")
    if "ExtraFile" in fields:
        config["ExtraFile"] = fields["ExtraFile"]
    patches = []
    for filename in (config["File"], config.get("ExtraFile")):
        if filename:
            check(re.fullmatch(r"[A-Za-z0-9_-]+\.patch", filename), "invalid role patch name")
            patches.append((filename, git(root, "show", revision + ":grammars/patches/" + filename)))
    encoded = json.dumps(config, separators=(",", ":")).encode()
    identity = sha(b"\0".join([encoded] + [sha(blob).encode() for _, blob in patches]))
    return dict(language=lang, repo=repo, commit=commit, subdir=subdir), patches, identity


def fetch(url):
    with urllib.request.urlopen(url, timeout=45) as response:
        return response.read()


def regular(root, path):
    resolved = path.resolve(strict=True)
    check(
        resolved == path and resolved.is_relative_to(root) and resolved.is_file(),
        "compiler input must be an ordinary file inside the grammar tree",
    )
    return resolved.read_bytes()


def isolated(image, cpu, mounts, working, command):
    opts = [
        "docker",
        "run",
        "--rm",
        "--network=none",
        "--read-only",
        "--cap-drop=ALL",
        "--security-opt=no-new-privileges",
        "--user",
        f"{os.getuid()}:{os.getgid()}",
        "--memory=3500m",
        "--memory-swap=3500m",
        "--cpus=1",
        "--cpuset-cpus",
        str(cpu),
        "--pids-limit=128",
        "--tmpfs",
        "/tmp",
        "-e",
        "HOME=/tmp",
        "-w",
        working,
    ]
    for source, dest, readonly in mounts:
        opts += ["--mount", f"type=bind,src={source},dst={dest}" + (",readonly" if readonly else "")]
    subprocess.run(opts + [image] + command, check=True, timeout=180)


def observe(upstream, root, role_revision, name, cold, pin, cli_archive, js_archive, image, cpu, workspace):
    lock, patches, identity = role_recipe(root, role_revision, name)
    for key, value in lock.items():
        check(cold["language" if key == "language" else "grammar_" + key] == value, "role lock mismatch")
    check(cold["grammar_patch_identity"] == identity, "role patch identity mismatch")
    commit, subdir = lock["commit"], lock["subdir"]
    dialect = subdir.split("/")[0]
    tree = git(upstream, "rev-parse", commit + "^{tree}").decode().strip()
    src = workspace / "source"
    src.mkdir()
    for rel in (
        "package-lock.json",
        "package.json",
        "tree-sitter.json",
        "common/define-grammar.js",
        "common/scanner.h",
        dialect + "/grammar.js",
        dialect + "/package.json",
        subdir + "/scanner.c",
    ):
        file = src / rel
        file.parent.mkdir(exist_ok=True, parents=True)
        file.write_bytes(git(upstream, "show", commit + ":" + rel))
    lockbytes = (src / "package-lock.json").read_bytes()
    packages = json.loads(lockbytes)["packages"]
    check(packages["node_modules/tree-sitter-cli"]["version"] == pin["version"], "generator version mismatch")
    js = packages["node_modules/tree-sitter-javascript"]
    integrity = "sha512-" + base64.b64encode(hashlib.sha512(js_archive).digest()).decode()
    check(js["integrity"] == integrity, "npm grammar package differs from lock")
    for _, blob in patches:
        # This recipe supports edits to existing grammar or scanner inputs only.
        files = subprocess.check_output(["git", "apply", "--numstat", "-z", "-"], input=blob, cwd=src)
        allowed = {
            "common/define-grammar.js",
            "common/scanner.h",
            dialect + "/grammar.js",
            subdir + "/scanner.c",
        }
        changed = [row.split(b"\t", 2)[-1].decode() for row in files.split(b"\0") if row]
        check(
            changed
            and set(changed) <= allowed
            and not re.search(
                rb"^(old mode|new mode|new file mode|deleted file mode|rename |copy )", blob, re.M
            ),
            "patch exceeds supported source recipe",
        )
        subprocess.run(
            ["git", "apply", "--whitespace=nowarn", "-"],
            cwd=src,
            input=blob,
            check=True,
            env={**os.environ, "GIT_CEILING_DIRECTORIES": str(workspace)},
        )
    check(regular(src, src / "package-lock.json") == lockbytes, "patch changes generator dependency lock")
    module = src / "node_modules/tree-sitter-javascript/grammar.js"
    module.parent.mkdir(parents=True)
    with tarfile.open(fileobj=io.BytesIO(js_archive)) as archive:
        item = archive.getmember("package/grammar.js")
        check(item.isfile(), "missing locked JavaScript grammar")
        module.write_bytes(archive.extractfile(item).read())
    check(sha(cli_archive) == pin["gzip_sha256"], "generator archive mismatch")
    cli = gzip.decompress(cli_archive)
    check(sha(cli) == pin["executable_sha256"], "generator mismatch")
    executable = workspace / "tree-sitter"
    executable.write_bytes(cli)
    executable.chmod(0o555)
    isolated(
        image,
        cpu,
        [(src, "/grammar", False), (executable, "/tools/tree-sitter", True)],
        "/grammar/" + dialect,
        ["/tools/tree-sitter", "generate"],
    )
    output = workspace / "compiled"
    output.mkdir()
    cmd = """set -eu
cc_path=$(command -v cc)
sha256sum "$cc_path" > /out/cc-before
for stem in parser scanner; do
 cc -std=c11 -fPIC -O2 -fno-strict-aliasing -fsigned-char -I . -MMD -MF /out/$stem.d -c $stem.c -o /out/$stem.o
done
cc -shared -fPIC -o /out/parser.so /out/parser.o /out/scanner.o
sha256sum "$cc_path" > /out/cc-after
cmp /out/cc-before /out/cc-after
"""
    isolated(
        image,
        cpu,
        [(src, "/grammar", True), (output, "/out", False)],
        "/grammar/" + subdir,
        ["sh", "-c", cmd],
    )
    cc_hash, cc_path = regular(output, output / "cc-before").decode().split()
    check(cc_path == cold["compiler_path"], "independent compiler path mismatch")
    sources = {}
    for stem in ("parser", "scanner"):
        deps = regular(output, output / (stem + ".d")).decode().replace("\\\n", " ").split(":", 1)[1]
        for rel in deps.split():
            check(not Path(rel).is_absolute(), "absolute compiler dependency")
            path = Path(os.path.normpath(src / subdir / rel))
            sources[str(path.relative_to(src))] = sha(regular(src, path))
    parser = subdir + "/parser.c"
    abi = int(re.search(rb"^#define LANGUAGE_VERSION (\d+)$", regular(src, src / parser), re.M)[1])
    artifact = sha(regular(output, output / "parser.so"))
    check(
        13 <= abi <= 15 and parser in sources and artifact == cold["grammar_artifact_sha256"],
        "independent compiler output differs from private oracle",
    )
    return dict(
        schema="w5-parent-source-replay-v1",
        role_revision=role_revision,
        locked=lock,
        patch_identity=identity,
        patches=[dict(file=n, sha256=sha(b)) for n, b in patches],
        tree=tree,
        package_lock_sha256=sha(lockbytes),
        generator_sha256=sha(cli),
        generator_version=pin["version"],
        compiler=[cc_path, cc_hash],
        image=image,
        abi=abi,
        sources=sources,
        artifact=artifact,
    )


def prepare(root, revision, base, trusted_revision, name, cold, image, cpu):
    pin = json.loads(git(base, "show", trusted_revision + ":" + PIN))
    for name_on_disk in ("w5_source_replay.py", "editor_latency_gate.py", "w5_private_oracle.py"):
        local = Path(__file__).resolve().parent / name_on_disk
        check(
            local.read_bytes() == git(base, "show", trusted_revision + ":scripts/" + name_on_disk),
            "patched-source mode requires the trusted common campaign implementation",
        )
    lock, _, _ = role_recipe(root, revision, name)
    with tempfile.TemporaryDirectory(prefix="w5-private-source-") as temp:
        directory = Path(temp).resolve()
        upstream = directory / "upstream"
        subprocess.run(["git", "init", "--bare", "--quiet", upstream], check=True)
        git(upstream, "fetch", "--quiet", "--depth=1", lock["repo"], lock["commit"])
        package_lock = json.loads(git(upstream, "show", lock["commit"] + ":package-lock.json"))
        js_url = package_lock["packages"]["node_modules/tree-sitter-javascript"]["resolved"]
        check(
            re.fullmatch(
                r"https://registry\.npmjs\.org/tree-sitter-javascript/-/tree-sitter-javascript-[0-9.]+\.tgz",
                js_url,
            ),
            "unknown JavaScript grammar dependency URL",
        )
        return observe(
            upstream,
            root,
            revision,
            name,
            cold,
            pin,
            fetch(pin["asset_url"]),
            fetch(js_url),
            image,
            cpu,
            directory,
        )


if __name__ == "__main__":
    if len(sys.argv) != 5 or sys.argv[1] != "recipe-changed":
        raise SystemExit("expected recipe-changed CHECKOUT BASE LANGUAGE")
    root, base, language = sys.argv[2:]
    # Compare identity from immutable role declarations. Unknown recipes defer
    # to the original strict gate; protect the launcher on that path as well.
    try:
        changed = role_recipe(root, base, language) != role_recipe(root, "HEAD", language)
    except (ValueError, subprocess.CalledProcessError):
        changed = True
    print("changed" if changed else "unchanged")
