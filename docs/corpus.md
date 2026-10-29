# Customs corpus: `customsvn prep`

`customsvn prep` builds the customs corpus from a luatdo data directory. It runs after luatdo's `fetch`, `parse`, `cite` and `subjects` and reads what they left on disk. It never writes into the luatdo store.

```sh
export LUATDO_DATA=/path/to/luatdo          # D:\DevCache\luatdo on the maintainer's machine
go run ./cmd/customsvn prep -as-of 2026-10-27            # every step
go run ./cmd/customsvn prep -as-of 2026-10-27 chunks     # one step, from the earlier steps' output
```

`-as-of` is the day force is judged on. It defaults to today, so pin it whenever two runs have to agree.

## Steps

| Step | Reads | Writes under `$LUATDO_DATA/customsvn/prep` |
| --- | --- | --- |
| `tier` | `docs/*.json`, `subject/assignments.jsonl`, `cite/*.json` | `domain.jsonl`: every store document's header, subjects and tier |
| `vbhn` | `metadata.parquet`, `content.parquet` | `vbhn/*.json`: tier 1–2 consolidated texts parsed with luatdo `parse` |
| `rescue` | `domain.jsonl`, `content.parquet` | `rescue/*.json`: quarantined in-force tier 1–2 documents, reparsed |
| `hs` | `domain.jsonl`, `content.parquet` | `hs/hs_31-2022.jsonl`: the HS nomenclature of TT 31/2022 with its parent chain |
| `tables` | `domain.jsonl`, `metadata.parquet`, `content.parquet` | `tables/*.json`: the data tables of every corpus document as grids (`internal/htmltable`, colspan and rowspan aligned) |
| `chunks` | all of the above, `docs/*.json` | `customs/chunks.jsonl`, `customs/documents.jsonl` |
| `report` | `domain.jsonl`, `customs/*` | `report/coverage.{md,csv}` for the mandatory list, `report/old_in_force.csv` for expert review |

`vbhn`, `rescue`, `hs` and `tables` share one pass over the 800 MB content file. Every step writes `summary/<step>.json` with its counts; `tier` and `chunks` also record the SHA-256 of what they wrote.

A full run takes one to two minutes. Two runs with the same inputs and `-as-of` write the same bytes: all 1,663 output files were compared on 2026-10-27.

### Tiers

1. Core customs: procedures, import and export duty, origin, valuation, classification, trade remedies, bonded areas, smuggling. A subject in `thue/hai-quan`, `thue/thue-xuat-nhap-khau` or `thuong-mai/xuat-nhap-khau`, or a core cue in the title, and not issued by a province.
2. Adjacent: other taxes, trade, fees, foreign exchange, borders and transport, product quality, food safety and chemicals; plus national acts that two or more tier 1 documents cite.
3. Everything else. Stays in the store, never retrieved for the customs bot.

### Force

A document is kept when it is in force on the as-of day. The expiry date wins over the status label, because the source updates the date before the label; a partly expired document is the exception, since its expiry date names the part that ended.

### Sources and chunks

Chunks come from three sources, in the order a retriever should trust them:

- `consolidated`: the latest VBHN of each base instrument whose base is still in force. Footnotes that record an amendment are attached to the chunks that cite them.
- `original`: in-force tier 1–2 documents of the store. When a VBHN consolidates one, `consolidated_in` names it, because this wording may be stale.
- `rescued`: in-force documents luatdo quarantined on a false alarm (the body's number differs from the metadata only in form, or by an apparent year typo, which is flagged `needs_review`) or because they have no article structure.

Each table with at least two rows and two filled columns also becomes a `table` chunk (`<doc>:table-N`): the grid as Markdown, cut at rows into pieces of about 2,500 characters with the header row repeated on every piece, placed under the provision whose text holds its first cells. Layout tables (the national motto beside the issuer, the signature block) are skipped. The plain-text chunks still carry the same words one cell per line; the table chunk adds the grid, so a fee or tariff row reads as a row.

One chunk per article with its clauses and points. An article over 4,000 characters is split at its clauses; a chunk still over 4,000 characters (a table, a schedule) is cut at line boundaries into pieces of about 2,500 that all keep the heading path. A text with no structure is cut at paragraphs into chunks of about 1,500 characters, without the signature block. Chunk identifiers are unique; the step stops if two chunks claim one.

### Consolidated texts from `legacy_content` (on by default)

314 of the 367 tier 1–2 VBHN rows have an empty page at vbpl.vn. The dataset's `legacy_content` config, an older crawl in the same dataset, holds text for 219 of them, matched on official number and signing date together (a VBHN number repeats across years and ministries). With it the kept consolidated texts go from 44 to 144 and consolidated chunks from 2,554 to 7,681. It is on by default since 2026-10-27 (the owner's decision); `-legacy=false` turns it off. The dataset does not say where that crawl came from; see NOTICE. Download `legacy/metadata.parquet` and `legacy/content.parquet` of the pinned revision into `raw/th1nhng0/<revision>/legacy/` to use it.

## Mandatory list and expert review

