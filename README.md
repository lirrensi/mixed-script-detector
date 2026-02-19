# Mixed Script Detector

**Detect mixed-script content in your UI before it breaks your layout.**

When your website displays multiple writing systems (Latin + Chinese, Latin + Arabic, etc.), fonts can clash, baselines can misalign, and your carefully designed UI can break. This tool helps you find where mixed scripts appear on your site so you can fix them before users notice.

## Why This Exists

Mixed scripts cause subtle but real problems:

- **Font fallback issues**: Latin text may render in a different font than CJK characters, causing jarring visual shifts
- **Baseline misalignment**: Different scripts have different baseline metrics, leading to uneven text alignment
- **Line height breaks**: Scripts with tall characters (Arabic, Devanagari) can overflow containers designed for Latin text
- **Inconsistent user experience**: Your site looks polished in English-only sections but breaks in internationalized content

This tool crawls your website and identifies exactly where Latin characters mix with other scripts, so you can:
- Audit i18n implementations
- Find user-generated content that might break layouts
- Ensure consistent typography across language boundaries

## Features

- **12 script types**: Han (Chinese), Hiragana, Katakana, Hangul, Cyrillic, Arabic, Hebrew, Devanagari, Thai, Tamil, Bengali, Greek
- **Two modes**: Interactive TUI or CLI flags
- **Web crawling**: Automatically follows links within the same domain
- **Pretty output**: Terminal UI with live progress, plus TXT and JSON export
- **Context-aware**: Shows trigger phrases with surrounding context for easy location

## Download

**Non-technical users:** Grab the latest release from the [Releases page](https://github.com/lirrensi/mixed-script-detector/releases) — no installation needed!

- **Windows:** Download `mixed-script-detector-windows-amd64.zip`, extract, and double-click the `.exe`
- **macOS:** Download `mixed-script-detector-darwin-amd64.tar.gz` (Intel) or `darwin-arm64.tar.gz` (M1/M2), extract, and run
- **Linux:** Download `mixed-script-detector-linux-amd64.tar.gz`, extract, and run

## Quick Start

### Interactive Mode (Recommended)

```bash
# Run and follow the interactive prompts
./mixed-script-detector
```

### CLI Mode

```bash
# Scan a site for Han (Chinese) characters
./mixed-script-detector --url=https://example.com --script=han --max=50

# Scan for Arabic script
./mixed-script-detector -url=https://example.com -script=arabic
```

### Available Scripts

| Flag | Script | Example Languages |
|------|--------|-------------------|
| `han` | Han (Chinese/Japanese Kanji) | 中文，日本語 |
| `hiragana` | Hiragana | ひらがな |
| `katakana` | Katakana | カタカナ |
| `hangul` | Hangul | 한글 |
| `cyrillic` | Cyrillic | Русский |
| `arabic` | Arabic | العربية |
| `hebrew` | Hebrew | עברית |
| `devanagari` | Devanagari | हिन्दी |
| `thai` | Thai | ไทย |
| `tamil` | Tamil | தமிழ் |
| `bengali` | বাংলা | Bengali |
| `greek` | Greek | Ελληνικά |

## Output

The tool generates two files:

- `domain_YYYY-MM-DD_findings.txt` — Human-readable report
- `domain_YYYY-MM-DD_findings.json` — Machine-readable JSON for automation

### Example TXT Output

```
=== Mixed Script Detector Results ===
Date: 2026-02-20 15:30:00
Root URL: https://example.com
Target Script: Han (Chinese/Japanese Kanji)
=====================================

--- URL: https://example.com/products ---
Found 3 instance(s):

[1] Check out our new [产品] collection
[2] Browse [商品] by category
[3] Contact [客服] for support
```

### Example JSON Output

```json
{
  "metadata": {
    "date": "2026-02-20 15:30:00",
    "root_url": "https://example.com",
    "target_script": "Han (Chinese/Japanese Kanji)",
    "total_pages_with_findings": 1,
    "total_matches": 3
  },
  "results": [
    {
      "url": "https://example.com/products",
      "match_count": 3,
      "matches": [
        {
          "trigger": "产品",
          "context_before": "our new",
          "context_after": "collection",
          "full_segment": "Check out our new 产品 collection"
        }
      ]
    }
  ]
}
```

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/lirrensi/mixed-script-detector.git
cd mixed-script-detector

# Build
go build -o mixed-script-detector

# Run
./mixed-script-detector
```

### Requirements

- Go 1.25 or later
- Internet connection (for crawling)

## Development

```bash
# Build
make build

# Run interactive
make run

# Build and run
make dev

# Clean build artifacts
make clean
```

## How It Works

1. **Crawl**: Starting from your URL, the tool fetches pages and follows same-domain links
2. **Extract**: Visible text is extracted (scripts, styles, and hidden elements are ignored)
3. **Detect**: Each sentence is checked for Latin + target script combinations
4. **Report**: Findings are exported with context to help you locate them in your UI

## Limitations

- Single-domain crawling only (won't follow external links)
- JavaScript-rendered content may not be fully captured (tool fetches raw HTML)
- Maximum 50 pages by default (configurable via `--max`)

## License

MIT License — see [LICENSE](LICENSE) for details.

## Contributing

This is a personal tool, but feel free to open issues or PRs if you find bugs!

---

**Made with** 🐱 **by Bastet** — Keeper of the Home
