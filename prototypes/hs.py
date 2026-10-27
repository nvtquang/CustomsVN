import json, re, glob
import pyarrow.parquet as pq
R = glob.glob(r"D:\DevCache\luatdo\raw\th1nhng0\*\data")[0]
d = json.load(open(r"D:\DevCache\luatdo\docs\vn_law_2022_31-2022-tt-btc.json", encoding="utf-8"))
iid = re.search(r"ItemID=(\d+)", d["source_url"])[1]
h = pq.read_table(R + r"\content.parquet", filters=[("id", "=", iid)]).to_pylist()[0]["content_html"]
pat = r"\b\d{4}\.\d{2}\.\d{2}\b"
raw = set(re.findall(pat, re.sub(r"<[^>]+>", " ", h)))
par = set(re.findall(pat, "\n".join((p.get("heading") or "") + " " + (p.get("text") or "") for p in d["provisions"])))
print("unique HS 8-digit codes: raw", len(raw), "parsed", len(par), "lost", len(raw - par))
ch = lambda s: sorted({c[:2] for c in s})
lost = raw - par
print("HS chapters with any loss:", len(ch(lost)), ch(lost)[:40])
print("example lost:", sorted(lost)[:8])
for p in d["provisions"][:0]: pass
chs = [p for p in d["provisions"] if p["kind"] == "chapter"]
print("parsed 'chapter' headings sample:", [ (p["number"], (p.get("heading") or "")[:40]) for p in chs[:6]])
big = sorted(d["provisions"], key=lambda p: -len(p.get("text") or ""))[:3]
print("largest provisions:", [(p["id"].split(":",4)[-1], len(p.get("text") or "")) for p in big])
