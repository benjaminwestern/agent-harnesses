from __future__ import annotations

from html import escape
from pathlib import Path
from textwrap import dedent

ROOT = Path(__file__).resolve().parents[1]
ASSETS_DIR = ROOT / "assets"

PALETTE = {
    "paper": "#F4F1EA",
    "paper_alt": "#FCFAF6",
    "ink": "#101721",
    "ink_alt": "#314252",
    "line": "#D7CCBD",
    "shadow": "#C2B29D",
    "text": "#FFF9F0",
    "muted": "#D9D0C4",
    "dark_text": "#20303D",
    "accent": "#C46B38",
    "teal": "#2E7074",
    "blue": "#4C6F94",
    "gold": "#B48B34",
    "sage": "#5C725A",
    "brick": "#9A4E45",
    "slate": "#506377",
}

FONT_MONO = (
    '"JetBrainsMono Nerd Font", "Hack Nerd Font", "CaskaydiaMono Nerd Font", '
    '"SFMono-Regular", Menlo, Monaco, Consolas, "Liberation Mono", monospace'
)


def write_asset(filename: str, content: str) -> None:
    (ASSETS_DIR / filename).write_text(content, encoding="utf-8")


def layered_card(
    x: int,
    y: int,
    width: int,
    height: int,
    radius: int,
    fill: str,
    *,
    stroke: str,
    shadow_dx: int = 8,
    shadow_dy: int = 8,
) -> str:
    return dedent(
        f"""\
        <rect x="{x + shadow_dx}" y="{y + shadow_dy}" width="{width}" height="{height}" rx="{radius}" fill="{PALETTE["shadow"]}" />
        <rect x="{x}" y="{y}" width="{width}" height="{height}" rx="{radius}" fill="{fill}" stroke="{stroke}" stroke-width="2" />
        """
    )


def create_banner() -> str:
    return dedent(
        f"""\
        <svg width="1200" height="360" viewBox="0 0 1200 360" fill="none" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <pattern id="ledger" width="48" height="48" patternUnits="userSpaceOnUse">
              <path d="M 48 0 L 0 0 0 48" fill="none" stroke="{PALETTE["line"]}" stroke-width="1" />
            </pattern>
            <clipPath id="frame-clip">
              <rect width="1200" height="360" rx="28" />
            </clipPath>
            <style><![CDATA[
              .badge {{ font-family: {FONT_MONO}; font-size: 13px; font-weight: 700; fill: {PALETTE["dark_text"]}; }}
              .eyebrow {{ font-family: {FONT_MONO}; font-size: 15px; font-weight: 700; letter-spacing: 0.14em; fill: {PALETTE["accent"]}; }}
              .title {{ font-family: {FONT_MONO}; font-size: 38px; font-weight: 700; fill: {PALETTE["text"]}; }}
              .subtitle {{ font-family: {FONT_MONO}; font-size: 17px; font-weight: 700; fill: {PALETTE["muted"]}; }}
              .footer {{ font-family: {FONT_MONO}; font-size: 13px; font-weight: 700; fill: {PALETTE["dark_text"]}; }}
              .panel-label {{ font-family: {FONT_MONO}; font-size: 12px; font-weight: 700; letter-spacing: 0.1em; fill: {PALETTE["accent"]}; }}
              .panel-title {{ font-family: {FONT_MONO}; font-size: 16px; font-weight: 700; fill: {PALETTE["dark_text"]}; }}
              .panel-copy {{ font-family: {FONT_MONO}; font-size: 14px; fill: {PALETTE["dark_text"]}; }}
              .panel-copy-muted {{ font-family: {FONT_MONO}; font-size: 13px; fill: {PALETTE["ink_alt"]}; }}
            ]]></style>
          </defs>

          <g clip-path="url(#frame-clip)">
            <rect width="1200" height="360" rx="28" fill="{PALETTE["paper"]}" />
            <rect width="1200" height="360" rx="28" fill="url(#ledger)" opacity="0.72" />
            <rect x="0" y="298" width="1200" height="62" fill="{PALETTE["paper_alt"]}" />

            {layered_card(42, 30, 724, 258, 22, PALETTE["ink"], stroke="#2E3D49").strip()}
            <rect x="70" y="58" width="250" height="32" rx="9" fill="{PALETTE["paper"]}" />
            <text x="90" y="79" class="badge">&gt; agentic-control</text>
            <rect x="70" y="112" width="28" height="3" rx="1.5" fill="{PALETTE["accent"]}" />
            <text x="114" y="120" class="eyebrow">RUNTIME EVENT STANDARDISATION</text>
            <text x="70" y="164" class="title">Agentic Control</text>
            <text x="70" y="208" class="subtitle">portable hook and plugin adapters</text>
            <text x="70" y="236" class="subtitle">one normalised event contract</text>
            <text x="70" y="264" class="subtitle">app-owned bindings through one installer</text>

            {layered_card(796, 44, 330, 140, 18, PALETTE["paper_alt"], stroke=PALETTE["line"], shadow_dx=6, shadow_dy=6).strip()}
            <text x="824" y="78" class="panel-label">[ runtimes ]</text>
            <text x="824" y="104" class="panel-title">current adapters</text>
            <text x="824" y="136" class="panel-copy">codex  gemini  claude  opencode</text>
            <text x="824" y="158" class="panel-copy-muted">hooks where available</text>
            <text x="824" y="176" class="panel-copy-muted">plugins where they are native</text>

            {layered_card(796, 188, 330, 148, 18, PALETTE["paper_alt"], stroke=PALETTE["line"], shadow_dx=6, shadow_dy=6).strip()}
            <text x="824" y="220" class="panel-label">[ contract ]</text>
            <text x="824" y="246" class="panel-title">core fields</text>
            <text x="824" y="276" class="panel-copy">runtime  session  tool</text>
            <text x="824" y="300" class="panel-copy">bindings  socket  debug</text>
            <text x="824" y="322" class="panel-copy-muted">product logic stays in your app</text>

            <rect x="786" y="60" width="10" height="216" rx="5" fill="{PALETTE["gold"]}" />
            <rect x="1156" y="62" width="16" height="16" rx="3" fill="{PALETTE["teal"]}" />
            <rect x="1156" y="88" width="16" height="16" rx="3" fill="{PALETTE["accent"]}" />
            <rect x="1156" y="114" width="16" height="16" rx="3" fill="{PALETTE["blue"]}" />

            <text x="70" y="334" class="footer">Hooks  •  Plugins  •  Unix sockets  •  Bindings  •  Portable adapters</text>
          </g>
        </svg>
        """
    )


