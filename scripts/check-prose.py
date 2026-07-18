#!/usr/bin/env python3
"""Enforce semantic line breaks in prose.

The gokit baseline (.github/copilot-instructions.md, Documentation section) wraps prose at a
100-column soft ceiling by breaking only at meaningful boundaries: sentences first, then clauses
(after a comma/semicolon/colon, around an em or en dash, or before a coordinating conjunction). A
clause with no legal break point may exceed the ceiling rather than break mid-clause. The rule
applies identically to Markdown, doc.go/godoc, and // code comments. This tool checks and fixes
that invariant across both surfaces.

Usage:
    scripts/check-prose.py [--fix] [paths...]

Default paths are all tracked Markdown and non-test Go files. Without --fix the tool lists files
that violate the standard and exits non-zero; with --fix it reflows them in place. Structure that is
not paragraph prose is preserved verbatim: fenced/indented code blocks, tables, headings, list and
blockquote layout, YAML frontmatter, godoc code blocks, decorative dividers, and code-example
lines inside comments.
"""

from __future__ import annotations

import re
import subprocess
import sys

from wrap_semantic import wrap_semantic

# --- Markdown -----------------------------------------------------------------------------------

_MD_FENCE = re.compile(r"^\s*(```|~~~)")
_MD_HEADING = re.compile(r"^\s*#{1,6}\s")
_MD_HR = re.compile(r"^\s*([-*_])\1{2,}\s*$")
_MD_LIST = re.compile(r"^(\s*)([-*+]|\d+[.)])\s+(.*)$")
_MD_TABLE = re.compile(r"^\s*\|")
_MD_BQ = re.compile(r"^(\s*>+\s?)(.*)$")
_MD_INDENT_CODE = re.compile(r"^(\s{4,}|\t)")
_MD_LINK = r"(?:\[!\[[^\]]*\]\([^)]*\)\]\([^)]*\)|!?\[[^\]]*\]\([^)]*\)|<[^>\s]+>)"
_MD_LINK_ONLY = re.compile(rf"^\s*{_MD_LINK}(?:\s*(?:·|\|)?\s*{_MD_LINK})*\s*$")
_MD_LINK_DEF = re.compile(r"^\s*\[[^\]]+\]:\s")


def reflow_markdown(text: str) -> str:
    lines = text.split("\n")
    out: list[str] = []
    n = len(lines)
    i = 0

    if lines and lines[0].strip() == "---":
        out.append(lines[0])
        i = 1
        while i < n and lines[i].strip() != "---":
            out.append(lines[i])
            i += 1
        if i < n:
            out.append(lines[i])
            i += 1

    buf: str | None = None
    buf_prefix = ""
    buf_type: str | None = None

    def flush() -> None:
        nonlocal buf, buf_prefix, buf_type
        if buf is not None:
            if buf_type == "list":
                first, cont = buf_prefix, " " * len(buf_prefix)
            elif buf_type == "bq":
                first, cont = buf_prefix, buf_prefix
            else:
                first, cont = buf_prefix, buf_prefix
            out.extend(wrap_semantic(buf, first, cont))
        buf, buf_prefix, buf_type = None, "", None

    while i < n:
        line = lines[i]

        if _MD_FENCE.match(line):
            flush()
            fence = line.strip()[:3]
            out.append(line)
            i += 1
            while i < n:
                out.append(lines[i])
                closed = lines[i].strip().startswith(fence)
                i += 1
                if closed:
                    break
            continue

        stripped = line.strip()
        if stripped == "":
            flush()
            out.append(line)
            i += 1
            continue

        if (
            _MD_HEADING.match(line)
            or _MD_HR.match(line)
            or _MD_TABLE.match(line)
            or _MD_LINK_ONLY.match(line)
            or _MD_LINK_DEF.match(line)
        ):
            flush()
            out.append(line)
            i += 1
            continue

        if _MD_INDENT_CODE.match(line) and buf_type in (None, "para"):
            flush()
            out.append(line)
            i += 1
            continue

        lm = _MD_LIST.match(line)
        if lm:
            flush()
            buf_prefix = f"{lm.group(1)}{lm.group(2)} "
            buf = lm.group(3).rstrip()
            buf_type = "list"
            i += 1
            continue

        bm = _MD_BQ.match(line)
        if bm:
            prefix, content = bm.group(1), bm.group(2)
            if buf_type == "bq":
                if content.strip() == "":
                    flush()
                    out.append(line)
                else:
                    buf = (buf + " " + content.strip()).strip()
            else:
                flush()
                buf_prefix, buf, buf_type = prefix, content.strip(), "bq"
            i += 1
            continue

        if buf_type in ("para", "list", "bq"):
            buf = (buf + " " + stripped).strip()
        else:
            buf, buf_prefix, buf_type = stripped, "", "para"
        i += 1

    flush()
    return "\n".join(out)