`internal/corpus/mandatory.tsv` is a draft list of 95 customs instruments, each with a phrase its title must carry, so a wrong number shows up as "check" rather than as a silent miss. `prep report` writes [coverage.md](coverage.md): on 2026-10-27, 84 are in the corpus, 8 are rightly absent because they are no longer in force (each with its expiry date), and 3 are missing because the source has metadata but no text: Luật 90/2025/QH15 (which amends the Customs Law and the Export and Import Duty Law), Luật Thuế GTGT 48/2024/QH15 and TT 121/2025/TT-BTC (which amends TT 38/2015). Those three need another source.

`report/old_in_force.csv` lists the 1,081 corpus documents signed 1995–2009 that the source still calls in force (404 tier 1), with empty columns for the expert's verdict. The plan's estimate of about 360 counted fewer document types.

## Baseline

th1nhng0 revision `8977887f17be`, luatdo `42cd481`, as of 2026-10-27. The table is without `legacy_content`; with it (the default, corpus version 2) the totals are 70,654 chunks, 7,681 of them consolidated, from 144 VBHN.

The port was first checked against the prototypes in a faithful form: tiers matched for all 128,094 documents, and every chunk the two produced in common matched in text and metadata. The only difference was NĐ 205/2026/NĐ-CP (defect 1 below). The defects were then fixed one at a time. The table gives the numbers after the fixes, with the prototype's numbers where they differ.

| Measure | Value | Prototype |
| --- | --- | --- |
| Documents parsed / quarantined / metadata only | 104,671 / 15,115 / 8,308 | |
| In force (kept) / dropped | 46,308 / 81,786 | |
| In-force tier 1 / tier 2 documents (original, parsed) | 830 / 3,229 | |
| Quarantined in-force tier 1–2 | 403 (352 no structure, 23 format, 4 year-typo, 5 number differs, 19 other document) | 24 other covered both of the last two |
| Rescued: structured / text only | 26 / 353 | 40 / 339 |
| VBHN rows / tier 1–2 / with text / kept | 2,242 / 378 / 53 / 44 | kept 38 |
| VBHN superseded by a newer consolidation / base expired | 4 / 5 | 10 / 5 |
| Chunks: total (text + table) | 65,527 (59,757 + 5,770) | 70,283 |
| Chunks: consolidated / original / rescued | 2,554 / 59,808 / 3,165 | 1,911 / 65,529 / 2,843 |
| Chunks tier 1 / tier 2 | 14,673 / 50,854 | 13,376 / 56,907 |
| Tables: documents with tables / tables kept | 903 / 3,433 | none |
| HS rows / 8-digit lines / unique 8-digit codes | 18,159 / 12,291 / 11,414 | 18,157 rows |

The drop in original chunks is duplicated text going away, not text lost: every one of the 218,407 provisions with text in a kept original document is in a chunk.

### Defects fixed in the port

1. **Wrong text for a rescued document.** The prototype read the source row from `ItemID=(\d+)`, which cuts a UUID such as `49828ac0-6e5a-…` to `49828`, another row. NĐ 205/2026/NĐ-CP on airports was given the text of a Nghệ An resolution on school fees. The row is now read whole.
2. **Rescued text dropped.** A page whose "Điều" markers were lost at the source (auto-numbering) parses into chapters and sections with no text. Chunking that tree emitted nothing, and 178,000 characters of NĐ 205/2026 vanished. A tree with no text is now treated as text only; 14 rescued documents moved that way.
3. **One identifier, two chunks.** Articles inside an annex (a regulation issued with a circular) were chunked once on their own and again inside the annex, so 1,775 identifiers were shared by 3,550 chunks. Each provision now belongs to exactly one chunk, and duplicate identifiers stop the run.
4. **Wrong VBHN base.** The base was the first number in the title, which is often the decree a circular implements (47/VBHN-BCT named 69/2018/NĐ-CP, not its base 12/2018/TT-BCT; 78/VBHN-NHNN named 140/2000/QĐ-TTg). Unrelated consolidations then shared a base and all but one were dropped as superseded. The base is now the instrument that opens the paragraph ending "được sửa đổi, bổ sung bởi". Six more VBHN are kept, and 06/VBHN-BTC is dropped: it consolidates TT 86/2024/TT-BTC, which expired on 01/07/2026.
5. **Amenders included instruments only mentioned.** Every number after "được sửa đổi … bởi" was taken, including those an amending paragraph mentions (14/VBHN-BTC listed 49/2022/NĐ-CP, 209/2013/NĐ-CP and others). Now only the first number of each paragraph counts.
6. **Rowspan ignored in HTML tables.** `internal/htmltable` aligns cells under `colspan` and `rowspan`. Every coded HS line is unchanged; two uncoded lines of a chapter 48 note table came out differently.

### Open issues

- The VBHN empty count is 1,942 of 2,242 rows with content, against 1,902 in the survey notes, whose method was not recorded. Not used downstream.
- HS rows accept a two-digit "code" anywhere, so a few cells of chapter note tables (`60`, `70 Ũ`) come out as rows. The tariff lines are unaffected; filter them at ingest or tighten the parser.
- About 360 documents issued 1995–2009 are still labelled "Còn hiệu lực" and need expert review before they are trusted.
- Table chunks sit beside the plain-text chunks of the same words. If Retrieval Testing shows the duplication costs ranking slots, the plain-text lines of a table can be dropped once the table chunk is placed reliably.
