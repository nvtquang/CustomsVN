"""Prototype: read the HS nomenclature of TT 31/2022/TT-BTC straight from the vbpl HTML tables.

Output D:\\DevCache\\luatdo\\rag\\hs\\hs_31-2022.csv with one row per line of the nomenclature,
its level (from the leading dashes), and its parent, so "Loại thuần chủng để nhân giống"
is known to sit under "Ngựa".
"""
import csv, glob, json, os, re
from html.parser import HTMLParser
import pyarrow.parquet as pq

ROOT = r"D:\DevCache\luatdo"


class Tables(HTMLParser):
    """Collect every table as rows of cell texts, honouring colspan so columns line up."""

    def __init__(self):
        super().__init__()
        self.tables, self.row, self.cell, self.depth = [], None, None, 0

    def handle_starttag(self, tag, attrs):
        if tag == "table":
            self.depth += 1
            if self.depth == 1:
                self.tables.append([])
        elif tag == "tr" and self.depth == 1:
            self.row = []
        elif tag in ("td", "th") and self.row is not None:
            self.cell = {"text": "", "span": int(dict(attrs).get("colspan", "1") or 1)}
        elif tag in ("br", "p") and self.cell is not None:
            self.cell["text"] += " "

    def handle_endtag(self, tag):
        if tag == "table":
            self.depth -= 1
        elif tag == "tr" and self.row is not None:
            if any(c.strip() for c in self.row):
                self.tables[-1].append(self.row)
            self.row = None
        elif tag in ("td", "th") and self.cell is not None:
            t = re.sub(r"\s+", " ", self.cell["text"]).strip()
            self.row += [t] + [""] * (self.cell["span"] - 1)
            self.cell = None

    def handle_data(self, data):
        if self.cell is not None:
            self.cell["text"] += data


d = json.load(open(ROOT + r"\docs\vn_law_2022_31-2022-tt-btc.json", encoding="utf-8"))
iid = re.search(r"ItemID=(\d+)", d["source_url"])[1]
R = glob.glob(ROOT + r"\raw\th1nhng0\*\data")[0]
html = pq.read_table(R + r"\content.parquet", filters=[("id", "=", iid)]).to_pylist()[0]["content_html"]
p = Tables(); p.feed(html)

CODE = re.compile(r"^\d{2}(?:\.\d{2})?$|^\d{4}(?:\.\d{2}){0,2}$")
rows, stack, seen_codes = [], [], set()
for table in p.tables:
    for r in table:
        if len(r) < 3 or not r[1]:
            continue
        code, desc_vi = r[0], r[1]
        desc_en = r[4] if len(r) >= 6 else ""
        unit = r[2] if len(r) >= 3 else ""
        if code and not CODE.match(code):
            continue  # header rows, chapter titles, notes
        dashes = len(re.match(r"^((?:-\s*)*)", desc_vi)[1].replace(" ", ""))
        level = dashes if dashes else (0 if len(code.replace(".", "")) <= 4 else 1)
        while stack and stack[-1]["level"] >= level:
            stack.pop()
        parent = stack[-1] if stack else None
        row = {"code": code, "level": level, "description_vi": desc_vi.lstrip("- ").strip(),
               "description_en": desc_en.lstrip("- ").strip(), "unit": unit,
               "chapter": (code or (parent or {}).get("code", ""))[:2],
               "parent_code": (parent or {}).get("code", ""), "parent_description": (parent or {}).get("description_vi", "")}
        rows.append(row)
        stack.append(row)
        if code:
            seen_codes.add(code)

os.makedirs(ROOT + r"\rag\hs", exist_ok=True)
with open(ROOT + r"\rag\hs\hs_31-2022.csv", "w", encoding="utf-8-sig", newline="") as f:
    w = csv.DictWriter(f, fieldnames=list(rows[0]))
    w.writeheader(); w.writerows(rows)

eight = [r for r in rows if re.match(r"^\d{4}\.\d{2}\.\d{2}$", r["code"])]
print(f"tables {len(p.tables)}, rows kept {len(rows)}, 8-digit lines {len(eight)}, unique 8-digit codes {len({r['code'] for r in eight})}")
for r in rows[:8]:
    print(f"  {r['code']:12s} L{r['level']} {r['unit']:8s} {r['description_vi'][:45]:45s} <- {r['parent_code']} {r['parent_description'][:30]}")
x = next(r for r in rows if r["code"] == "8703.23.61") if any(r["code"] == "8703.23.61" for r in rows) else eight[len(eight) // 2]
chain, cur = [], x
by = {}
for r in rows:
    by.setdefault(r["code"] or r["description_vi"], r)
print("\nexample:", x["code"], "|", x["description_vi"], "|", x["description_en"], "| parent:", x["parent_code"], x["parent_description"][:60])
