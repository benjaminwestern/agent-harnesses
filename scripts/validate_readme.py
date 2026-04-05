from __future__ import annotations

import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
README = ROOT / "README.md"

GENERATED_ASSETS = [
    "assets/architecture.d2",
    "assets/banner.svg",
    "assets/header-overview.svg",
    "assets/header-install.svg",
    "assets/header-runtimes.svg",
    "assets/header-contract.svg",
    "assets/header-bindings.svg",
    "assets/header-architecture.svg",
    "assets/header-debug.svg",
    "assets/header-scripts.svg",
    "assets/header-repository.svg",
    "assets/architecture.svg",
]

REQUIRED_PATHS = [
    "README.md",
    "go.mod",
    "mise.toml",
    "hk.pkl",
    "cmd/agent-control/main.go",
    "cmd/agent-harness/main.go",
    "internal/harness/harness.go",
    "internal/harness/install.go",
    "internal/harness/run.go",
    "pkg/contract/harness.go",
    "pkg/contract/controlplane.go",
    "pkg/controlplane/types.go",
    "scripts/generate_assets.py",
    "scripts/validate_readme.py",
    "docs/contract.md",
    "docs/control-plane.md",
    "docs/integration.md",
    "docs/codex.md",
    "docs/gemini.md",
    "docs/claude.md",
    "docs/opencode.md",
    *GENERATED_ASSETS,
]

MARKDOWN_LINK_PATTERN = re.compile(r"\]\(([^)]+)\)")
IMAGE_SRC_PATTERN = re.compile(r'src="([^"]+)"')


def normalize_target(target: str) -> str | None:
    clean_target = target.strip()
    if not clean_target:
        return None
    if clean_target.startswith(("http://", "https://", "mailto:", "#")):
        return None
    clean_target = clean_target.split("#", 1)[0]
    if clean_target.startswith("./"):
        clean_target = clean_target[2:]
    return clean_target or None


def main() -> int:
    if not README.exists():
        print(f"ERROR: missing README: {README}", file=sys.stderr)
        return 1

    text = README.read_text(encoding="utf-8")
    errors: list[str] = []

    for relative_path in REQUIRED_PATHS:
        target = ROOT / relative_path
        if not target.exists():
            errors.append(f"missing required path: {relative_path}")

    referenced_targets: set[str] = set()

    for pattern in (MARKDOWN_LINK_PATTERN, IMAGE_SRC_PATTERN):
        for raw_target in pattern.findall(text):
            normalized = normalize_target(raw_target)
            if normalized is None:
                continue
            referenced_targets.add(normalized)
            target = ROOT / normalized
            if not target.exists():
                errors.append(f"README references missing path: {normalized}")

    for asset in GENERATED_ASSETS:
        if asset not in referenced_targets:
            errors.append(f"README does not reference generated asset: {asset}")

    required_snippets = [
        "`scripts/generate_assets.py`",
        "`scripts/validate_readme.py`",
        "[Codex CLI quickstart and install](https://github.com/openai/codex#quickstart)",
        "[Codex hooks](https://developers.openai.com/codex/hooks)",
        "[Gemini CLI installation](https://geminicli.com/docs/get-started/installation/)",
        "[Gemini CLI hooks reference](https://geminicli.com/docs/hooks/reference/)",
        "[Claude Code setup](https://docs.claude.com/en/docs/claude-code/setup)",
        "[Claude Code hooks reference](https://code.claude.com/docs/en/hooks)",
        "[OpenCode install guide](https://opencode.ai/docs/)",
        "[OpenCode plugins](https://opencode.ai/docs/plugins/)",
    ]
    for snippet in required_snippets:
        if snippet not in text:
            errors.append(f"README does not document required reference: {snippet}")

    if errors:
        for error in errors:
            print(f"ERROR: {error}", file=sys.stderr)
        return 1

    print("README validation passed.")
    print(f"- validated paths: {len(REQUIRED_PATHS)}")
    print(f"- local README references checked: {len(referenced_targets)}")
    print(f"- generated assets referenced: {len(GENERATED_ASSETS)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
