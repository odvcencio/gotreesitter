import copy
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import editor_latency_gate as gate
import w5_source_replay as replay


class ReplayTests(unittest.TestCase):
    def test_bind_only_a_changed_patched_role(self):
        lock = dict(grammar_repo=replay.RECIPE_REPO, grammar_commit="a" * 40, grammar_subdir="tsx/src")
        cold = dict(lock, language="tsx", grammar_patch_identity="0" * 64, grammar_artifact_sha256="1" * 64)
        roles = (("base", "a" * 40, "base-source"), ("head", "b" * 40, "head-source"))
        with tempfile.TemporaryDirectory() as tmp:
            dirs = [Path(tmp) / r for r, _, _ in roles]
            for d in dirs:
                d.mkdir()
            compilers = [["/usr/bin/cc", "4" * 64]]
            expected = dict(
                oracle=cold, fresh_proof=None, artifact="1" * 64, image_id="image", compilers=compilers
            )
            receipts = {
                "base": (dirs[0], [], copy.deepcopy(expected)),
                "head": (dirs[1], [], copy.deepcopy(expected)),
            }
            with mock.patch.object(replay, "prepare") as produce:
                gate.bind_changed_patch_roles(roles, receipts, "base-source", "a" * 40, "tsx", "image", 1)
                produce.assert_not_called()
                receipts["base"][2]["oracle"]["grammar_patch_identity"] = "none"
                receipts["base"][2]["fresh_proof"] = {"mode": "committed-parser-no-patch-no-regeneration"}
                produce.return_value = dict(
                    schema="w5-parent-source-replay-v1",
                    role_revision="b" * 40,
                    image="image",
                    compiler=compilers[0],
                    artifact="1" * 64,
                    patch_identity="0" * 64,
                    locked=dict(language="tsx", repo=replay.RECIPE_REPO, commit="a" * 40, subdir="tsx/src"),
                )
                gate.bind_changed_patch_roles(roles, receipts, "base-source", "a" * 40, "tsx", "image", 1)
                produce.assert_called_once()
                self.assertFalse((dirs[0] / "parent-source-replay.json").exists())
                e = receipts["head"][2]
                gate.verify_source_replay(e, "head", "b" * 40)
                for key, value in (
                    ("role", "base"),
                    ("artifact", "2" * 64),
                    ("compiler", ["other", "3" * 64]),
                    ("patch_identity", "none"),
                    ("sources", {"foreign.h": "4" * 64}),
                ):
                    changed = copy.deepcopy(e)
                    changed["source_replay"][key] = value
                    with self.subTest(key=key), self.assertRaises((ValueError, SystemExit, RuntimeError)):
                        gate.verify_source_replay(changed, "head", "b" * 40)

    def test_recipe_identity_binds_order_and_role_patch_bytes(self):
        lock = b"tsx https://github.com/tree-sitter/tree-sitter-typescript " + b"a" * 40 + b" tsx/src .tsx\n"
        spec = b'"tsx": {\n File: "one.patch",\n RegenerateParser: true,\n ExtraFile: "two.patch",\n },\n'
        with mock.patch.object(replay, "git", side_effect=[lock, spec, b"one", b"two"]):
            _, patches, identity = replay.role_recipe("root", "role", "tsx")
        self.assertEqual([p[0] for p in patches], ["one.patch", "two.patch"])
        fields = b'{"File":"one.patch","RegenerateParser":true,"GeneratorVersion":"","ExtraFile":"two.patch"}'
        self.assertEqual(
            identity,
            replay.sha(b"\0".join([fields, replay.sha(b"one").encode(), replay.sha(b"two").encode()])),
        )
        with mock.patch.object(replay, "git", side_effect=[lock, spec.replace(b"true", b"compute()")]):
            with self.assertRaises(ValueError):
                replay.role_recipe("root", "role", "tsx")

    def test_scanner_only_recipe_identity_matches_go_spec(self):
        repo = b"https://github.com/tree-sitter/tree-sitter-javascript"
        lock = b"javascript " + repo + b" " + b"a" * 40 + b" src .js\n"
        spec = b'"javascript": {\n File: "one.patch",\n },\n'
        with mock.patch.object(replay, "git", side_effect=[lock, spec, b"one"]):
            got_lock, patches, identity = replay.role_recipe("root", "role", "javascript")
        self.assertTrue(replay.scanner_only(got_lock))
        self.assertEqual([p[0] for p in patches], ["one.patch"])
        # json.Marshal of grammarpatch.Spec{File: "one.patch"}, then the patch digest.
        fields = b'{"File":"one.patch","RegenerateParser":false,"GeneratorVersion":""}'
        self.assertEqual(identity, replay.sha(b"\0".join([fields, replay.sha(b"one").encode()])))
        for bad in (spec.replace(b' },', b' RegenerateParser: true,\n },'),
                    spec.replace(b' },', b' ExtraFile: "two.patch",\n },')):
            with self.subTest(spec=bad), mock.patch.object(replay, "git", side_effect=[lock, bad, b"one", b"two"]):
                with self.assertRaisesRegex(ValueError, "unsupported scanner-only recipe"):
                    replay.role_recipe("root", "role", "javascript")
        other = b"javascript https://github.com/example/tree-sitter-javascript " + b"a" * 40 + b" src .js\n"
        with mock.patch.object(replay, "git", side_effect=[other, spec]):
            with self.assertRaisesRegex(ValueError, "unsupported upstream generation recipe"):
                replay.role_recipe("root", "role", "javascript")
        wrong_subdir = lock.replace(b" src ", b" grammar/src ")
        with mock.patch.object(replay, "git", side_effect=[wrong_subdir, spec]):
            with self.assertRaisesRegex(ValueError, "unsupported upstream generation recipe"):
                replay.role_recipe("root", "role", "javascript")

    def test_scanner_only_replay_patches_only_the_committed_scanner(self):
        import subprocess
        with tempfile.TemporaryDirectory() as tmp:
            upstream = Path(tmp) / "upstream"
            (upstream / "src" / "tree_sitter").mkdir(parents=True)
            (upstream / "src" / "parser.c").write_text("int parser;\n")
            (upstream / "src" / "scanner.c").write_text("int scanner;\n")
            (upstream / "src" / "tree_sitter" / "parser.h").write_text("/* header */\n")
            (upstream / "grammar.js").write_text("module.exports = {};\n")
            run = lambda *args: subprocess.run(["git", "-C", str(upstream), *args], check=True, capture_output=True)
            run("init", "--quiet")
            run("add", ".")
            run("-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "--quiet", "-m", "pin")
            commit = subprocess.check_output(["git", "-C", str(upstream), "rev-parse", "HEAD"], text=True).strip()
            scanner_patch = (b"--- a/src/scanner.c\n+++ b/src/scanner.c\n@@ -1 +1 @@\n"
                             b"-int scanner;\n+int scanner_patched;\n")
            parser_patch = (b"--- a/src/parser.c\n+++ b/src/parser.c\n@@ -1 +1 @@\n"
                            b"-int parser;\n+int parser_patched;\n")
            lock = dict(language="javascript", repo="https://github.com/tree-sitter/tree-sitter-javascript",
                        commit=commit, subdir="src")
            cold = dict(language="javascript", grammar_repo=lock["repo"], grammar_commit=commit,
                        grammar_subdir="src", grammar_patch_identity="0" * 64)
            seen = {}

            def compiled(src, subdir, cold_arg, image, cpu, workspace):
                seen["scanner"] = (src / subdir / "scanner.c").read_text()
                seen["parser"] = (src / subdir / "parser.c").read_text()
                seen["header"] = (src / subdir / "tree_sitter" / "parser.h").is_file()
                seen["grammar"] = (src / "grammar.js").exists()
                return dict(compiler=["/usr/bin/cc", "1" * 64], abi=14, sources={}, artifact="2" * 64)

            with mock.patch.object(replay, "role_recipe", return_value=(lock, [("one.patch", scanner_patch)], "0" * 64)), \
                    mock.patch.object(replay, "compile_bound", side_effect=compiled):
                workspace = Path(tmp) / "ok"
                workspace.mkdir()
                got = replay.observe_scanner_only(upstream, "root", "b" * 40, "javascript", cold, "image", 0, workspace)
            self.assertEqual(seen, dict(scanner="int scanner_patched;\n", parser="int parser;\n", header=True, grammar=False))
            self.assertEqual((got["recipe"], got["patch_identity"], got["artifact"], got["locked"]),
                             ("scanner-only", "0" * 64, "2" * 64, lock))
            with mock.patch.object(replay, "role_recipe", return_value=(lock, [("one.patch", parser_patch)], "0" * 64)), \
                    mock.patch.object(replay, "compile_bound", side_effect=compiled):
                workspace = Path(tmp) / "bad"
                workspace.mkdir()
                with self.assertRaisesRegex(ValueError, "patch exceeds supported source recipe"):
                    replay.observe_scanner_only(upstream, "root", "b" * 40, "javascript", cold, "image", 0, workspace)
            with mock.patch.object(replay, "role_recipe", return_value=(lock, [("one.patch", scanner_patch)], "1" * 64)):
                workspace = Path(tmp) / "identity"
                workspace.mkdir()
                with self.assertRaisesRegex(ValueError, "role patch identity mismatch"):
                    replay.observe_scanner_only(upstream, "root", "b" * 40, "javascript", cold, "image", 0, workspace)
