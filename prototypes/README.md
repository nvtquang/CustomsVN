# Prototypes

Scripts from the data survey on 2026-10-27, kept as the reference for porting `customsvn prep` to Go in Phase 0. **Ported:** `build_rag.py`, `domain.py`, `select.py`, `hqprep`, `build_customs.py` and `hs_tables.py` now live in `internal/corpus` and `internal/prep`; the port reproduced the numbers below and then fixed the defects listed in `docs/corpus.md`. They can be deleted once the repository has a first commit that keeps them in history. They read and write `D:\DevCache\luatdo` directly and are not part of the product. Delete each one once its Go port reproduces the numbers below.

Python needs a virtualenv with `pyarrow`; run with `python -I -X utf8 <script>`.

| Script | What it does | Port to |
| --- | --- | --- |
| `build_rag.py` | In-force filter (status and expiry date), chunk by article, split long articles at clauses | `internal/corpus` chunks |
| `domain.py` | Tier 1/2/3 by subject and title cues, promote documents cited by 2+ tier 1 documents | `customsvn prep tier` |
| `select.py` | Pick customs VBHN rows; classify quarantined documents (format, year-typo, number-differs, other-document) | `prep vbhn`, `prep rescue` |
| `hqprep/main.go.txt` | Parse selected rows with luatdo `parse.HTMLText` + `parse.Fragment` | `prep vbhn`, `prep rescue` |
| `build_customs.py` | Merge consolidated, original and rescued sources; VBHN base, amenders and footnotes; cut long chunks | `prep chunks` |
| `hs_tables.py` | HS nomenclature of TT 31/2022 straight from the HTML tables, with parent chain | `prep hs` |
| `scope.py`, `force.py`, `annex.py`, `raw.py`, `hs.py` | One-off analyses | not ported |

## Baseline (th1nhng0 revision 8977887f17be, as of 2026-10-27, luatdo 42cd481)

| Measure | Value |
| --- | --- |
| Documents parsed / quarantined / metadata only | 104,671 / 15,115 / 8,308 |
| In force (kept) / dropped | 46,308 / 81,786 |
| In-force tier 1 / tier 2 documents (original, parsed) | 830 / 3,229 |
| Quarantined in-force tier 1–2 | 403 (352 no structure, 23 format, 4 year-typo, 24 other document) |
| Rescued | 379 (40 structured, 339 text only) |
| VBHN total / empty HTML / customs with text / kept | 2,242 / 1,902 / 53 / 38 |
| Customs chunks: total / consolidated / original / rescued | 70,283 / 1,911 / 65,529 / 2,843 |
| Customs chunks tier 1 / tier 2 | 13,376 / 56,907 |
| HS rows / unique 8-digit codes (TT 31/2022) | 18,157 / 11,414 |

Known defects to fix while porting: VBHN base detection picks the instrument named in the title when it is not the base (47/VBHN-BCT); amenders can include legal-basis instruments listed on the same line (14/VBHN-BTC); old documents (1995–2009) marked "Còn hiệu lực" need expert review.