# --- Go comments --------------------------------------------------------------------------------

_GO_DIRECTIVE = re.compile(r"^//[a-z][a-z0-9]*:")
_GO_COMMENT = re.compile(r"^(\s*)//(.*)$")
_GO_LIST = re.compile(r"^(\s*(?:[-*]|\d+[.)])\s+)(.*)$")
_GO_CODE_SIG = re.compile(
    r"(:=|=>|^\s*(import|func|return|type|var|const|package|if|for|go|defer|range|switch|case|else)\b|\{\s*$|^\s*\}|\)\s*\{)"
)
_GO_RULE = re.compile(r"^[\-=_~*.\u2500-\u257F\u2014\u2013 ]+$")


def _go_text(raw: str) -> str:
    return raw[1:] if raw.startswith(" ") else raw


def _go_is_code(raw: str) -> bool:
    return _go_text(raw).startswith("\t") or raw.startswith("    ")


def _go_is_heading(raw: str) -> bool:
    return _go_text(raw).startswith("# ")


def _go_is_divider(raw: str) -> bool:
    t = _go_text(raw).strip()
    return len(t) >= 3 and _GO_RULE.match(t) is not None and any(c in t for c in "-=_~*\u2500\u2014\u2013")


def _go_looks_code(t: str) -> bool:
    return bool(_GO_CODE_SIG.search(t))


def _reflow_go_block(indent: str, raws: list[str]) -> list[str]:
    res: list = []
    run: list[str] = []

    def flush_run() -> None:
        nonlocal run
        if not run:
            return
        if any(_go_looks_code(_go_text(r).strip()) for r in run):
            for r in run:
                res.append(f"{indent}//{r}")
        else:
            joined = " ".join(_go_text(r).strip() for r in run).strip()
            prefix = f"{indent}// "
            res.extend(wrap_semantic(joined, prefix, prefix))
        run = []

    for raw in raws:
        t = _go_text(raw)
        if t.strip() == "":
            flush_run()
            res.append(f"{indent}//")
            continue
        if _go_is_code(raw) or _go_is_heading(raw) or _go_is_divider(raw):
            flush_run()
            res.append(f"{indent}//{raw}")
            continue
        lm = _GO_LIST.match(t)
        if lm:
            flush_run()
            res.append(("LIST", indent, lm.group(1), lm.group(2).strip()))
            continue
        run.append(raw)
    flush_run()

    final: list[str] = []
    for x in res:
        if isinstance(x, tuple):
            first = f"{x[1]}// {x[2]}"
            cont = f"{x[1]}// " + " " * len(x[2])
            final.extend(wrap_semantic(x[3], first, cont))
        else:
            final.append(x)
    return final


def reflow_go(text: str) -> str:
    lines = text.split("\n")
    out: list[str] = []
    i, n = 0, len(lines)
    while i < n:
        m = _GO_COMMENT.match(lines[i])
        if not m or _GO_DIRECTIVE.match(lines[i].lstrip()):
            out.append(lines[i])
            i += 1
            continue
        indent = m.group(1)
        block: list[str] = []
        while i < n:
            mm = _GO_COMMENT.match(lines[i])
            if not mm or mm.group(1) != indent or _GO_DIRECTIVE.match(lines[i].lstrip()):
                break
            block.append(mm.group(2))
            i += 1
        out.extend(_reflow_go_block(indent, block))
    return "\n".join(out)


# --- Driver -------------------------------------------------------------------------------------


def default_paths() -> list[str]:
    tracked = subprocess.run(
        ["git", "ls-files", "*.md", "*.go"], capture_output=True, text=True, check=True
    ).stdout.split("\n")
    return [p for p in tracked if p and not p.endswith("_test.go")]


def reflow_for(path: str, text: str) -> str:
    if path.endswith(".md"):
        return reflow_markdown(text)
    return reflow_go(text)


def main(argv: list[str]) -> int:
    fix = "--fix" in argv
    paths = [a for a in argv if not a.startswith("--")] or default_paths()

    offenders: list[str] = []
    for path in paths:
        try:
            with open(path, encoding="utf-8") as fh:
                original = fh.read()
        except (OSError, UnicodeDecodeError):
            continue
        reflowed = reflow_for(path, original)
        if reflowed == original:
            continue
        offenders.append(path)
        if fix:
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(reflowed)

    if fix:
        for path in offenders:
            print(f"reflowed: {path}")
        return 0

    if offenders:
        print("Prose not in semantic line breaks (run scripts/check-prose.py --fix):", file=sys.stderr)
        for path in offenders:
            print(f"  {path}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
