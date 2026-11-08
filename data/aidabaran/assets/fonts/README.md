# Self-hosted fonts (UI-01)

The existing `unbounded-{latin,cyrillic}.woff2` and
`montserrat-{latin,cyrillic}.woff2` files are preserved unchanged.
They do not cover the ruble sign (U+20BD).

## Currency coverage

Downloaded on 2026-11-08 from official Google Fonts infrastructure, after
checking the official CSS and upstream licenses:

<https://fonts.googleapis.com/css2?family=Unbounded:wght@200..900&family=Montserrat:wght@100..900&display=swap>

| Local file | Official subset | Style | Variable weight | Required character |
| --- | --- | --- | --- | --- |
| `unbounded-currency.woff2` | Unbounded v12 latin-ext | normal | 200-900 | U+20BD |
| `montserrat-currency.woff2` | Montserrat v31 latin-ext | normal | 100-900 | U+20BD |

These are complete, unmodified official latin-ext subsets, not custom
single-character fonts. Both CSS ranges include `U+20AD-20C4`, covering
U+20BD; the official cyrillic/cyrillic-ext/latin subsets do not cover it.
Use the currency files with `unicode-range: U+20BD` and `font-display: swap`
in the matching self-hosted font faces. The URLs below document provenance
only; runtime font loading must remain local under
`/static/aidabaran/assets/fonts/`.

Official binary URLs:

- Unbounded: <https://fonts.gstatic.com/s/unbounded/v12/Yq6W-LOTXCb04q32xlpwtcZfrxE.woff2>
- Montserrat: <https://fonts.gstatic.com/s/montserrat/v31/JTUSjIg1_i6t8kCHKm459Wdhyzbi.woff2>

SHA-256 of the downloaded binaries:

```text
845e1c9fe2e9a7bfbedfeb447fbeba4bc1b5ecd6246277910ccdaad35a7a532c  unbounded-currency.woff2
54d9a78b7ff60b689ad9f3017ffac8547b5d871afec733f6c1c3ae36577ee504  montserrat-currency.woff2
```

Both downloads returned HTTP 200 and `Content-Type: font/woff2`.
`file` identified real WOFF2/TrueType binaries, respectively 118348 and
70688 bytes. Chromium CDP `CSS.getPlatformFontsForNode` confirmed U+20BD
as one custom-font glyph with no fallback at weights 300/400/600/700 and
both weight-range endpoints, with font synthesis disabled. The four
existing files were tested separately as negative controls and used a
system fallback for U+20BD. No new dependencies or font transformations
were used; fontTools was not available in the checked Python environments.

## Licenses

Both families are distributed under SIL Open Font License 1.1. Retain the
copyright notices and full licenses when redistributing these fonts:

- `OFL-Unbounded.txt`: Copyright 2022 The Unbounded Project Authors.
  Source: <https://raw.githubusercontent.com/google/fonts/ca71f3524c547bb503e6fbc025ec7b8a088f4aff/ofl/unbounded/OFL.txt>
- `OFL-Montserrat.txt`: Copyright 2024 The Montserrat.Git Project Authors.
  Source: <https://raw.githubusercontent.com/google/fonts/76fca9fd0bb4ea46583f92e978660f3984ab9442/ofl/montserrat/OFL.txt>

License text was compared against these pinned upstream copies.
Unbounded is byte-identical; Montserrat differs only by an added final LF.
