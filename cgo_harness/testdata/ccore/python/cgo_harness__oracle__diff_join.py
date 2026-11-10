#!/usr/bin/env python3
"""Join two oracle-diff JSONL files (A = old oracle, B = new oracle).

usage: join.py A.jsonl B.jsonl joined.tsv summary.json
"""
import collections
import json
import sys


def load(path):
    rows = {}
    for line in open(path, encoding="utf-8"):
        line = line.strip()
        if not line:
            continue
        r = json.loads(line)
        key = (r["grammar"], r["class"], r["id"])
        if key in rows:
            raise ValueError(f"duplicate input key in {path}: {key}")
        rows[key] = r
    return rows


def g(r, k, d=""):
    return r.get(k, d) if r else d


def main():
    a = load(sys.argv[1])
    b = load(sys.argv[2])
    keys = sorted(set(a) | set(b))
    cols = [
        "grammar", "class", "id", "input_bytes",
        "a_c_digest", "b_c_digest", "c_comparable", "c_digest_changed",
        "a_c_has_error", "b_c_has_error", "c_has_error_flipped",
        "a_c_sexp_sha", "b_c_sexp_sha", "c_shape_changed", "c_span_or_flag_only_change",
        "a_c_root_type", "b_c_root_type", "c_root_type_changed",
        "a_c_nodes", "b_c_nodes", "a_c_err_nodes", "b_c_err_nodes", "a_c_missing", "b_c_missing",
        "a_c_error", "b_c_error",
        "go_digest_a", "go_digest_b", "go_comparable", "go_changed_between_runs", "go_has_error", "go_error",
        "a_go_equals_c", "b_go_equals_c", "parity_change",
        "a_div", "b_div",
    ]
    out = open(sys.argv[3], "w", encoding="utf-8")
    out.write("\t".join(cols) + "\n")
    summary = {
        "rows_a": len(a), "rows_b": len(b), "rows_joined": 0,
        "missing_in_a": 0, "missing_in_b": 0,
        "c_digest_changed": 0, "c_has_error_flipped": 0, "c_shape_changed": 0,
        "c_root_type_changed": 0, "c_span_or_flag_only_change": 0,
        "go_changed_between_runs": 0,
        "parity": collections.Counter(),
        "c_changed_by_grammar": collections.Counter(),
        "c_changed_by_class": collections.Counter(),
        "haserr_flipped_by_class": collections.Counter(),
        "shape_changed_by_class": collections.Counter(),
        "rows_by_class": collections.Counter(),
        "parity_by_class": collections.defaultdict(collections.Counter),
        "parity_by_grammar": collections.defaultdict(collections.Counter),
        "newly_differing": [],
        "newly_matching": [],
        "c_errors": collections.Counter(),
    }
    for k in keys:
        ra, rb = a.get(k), b.get(k)
        if ra is None:
            summary["missing_in_a"] += 1
            continue
        if rb is None:
            summary["missing_in_b"] += 1
            continue
        if not ra.get("input_sha256") or ra.get("input_sha256") != rb.get("input_sha256"):
            raise ValueError(f"input identity mismatch for {k}")
        summary["rows_joined"] += 1
        grammar, cls, rid = k
        summary["rows_by_class"][cls] += 1
        da, db = g(ra, "c_digest"), g(rb, "c_digest")
        ea, eb = g(ra, "c_has_error", False), g(rb, "c_has_error", False)
        sa, sb = g(ra, "c_sexp_sha256"), g(rb, "c_sexp_sha256")
        ta, tb = g(ra, "c_root_type"), g(rb, "c_root_type")
        errc_a, errc_b = g(ra, "c_error"), g(rb, "c_error")
        if errc_a or errc_b:
            summary["c_errors"][(errc_a[:60], errc_b[:60])] += 1
        c_valid = bool(da and db and not errc_a and not errc_b)
        digest_changed = c_valid and da != db
        flipped = c_valid and ea != eb
        shape = c_valid and bool(sa and sb) and sa != sb
        rootchg = c_valid and ta != tb
        spanflag = digest_changed and bool(sa and sb) and not shape
        gda, gdb = g(ra, "go_digest"), g(rb, "go_digest")
        go_valid = bool(gda and gdb and not g(ra, "go_error") and not g(rb, "go_error"))
        gochg = go_valid and gda != gdb
        eqa, eqb = bool(da and gda and da == gda), bool(db and gdb and db == gdb)
        if not c_valid or not go_valid:
            pc = "unavailable"
        elif eqa and eqb:
            pc = "still_matching"
        elif eqa and not eqb:
            pc = "newly_differing"
        elif not eqa and eqb:
            pc = "newly_matching"
        else:
            pc = "still_differing"
        for name, flag in (
            ("c_digest_changed", digest_changed), ("c_has_error_flipped", flipped),
            ("c_shape_changed", shape), ("c_root_type_changed", rootchg),
            ("c_span_or_flag_only_change", spanflag), ("go_changed_between_runs", gochg),
        ):
            if flag:
                summary[name] += 1
        if digest_changed:
            summary["c_changed_by_grammar"][grammar] += 1
            summary["c_changed_by_class"][cls] += 1
        if flipped:
            summary["haserr_flipped_by_class"][cls] += 1
        if shape:
            summary["shape_changed_by_class"][cls] += 1
        summary["parity"][pc] += 1
        summary["parity_by_class"][cls][pc] += 1
        summary["parity_by_grammar"][grammar][pc] += 1
        if pc == "newly_differing":
            summary["newly_differing"].append(list(k))
        if pc == "newly_matching":
            summary["newly_matching"].append(list(k))

        def div(r):
            c = g(r, "first_div_category")
            return f"{c}@{g(r, 'first_div_path')}" if c else ""

        vals = [
            grammar, cls, rid, g(ra, "input_bytes"),
            da, db, c_valid, digest_changed,
            ea, eb, flipped,
            sa, sb, shape, spanflag,
            ta, tb, rootchg,
            g(ra, "c_nodes", 0), g(rb, "c_nodes", 0), g(ra, "c_error_nodes", 0), g(rb, "c_error_nodes", 0),
            g(ra, "c_missing_nodes", 0), g(rb, "c_missing_nodes", 0),
            errc_a, errc_b,
            gda, gdb, go_valid, gochg, g(ra, "go_has_error", False), g(ra, "go_error"),
            eqa, eqb, pc, div(ra), div(rb),
        ]
        out.write("\t".join(str(v).replace("\t", " ").replace("\n", " ") for v in vals) + "\n")
    out.close()

    def plain(o):
        if isinstance(o, collections.Counter) or isinstance(o, dict):
            return {str(k): plain(v) for k, v in o.items()}
        if isinstance(o, list):
            return [plain(x) for x in o]
        return o

    json.dump(plain(summary), open(sys.argv[4], "w"), indent=1, sort_keys=True)
    print(json.dumps({k: plain(v) for k, v in summary.items() if k in (
        "rows_a", "rows_b", "rows_joined", "missing_in_a", "missing_in_b", "c_digest_changed",
        "c_has_error_flipped", "c_shape_changed", "c_root_type_changed", "c_span_or_flag_only_change",
        "go_changed_between_runs", "parity")}, indent=1))


if __name__ == "__main__":
    main()
