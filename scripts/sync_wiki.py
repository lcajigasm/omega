#!/usr/bin/env python3
"""Rebuild the wiki's manual pages from docs/manual. usage: wiki_sync.py <manual dir> <out dir>"""
import re, sys, os
PAGES = {"development.md": "How-it-works-and-development", "help.md": "Troubleshooting-and-FAQ", "install.md": "Install-and-set-up",
         "interface.md": "The-interface", "linux.md": "Linux", "mac.md": "Your-Mac", "models.md": "Models-and-cost",
         "reference.md": "Reference", "safety.md": "Safety-approvals-and-memory", "tasks.md": "Tasks-and-the-guided-run",
         "tools.md": "Your-tools", "windows.md": "Windows", "README.md": "Home"}
RAW = "https://raw.githubusercontent.com/lcajigasm/omega/main/docs/"
BLOB = "https://github.com/lcajigasm/omega/blob/main/"
def link(m):
    text, url = m.group(1), m.group(2)
    path, _, frag = url.partition("#")
    frag = "#" + frag if frag else ""
    if path in PAGES: return f"{text}({PAGES[path]}{frag})"
    if path == "../../README.md": return f"{text}(https://github.com/lcajigasm/omega{frag or '#readme'})"
    if path == "../ROADMAP.md": return f"{text}(Roadmap{frag})"
    if path.startswith("../assets/"): return f"{text}({RAW}{path[3:]})"
    if path.startswith("../../"): return f"{text}({BLOB}{path[6:]}{frag})"
    if path.startswith("../"): return f"{text}({BLOB}docs/{path[3:]}{frag})"
    return m.group(0)
src, out = sys.argv[1], sys.argv[2]
for name, page in PAGES.items():
    if name == "README.md": continue
    lines = open(os.path.join(src, name)).read().split("\n")
    assert lines[0].startswith("# ") and "›" in lines[2], name
    body = "\n".join(lines[4:])
    body = re.sub(r"(!?\[[^\]]*\])\(([^)\s]+)\)", link, body)
    open(os.path.join(out, page + ".md"), "w").write(body)
