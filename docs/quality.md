# Quality plan (from 2026-11-02)

What the first hand-graded run (94 questions, 70.2%) showed, and the work that follows. The run itself is not kept as a gold set (owner's decision, 2026-11-02: a new 50-question set from the experts replaces it), so its numbers are a diagnosis, not a baseline.

## Diagnosis

About 77% of the points lost came from the data and from force, not from retrieval:

| Cause | Points lost | Where it lives |
| --- | --- | --- |
| A text the corpus lacks (Luật 48/2024, TT 36/2026, chemical annexes) | 11 | input |
| A superseded text cited as current law (TT 219/2013 after TT 69/2025) | 10.5 | force |
| Part of the question left unanswered | 3 | retrieval, composition |
| Wrong refusal or wrong reasoning | 3.5 | retrieval, agent |

The force fault is in code, not in the source: `corpus.Force` reads the status label and the expiry date only, while the `edges` table already holds vbpl.vn's `replaces`, `repeals` and `expires` relations (124 documents and 14 VBHN are replaced outright, about 8,000 chunks).

## Test set

The measure is the 50 questions the experts reviewed (`Bo_50_cau_hoi_chatbot_hai_quan.docx`, answers and scoring in `Dap_an_va_cham_diem_chatbot_hai_quan.xlsx`: content 0-5, citation 0-3, force and traps 0-2, a serious error zeroes the question). The 94-question hand-graded run and `goldsets/eval60.csv` are not used to judge quality any more (owner's decision, 2026-11-02). `customsvn ask -questions F -out F` puts a list of questions through the whole answer, each in a fresh conversation, with no login, and writes what a reader would have seen; the file is then graded against the sheet.

## Phases

Each step is tested without network or credentials. Mini10 stays as the cheap check between runs.

**0. Baseline.** `stale_citations` (answers citing a provision that is no longer law on the question's day; the rule is 0) is a metric of every answer run. mini10 holds three regression questions (s01-s03). Done.

**1. Identity and display.** Done 2026-11-02. A bare number that several documents share (18/VBHN-BTC of 2023 and of 2026) resolves, in the status, amendments and graph tools, to the document the answer already works from, then to one in the corpus, in force, of the lowest tier, the newest; the card names the others. A consolidated text shows the force of its base.

**2. Force from supersession.** Done 2026-11-02 (see [corpus.md](corpus.md)). A text that replaced another as a whole, confirmed by vbpl.vn's edge and by its own wording, ends it; 119 documents, 16 of tier 1, on 2026-11-02. The status card says who replaced it.

**3. Data.** Partly done.
- Done: the watch list `mandatory.tsv` holds the instruments the test set needs; a text vbpl.vn has not labelled yet counts as in force for a year (decree 275/2026/NĐ-CP and three more were dropped for it); corpus version 4 (2026-11-02).
- Open: the crawl of vbpl.vn (a plain request from the container got 403; the project's own crawler is not run yet); a command to load a text from a file for what vbpl.vn holds only as metadata (Luật 48/2024, Luật 90/2025, TT 121/2025, TT 275/2026 is in); the annexes (chemical lists).

**4. Retrieval for synthesis.** Done 2026-11-02: the router splits a multi-part question and each part is searched on its own with the places shared among them; transition provisions join the sources by code; the composer is told the parts; one repair try when every quote fell. Not done: a completeness check beyond the gap lines.

**5. Experts.** The experts' 50 questions, run in full, graded on their sheet. Target: 90-95% on content and 0 stale citations.

## Result of 2026-11-02

First run of the experts' 50 questions (corpus version 4, after phases 1, 2 and 4; `customsvn ask`, BM25, answer model openai/gpt-6-luna, $0.49, 29 minutes), graded by Claude on the sheet (an expert must review it; the graded file is `Ket_qua_cham_diem_chatbot_hai_quan.xlsx`):

| Level | Questions | Mean /10 | Pass (≥7) |
| --- | --- | --- | --- |
| Recall | 12 | 7.46 | 9 |
| Understanding | 10 | 8.15 | 9 |
| Application | 10 | 6.55 | 5 |
| Analysis, synthesis | 8 | 4.31 | 2 |
| Traps | 6 | 3.67 | 0 |
| Open | 4 | 4.88 | 0 |
| All | 50 | 6.25 (weighted 5.55) | 25 pass, 12 partial, 13 fail |

Content 58%, citations 65%, force and traps 71%; two serious errors (Q9, Q41). Against the target of 90-95% on content and no stale citation, the gap is in three places:

- **Texts the corpus lacks (about 11 of the 50 questions touch one):** Luật 90/2025/QH15 and Luật 48/2024/QH15 (metadata only), TT 121/2025/TT-BTC (metadata only), Luật 11/2026/QH16, NĐ 292/2026/NĐ-CP, QĐ 31/2026/QĐ-TTg, QĐ 382/QĐ-BTC (absent). Without the replacing text, the old one stays "in force": NĐ 69/2018 (Q29) and QĐ 23/2019 (Q46) were cited as law.
- **Old provisions the source still calls law:** QĐ 153/2002/QĐ-BTC (Q36), the 0.05% late-payment rate of TT 38/2015 art. 133 (Q27), the form 09/BQHH/GSQL (Q38), TT 39/2015 as the base of customs value (Q40), the exemption of NĐ 134/2016 for express consignments under 1 million (Q41). `endedAsAWhole` ends whole documents only; a provision replaced by a later law, or a document replaced by an edge the source never recorded, is not seen. Seven answers cite something the experts count as out of force, so the "0 stale citations" target is not met, and the system's own `stale_citations` reads 0 because it trusts the same labels.
- **Retrieval and composition:** synthesis answers still drop a part (Q33 omits case 3; Q49 and Q47 omit most of five expected ideas); Q31 was asked back instead of answered; Q32 was refused by the tariff rule on the words "ảnh hưởng đến thuế suất".

## Not doing

Commercial legal databases as a source: their editorial layer (force, links) is under licence terms that forbid redistribution, which an MIT project cannot honour.
