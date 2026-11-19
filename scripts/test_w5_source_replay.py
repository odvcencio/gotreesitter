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
