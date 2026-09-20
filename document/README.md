# gitdash documentation site (Hugo)

This directory is an **independently deployed** Hugo site documenting gitdash,
organized by module and feature. The application links to it via
`GITDASH_DOCS_URL` and shows a docs entry on the login page and in the header.

## Languages

The site is multilingual. English is the **primary** language and is served
from the site root; other languages live under their language prefix.

| Language | URL | Content |
|---|---|---|
| English (`en`) | `/gitdash/` | `content/en/` (primary) |
| 中文 (`zh-cn`) | `/gitdash/zh-cn/` | `content/zh-cn/` (complete) |
| 日本語 / 한국어 / Français / Deutsch / Русский / Español / Português | `/gitdash/<lang>/` | falls back to `content/en/` until translated |

- UI strings live in `i18n/<lang>.toml` (missing keys fall back to English).
- The header language selector links to the current page's translation, or to
  that language's home page if no translation exists.
- `hreflang` alternates are emitted automatically for SEO.

## Theme

The header offers **Light / Dark / System**, matching the app. The preference
is stored under the same `gitdash-theme` key the app uses, and `System` follows
`prefers-color-scheme` changes live.

### Adding a translation

1. Create `content/<lang>/…` mirroring the English tree (e.g. `content/ja/getting-started/register.md`).
2. In `hugo.toml`, point that language's `contentDir` at it (remove the `content/en` fallback).
3. Add `i18n/<lang>.toml` for UI strings.

## Local preview

```bash
# Install Hugo (extended, >= 0.120)
hugo server -D --source document
# Open http://localhost:1313/gitdash/
```

## Build

```bash
hugo --source document --minify
# Output in document/public/
```

## Deploy

Any static host works. The repository ships a GitHub Actions workflow
(`.github/workflows/docs.yml`) that builds and publishes to GitHub Pages.

For a sub-path deployment (such as `https://<user>.github.io/gitdash/`), set
`baseURL` accordingly; then point the app's `GITDASH_DOCS_URL` at that address.

## Structure

```
document/
├── hugo.toml          # site config (languages / baseURL / highlight)
├── i18n/              # UI strings per language
├── content/
│   ├── en/            # English (primary)
│   └── zh-cn/         # 中文 (complete)
├── layouts/           # minimal custom theme (matches the app's design tokens)
├── static/css|js/     # styles + theme toggle (shares gitdash-theme)
└── README.md
```

Content is grouped by module: `getting-started`, `account`, `repositories`,
`issues`, `pulls`, `projects`, `ci`, `packages`, `copilot`, `admin`,
`self-hosting`.

## Add a page

Create `content/en/<module>/<page>.md` (and the matching `content/zh-cn/...`):

```yaml
---
title: "Page title"
weight: 10
summary: "One-line summary shown in lists."
---
```

The sidebar is generated automatically from the sections
(`layouts/partials/sidebar.html`).
