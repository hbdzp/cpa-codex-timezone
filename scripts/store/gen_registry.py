#!/usr/bin/env python3
"""Generate a CPA plugin store registry.json for self-hosted downloads.

Scans --dist for codex-timezone_<version>_<goos>_<goarch>.zip files and emits a
schema_version 2 registry with a "direct" install plan: one artifact per
platform, each with its download URL, sha256 and size. CPA verifies the sha256
before installing.

    python3 scripts/store/gen_registry.py --dist dist --version 0.1.0 \
        --base-url https://example.com/cpa-plugins/codex-timezone/0.1.0 \
        --out dist/registry.json

If --merge points at an existing registry.json, the codex-timezone entry is
added or replaced and every other plugin in it is kept. The previous
codex-timezone version is moved into "versions" so it stays installable.
"""
import argparse
import hashlib
import json
import pathlib
import re
import sys

PLUGIN = {
    "id": "codex-timezone",
    "name": "Codex 时区",
    "description": "在 CPA 把请求发往 Codex / GPT 模型前，改写请求中的时区与当前日期，并提供中文 Web 配置界面。",
    "author": "cpa-codex-timezone",
    "tags": ["Codex", "Timezone", "Request Normalizer", "Management"],
}


def artifacts(dist: pathlib.Path, version: str, base_url: str):
    pattern = re.compile(rf"^{re.escape(PLUGIN['id'])}_{re.escape(version)}_([a-z0-9]+)_([a-z0-9]+)\.zip$")
    out = []
    for path in sorted(dist.iterdir()):
        m = pattern.match(path.name)
        if not m:
            continue
        data = path.read_bytes()
        out.append({
            "goos": m.group(1),
            "goarch": m.group(2),
            "url": f"{base_url.rstrip('/')}/{path.name}",
            "sha256": hashlib.sha256(data).hexdigest(),
            "size": len(data),
        })
    return out


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dist", default="dist")
    ap.add_argument("--version", required=True)
    ap.add_argument("--base-url", required=True, help="URL prefix where the zip files are served")
    ap.add_argument("--out", default="dist/registry.json")
    ap.add_argument("--merge", help="existing registry.json to update instead of starting fresh")
    ap.add_argument("--repository", default="", help="optional source repository URL shown in the store")
    ap.add_argument("--homepage", default="")
    args = ap.parse_args()

    if not args.base_url.startswith(("http://", "https://")):
        print("--base-url must start with http:// or https://", file=sys.stderr)
        return 1
    arts = artifacts(pathlib.Path(args.dist), args.version, args.base_url)
    if not arts:
        print(f"no {PLUGIN['id']}_{args.version}_<goos>_<goarch>.zip found in {args.dist}; run `make package` first", file=sys.stderr)
        return 1

    registry = {"schema_version": 2, "plugins": []}
    if args.merge:
        registry = json.loads(pathlib.Path(args.merge).read_text(encoding="utf-8"))
        registry["schema_version"] = 2

    entry = dict(PLUGIN, version=args.version, install={"type": "direct", "artifacts": arts})
    if args.repository:
        entry["repository"] = args.repository
    if args.homepage:
        entry["homepage"] = args.homepage

    plugins = registry.setdefault("plugins", [])
    previous = next((p for p in plugins if p.get("id") == PLUGIN["id"]), None)
    if previous:
        history = [v for v in previous.get("versions", []) if v.get("version") != args.version]
        if previous.get("version") and previous["version"] != args.version and previous.get("install"):
            history.insert(0, {"version": previous["version"], "install": previous["install"]})
        if history:
            entry["versions"] = history
        plugins[plugins.index(previous)] = entry
    else:
        plugins.append(entry)

    out = pathlib.Path(args.out)
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(registry, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(f"wrote {out} ({len(arts)} platform(s): {', '.join(a['goos'] + '/' + a['goarch'] for a in arts)})")
    return 0


if __name__ == "__main__":
    sys.exit(main())
