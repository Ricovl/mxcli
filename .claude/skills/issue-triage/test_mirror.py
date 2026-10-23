#!/usr/bin/env python3
"""Scenario tests for mirror.py against a fake GitHub API (no network).

  python3 -I .claude/skills/issue-triage/test_mirror.py

Covers the 2026-10-22 incident, where a short listing of existing mirrors made
mirror.py re-create 40 of them, and the duplicate-tracking bug it exposed.
"""
import contextlib, importlib.util, io, json, os, re, sys
sys.dont_write_bytecode = True  # loading mirror.py must not leave __pycache__ in the skill dir
UP, FORK = "up/repo", "me/repo"

def load(path):
    spec = importlib.util.spec_from_file_location("mirror_" + str(id(path)), path)
    mod = importlib.util.module_from_spec(spec); spec.loader.exec_module(mod)
    mod.time.sleep = lambda s: None
    return mod

def mirror(fork_no, up_no, state="open", updated="T"):
    return {"number": fork_no, "state": state, "labels": [{"name": "mirror"}],
            "body": f"<!-- upstream: {UP}#{up_no} updated={updated} -->\nbody"}

def run(path, up_open, fork_mirrors, glitch=lambda page, calls: False, args=()):
    """glitch(page, call_index) -> True makes that fork-listing page come back empty."""
    mod = load(path)
    calls = {"fork_pages": 0}
    up_issues = [{"number": n, "title": f"t{n}", "body": "b", "updated_at": "T", "created_at": "2026-01-01",
                  "comments": 0, "labels": [], "user": {"login": "u"}} for n in up_open]
    # the fork lists newest first, like GitHub
    fork_sorted = sorted(fork_mirrors, key=lambda m: -m["number"])
    def api(path, method="GET", data=None, ok_codes=()):
        if method != "GET":
            return {}
        m = re.search(r"[?&]page=(\d+)", path); page = int(m.group(1)) if m else 1
        if path.startswith(f"/repos/{UP}/issues?"):
            return up_issues[(page-1)*100: page*100]
        if path.startswith(f"/repos/{FORK}/issues?") and "labels=mirror" in path:
            calls["fork_pages"] += 1
            if glitch(page, calls["fork_pages"]):
                return []
            return fork_sorted[(page-1)*100: page*100]
        if path.startswith(f"/repos/{FORK}/pulls"):
            return []
        if path.startswith(f"/repos/{UP}/issues/"):
            return {"state": "open"}
        raise AssertionError(path)
    mod.api = api
    out = io.StringIO(); code = 0
    sys.argv = ["mirror.py", "--upstream", UP, "--fork", FORK, "--dry-run", *args]
    with contextlib.redirect_stdout(out), contextlib.redirect_stderr(io.StringIO()) as err:
        try:
            mod.main()
        except SystemExit as e:
            code = e.code if isinstance(e.code, int) else 1
            err.write(str(e.code))
    lines = out.getvalue().strip().splitlines()
    summary = json.loads(lines[-1]) if lines and code == 0 else None
    return code, summary, err.getvalue()

def scenarios(path):
    res = {}
    ups = list(range(1, 251))
    fork = [mirror(1000 + n, n) for n in ups]            # 250 mirrors = 3 pages
    # A: page 3 empty once (transient) -> recovered, nothing created
    code, s, _ = run(path, ups, fork, glitch=lambda p, c: p == 3 and c == 3)
    res["A transient empty page recovers"] = code == 0 and s["created"] == []
    # B: page 3 empty persistently -> must refuse, never create duplicates
    code, s, _ = run(path, ups, fork, glitch=lambda p, c: p == 3)
    res["B short listing refused"] = code != 0
    # C: whole listing empty -> must refuse
    code, s, _ = run(path, ups, fork, glitch=lambda p, c: c == 1)
    res["C empty listing refused"] = code != 0
    # D: duplicate pair, newer copy closed -> refresh the open original
    code, s, _ = run(path, [7], [mirror(10, 7, "open", updated="OLD"), mirror(50, 7, "closed", updated="OLD")])
    res["D open original tracked"] = code == 0 and s["refreshed"] == [7]
    # E: fresh fork seeds normally, oldest first, capped
    code, s, _ = run(path, ups, [])
    res["E fresh seed works"] = code == 0 and s["created"] == list(range(1, 41))
    # F: new upstream issues beyond the newest mirrored are created normally
    code, s, _ = run(path, ups + [251, 252], fork)
    res["F new issues still mirrored"] = code == 0 and s["created"] == [251, 252]
    # G: backfill allowed explicitly
    code, s, _ = run(path, ups, fork, glitch=lambda p, c: p == 3, args=["--allow-backfill"])
    res["G --allow-backfill overrides"] = code == 0 and len(s["created"]) == 40
    return res

if __name__ == "__main__":
    here = os.path.dirname(os.path.abspath(__file__))
    results = scenarios(os.path.join(here, "mirror.py"))
    for k, v in results.items():
        print(("PASS " if v else "FAIL ") + k)
    sys.exit(0 if all(results.values()) else 1)
