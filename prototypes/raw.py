import json, re, glob
import pyarrow.parquet as pq
import pyarrow.compute as pc

R = glob.glob(r"D:\DevCache\luatdo\raw\th1nhng0\*\data")[0]
docs = {k: json.load(open(rf"D:\DevCache\luatdo\docs\{k}.json", encoding="utf-8"))
        for k in ["vn_law_2023_26-2023-nd-cp", "vn_law_2022_31-2022-tt-btc", "vn_law_2015_38-2015-tt-btc"]}
ids = {re.search(r"ItemID=(\d+)", d["source_url"])[1]: k for k, d in docs.items()}

t = pq.read_table(R + r"\content.parquet", filters=[("id", "in", list(ids))])
for row in t.to_pylist():
    k = ids[str(row["id"])]
    h = row["content_html"] or ""
    text = re.sub(r"<[^>]+>", " ", h)
    print(f"\n=== {k}  html={len(h)} chars  tables={h.lower().count('<table')}  rows={h.lower().count('<tr')}")
    print("   'Phụ lục' mentions:", len(re.findall(r"PHỤ LỤC|Phụ lục", text)))
    print("   attachment links:", re.findall(r'href="([^"]+\.(?:pdf|doc|docx|xls|xlsx|zip))"', h, re.I)[:5])
    hs = re.findall(r"\b\d{4}\.\d{2}\.\d{2}\b", text)
    print("   HS-like codes in raw:", len(hs), hs[:5])
    i = text.rfind("Phụ lục")
    print("   tail:", re.sub(r"\s+", " ", text[-500:]))
    parsed = "\n".join(p.get("text") or "" for p in docs[k]["provisions"])
    print("   HS-like codes in parsed:", len(re.findall(r"\b\d{4}\.\d{2}\.\d{2}\b", parsed)))
