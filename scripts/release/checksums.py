#!/usr/bin/env python3
"""Write a sha256sum-style checksums.txt covering every zip in a directory."""
import hashlib
import pathlib
import sys

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else "dist")
zips = sorted(root.glob("*.zip"))
if not zips:
    sys.exit(f"no zip files in {root}")
lines = [f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n" for p in zips]
(root / "checksums.txt").write_text("".join(lines), encoding="utf-8")
print("".join(lines), end="")
