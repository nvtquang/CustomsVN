import json, os, re, sys, collections
import pyarrow.parquet as pq

S = sys.argv[1]
D = os.path.join(S, "data")
PROV = ["ubnd", "ủy ban nhân dân", "uỷ ban nhân dân", "hđnd", "hội đồng nhân dân"]

def iso(d):
    m = re.match(r"(\d{1,2})/(\d{1,2})/(\d{4})", d or "")
    return f"{m[3]}-{int(m[2]):02d}-{int(m[1]):02d}" if m else (d or "")

# documents luatdo took, keyed by vbpl ItemID
docs = {}
for f in os.listdir(os.path.join(D, "docs")):
    d = json.load(open(os.path.join(D, "docs", f), encoding="utf-8"))
    docs[d["id"]] = d
thue = set()
for line in open(os.path.join(D, "subject", "assignments.jsonl"), encoding="utf-8"):
    r = json.loads(line)
    if any(s["subject_id"] == "thue" or s["subject_id"].startswith("thue/") for s in r.get("subjects") or []):
        thue.add(r["doc_id"])

raw = os.path.join(D, "raw", "th1nhng0")
raw = os.path.join(raw, os.listdir(raw)[0], "data", "metadata.parquet")
meta = {}
for row in pq.read_table(raw).to_pylist():
    meta[str(row["id"])] = row

def item(d):
    m = re.search(r"ItemID=(\d+)", d.get("source_url", ""))
    return meta.get(m[1]) if m else None

central = [d for i, d in docs.items() if i in thue and not any(p in d.get("issuing_body", "").lower() for p in PROV)]
buggy = [d for d in central if d.get("effective_from", "") <= "2025-12-31"]  # v0.1.1 string compare
fixed = [d for d in central if iso(d.get("effective_from")) <= "2025-12-31"]

status = collections.Counter()
inforce, customs = [], []
for d in fixed:
    m = item(d) or {}
    st = (m.get("tinh_trang_hieu_luc") or "?").strip()
    status[st] += 1
    if st.lower().startswith("còn hiệu lực") or st.lower().startswith("hết hiệu lực một phần"):
        inforce.append((d, m))

print("central thue docs        ", len(central))
print("v0.1.1 scope (buggy date)", len(buggy))
print("fixed date scope         ", len(fixed))
print("force status of fixed scope:")
for k, v in status.most_common():
    print(f"   {v:5d}  {k}")
print("in force (con / het mot phan)", len(inforce))

hq = [(d, m) for d, m in inforce if "hải quan" in (d["title"] + " " + (m.get("linh_vuc") or "") + " " + (m.get("nganh") or "")).lower()
      or "xuất khẩu" in d["title"].lower() or "nhập khẩu" in d["title"].lower()]
print("in force, customs/XNK related", len(hq))
types = collections.Counter(d["doc_type"] for d, _ in hq)
print("by type:", dict(types.most_common()))
print("by issuer:", dict(collections.Counter(d["issuing_body"] for d, _ in hq).most_common(8)))

key = ["luật hải quan", "54/2014/qh13", "08/2015/nđ-cp", "59/2018/nđ-cp", "38/2015/tt-btc", "39/2018/tt-btc",
       "134/2016/nđ-cp", "18/2021/nđ-cp", "26/2023/nđ-cp", "128/2020/nđ-cp", "31/2022/tt-btc", "107/2016/qh13",
       "167/2025/nđ-cp", "121/2025/tt-btc", "382/2025/nđ-cp", "90/2025/qh15"]
print("\nkey customs instruments:")
for k in key:
    hits = [d for d in docs.values() if k in (d["official_number"] + " " + d["title"]).lower()][:3]
    for d in hits:
        m = item(d) or {}
        tag = "IN SCOPE" if d in fixed else ("thue-subject" if d["id"] in thue else "NOT thue subject")
        print(f"  {k:16s} {d['official_number']:18s} {iso(d.get('effective_from')):10s} {(m.get('tinh_trang_hieu_luc') or '?'):22s} {tag:16s} {d['title'][:70]}")
    if not hits:
        print(f"  {k:16s} -- not in corpus")

print("\nnewest effective dates in corpus:", sorted(iso(d.get("effective_from")) for d in docs.values() if iso(d.get("effective_from"))[:1] == "2")[-3:])
print("sample in-force customs titles:")
for d, m in sorted(hq, key=lambda x: iso(x[0].get("effective_from")), reverse=True)[:15]:
    print("  ", iso(d.get("effective_from")), d["official_number"], "|", d["title"][:90])