def create_header(text: str, accent: str) -> str:
    label = escape(text)
    card_width = max(320, min(620, 176 + len(text) * 16))
    return dedent(
        f"""\
        <svg width="920" height="82" viewBox="0 0 920 82" fill="none" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <style><![CDATA[
              .label {{ font-family: {FONT_MONO}; font-size: 23px; font-weight: 700; fill: {PALETTE["dark_text"]}; }}
              .meta {{ font-family: {FONT_MONO}; font-size: 11px; font-weight: 700; letter-spacing: 0.14em; fill: {accent}; }}
            ]]></style>
          </defs>
          <rect x="8" y="14" width="{card_width}" height="48" rx="14" fill="{PALETTE["shadow"]}" />
          <rect x="0" y="6" width="{card_width}" height="48" rx="14" fill="{PALETTE["paper_alt"]}" stroke="{PALETTE["line"]}" stroke-width="2" />
          <rect x="22" y="0" width="92" height="18" rx="6" fill="{accent}" />
          <text x="33" y="13" class="meta">SECTION</text>
          <rect x="24" y="30" width="26" height="3" rx="1.5" fill="{accent}" />
          <text x="62" y="35" class="label" dominant-baseline="middle">{label}</text>
        </svg>
        """
    )


def main() -> None:
    ASSETS_DIR.mkdir(parents=True, exist_ok=True)

    write_asset("banner.svg", create_banner())
    write_asset("header-overview.svg", create_header("Overview", PALETTE["accent"]))
    write_asset("header-install.svg", create_header("Install and run", PALETTE["teal"]))
    write_asset(
        "header-runtimes.svg", create_header("Runtime support", PALETTE["blue"])
    )
    write_asset("header-contract.svg", create_header("Event contract", PALETTE["sage"]))
    write_asset("header-bindings.svg", create_header("Bindings", PALETTE["gold"]))
    write_asset(
        "header-architecture.svg", create_header("How it works", PALETTE["brick"])
    )
    write_asset("header-debug.svg", create_header("Debug mode", PALETTE["slate"]))
    write_asset(
        "header-scripts.svg", create_header("Scripts and assets", PALETTE["teal"])
    )
    write_asset(
        "header-repository.svg", create_header("Repository layout", PALETTE["accent"])
    )


if __name__ == "__main__":
    main()
