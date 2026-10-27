"""Pick the raw rows the Go step should parse: customs VBHN, and quarantined documents worth rescuing."""
import json, os, re, glob, collections, sys
import pyarrow.parquet as pq

sys.path.insert(0, os.path.dirname(__file__))
ROOT = r"D:\DevCache\luatdo"
R = glob.glob(ROOT + r"\raw\th1nhng0\*\data")[0]
exec(open(os.path.join(os.path.dirname(__file__), "domain.py"), encoding="utf-8").read().split("docs, subj = {}, {}")[0])  # cues only

meta = {str(r["id"]): r for r in pq.read_table(R + r"\metadata.parquet").to_pylist()}


def slug(s):
    s = s.lower().replace("đ", "d")
    import unicodedata
    s = "".join(c for c in unicodedata.normalize("NFD", s) if unicodedata.category(c) != "Mn")
    return re.sub(r"[^a-z0-9]+", "-", s).strip("-")


def year(d):
    m = re.search(r"(\d{4})$", d or "")
    return m[1] if m else ""


sel = []
# 1. consolidated texts about customs and its neighbours
vb = collections.Counter()
for iid, m in meta.items():
    if (m["loai_van_ban"] or "") != "Văn bản hợp nhất" and "VBHN" not in (m["so_ky_hieu"] or "").upper():
        continue
    vb["all"] += 1
    t = (m["title"] or "") + " " + (m["linh_vuc"] or "") + " " + (m["nganh"] or "")
    tier = 1 if has(t, CORE_CUES) else 2 if has(t, NEAR_CUES) else 3
    vb[f"tier{tier}"] += 1
    if tier == 3:
        continue
    num = (m["so_ky_hieu"] or "").strip()
    y = year(m["ngay_ban_hanh"]) or "0000"
    sel.append({"item_id": iid, "kind": "vbhn", "tier": tier,
                "id": f"vn:vbhn:{y}:{slug(num)}:{slug(m['co_quan_ban_hanh'] or 'unknown')}:{iid}",
                "official_number": num, "title": m["title"], "doc_type": "Văn bản hợp nhất",
                "issuing_body": m["co_quan_ban_hanh"] or "", "signed_on": m["ngay_ban_hanh"] or "",
                "effective_from": m["ngay_co_hieu_luc"] or "", "expired_on": m["ngay_het_hieu_luc"] or "",
                "force_status": m["tinh_trang_hieu_luc"] or ""})
print("VBHN rows", dict(vb))

# 2. quarantined documents in tiers 1 and 2 that are still in force
def norm(n):
    n = n.upper().replace("Đ", "D").replace(" ", "")
    n = re.sub(r"[^A-Z0-9/]", "", n)
    return "/".join(p.lstrip("0") for p in n.split("/"))

verdicts = collections.Counter()
for i in json.load(open(ROOT + r"\rag\rescue_ids.json", encoding="utf-8")):
    d = json.load(open(ROOT + "\\docs\\" + i.replace(":", "_") + ".json", encoding="utf-8"))
    q = d.get("quarantine") or ""
    m = re.match(r"body carries official number (.+), metadata says (.+)$", q)
    if m:
        body, mine = norm(m[1]), norm(m[2])
        bp, mp = body.split("/"), mine.split("/")
        if body == mine or mine.startswith(body) or body.startswith(mine):
            verdict = "format"            # 09 vs 9, & vs nothing, a truncated suffix: the same instrument
        elif len(bp) >= 3 and len(mp) >= 3 and bp[0] == mp[0] and bp[2:] == mp[2:]:
            verdict = "year-typo"         # same number and suffix, another year: likely a typo at the source
        elif len(bp) >= 3 and len(mp) >= 3 and bp[1:] == mp[1:] and bp[0] != mp[0]:
            verdict = "number-differs"    # another number entirely: maybe another instrument's text
        else:
            verdict = "other-document"
    else:
        verdict = "no-structure"
    verdicts[verdict] += 1
    if verdict in ("number-differs", "other-document"):
        continue
    iid = re.search(r"ItemID=(\d+)", d["source_url"])[1]
    sel.append({"item_id": iid, "kind": "rescue", "verdict": verdict, "id": d["id"],
                **{k: d.get(k, "") for k in ("official_number", "title", "doc_type", "issuing_body", "signed_on",
                                             "effective_from", "expired_on", "force_status")}})
print("quarantine verdicts", dict(verdicts))
json.dump(sel, open(ROOT + r"\rag\select.json", "w", encoding="utf-8"), ensure_ascii=False)
print("selected", collections.Counter(s["kind"] for s in sel))
