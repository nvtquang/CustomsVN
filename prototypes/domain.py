"""Tier every document by how close it is to customs work.

tier 1  core customs: procedures, XNK tax, origin, valuation, HS, trade remedies, bonded areas, smuggling
tier 2  adjacent: other taxes, trade, fees, foreign exchange, border and transport, and national
        instruments that tier 1 documents cite at least twice (the laws customs leans on)
tier 3  everything else: kept in the store, never retrieved for the customs bot

Writes D:\\DevCache\\luatdo\\rag\\domain.jsonl and prints the counts, plus the
quarantined documents that would have been tier 1 or 2.
"""
import json, os, re, collections

ROOT = r"D:\DevCache\luatdo"
PROVINCIAL = ("ubnd", "ủy ban nhân dân", "uỷ ban nhân dân", "hđnd", "hội đồng nhân dân")

CORE_SUBJECTS = {"thue/hai-quan", "thue/thue-xuat-nhap-khau", "thuong-mai/xuat-nhap-khau"}
NEAR_SUBJECTS = ("thue", "thuong-mai/canh-tranh", "thuong-mai/hoat-dong-thuong-mai",
                 "ngan-hang-tai-chinh/ngoai-hoi", "tai-chinh-cong/phi-le-phi")
CORE_CUES = ["hải quan", "xuất khẩu", "nhập khẩu", "xuất, nhập khẩu", "quá cảnh", "tạm nhập", "tái xuất",
             "tái nhập", "tạm xuất", "kho ngoại quan", "kho bảo thuế", "thu gom hàng lẻ", "xuất xứ hàng hóa",
             "xuất xứ hàng hoá", "trị giá hải quan", "phân loại hàng hóa", "mã số hàng hóa", "biểu thuế",
             "chế xuất", "phi thuế quan", "buôn lậu", "gian lận thương mại", "phòng vệ thương mại",
             "chống bán phá giá", "chống trợ cấp", "biện pháp tự vệ", "quản lý ngoại thương", "một cửa quốc gia",
             "một cửa asean", "kiểm tra chuyên ngành", "kiểm dịch", "hành lý", "cửa khẩu", "gia công hàng hóa",
             "chuyển khẩu", "tạm dừng làm thủ tục", "doanh nghiệp ưu tiên", "hàng hóa xuất", "hàng hóa nhập",
             "thuế xuất khẩu", "thuế nhập khẩu", "hạn ngạch thuế quan", "ưu đãi đặc biệt"]
NEAR_CUES = ["biên giới", "xuất cảnh", "nhập cảnh", "logistics", "vận tải quốc tế", "vận tải đa phương thức",
             "cảng biển", "cảng hàng không", "hiệp định thương mại", "miễn thuế", "hoàn thuế", "hóa đơn",
             "quản lý thuế", "thuế giá trị gia tăng", "thuế tiêu thụ đặc biệt", "thuế bảo vệ môi trường",
             "xử phạt vi phạm hành chính", "ngoại hối", "thương mại điện tử", "chất lượng sản phẩm, hàng hóa",
             "an toàn thực phẩm", "nhãn hàng hóa", "khu kinh tế", "bưu chính", "chuyển phát"]


def has(text, cues):
    t = text.lower()
    return [c for c in cues if c in t]


docs, subj = {}, {}
for f in os.listdir(os.path.join(ROOT, "docs")):
    d = json.load(open(os.path.join(ROOT, "docs", f), encoding="utf-8"))
    d.pop("provisions", None)
    docs[d["id"]] = d
for line in open(os.path.join(ROOT, "subject", "assignments.jsonl"), encoding="utf-8"):
    r = json.loads(line)
    subj[r["doc_id"]] = {s["subject_id"] for s in r.get("subjects") or []}

tier, why = {}, {}
for i, d in docs.items():
    s = subj.get(i, set())
    local = any(p in (d.get("issuing_body") or "").lower() for p in PROVINCIAL)
    core = has(d["title"], CORE_CUES)
    if (s & CORE_SUBJECTS or core) and not local:
        tier[i], why[i] = 1, sorted(s & CORE_SUBJECTS) + core[:3]
    elif s & CORE_SUBJECTS or core or any(x == n or x.startswith(n + "/") for x in s for n in NEAR_SUBJECTS) \
            or has(d["title"], NEAR_CUES):
        tier[i], why[i] = 2, (sorted(s)[:2] + (core or has(d["title"], NEAR_CUES))[:3])
    else:
        tier[i], why[i] = 3, []

# the laws customs leans on: national documents cited by two or more tier 1 documents
cited = collections.Counter()
for i in [i for i, t in tier.items() if t == 1]:
    p = os.path.join(ROOT, "cite", i.replace(":", "_") + ".json")
    if os.path.exists(p):
        for l in json.load(open(p, encoding="utf-8")) or []:
            if l.get("to_doc") and l["to_doc"] != i:
                cited[l["to_doc"]] += 1
promoted = []
for j, n in cited.items():
    d = docs.get(j)
    if d and n >= 2 and tier.get(j) == 3 and not any(p in (d.get("issuing_body") or "").lower() for p in PROVINCIAL):
        tier[j], why[j] = 2, [f"cited by {n} tier-1 documents"]
        promoted.append((n, j))

with open(os.path.join(ROOT, "rag", "domain.jsonl"), "w", encoding="utf-8") as out:
    for i in sorted(tier):
        out.write(json.dumps({"id": i, "tier": tier[i], "why": why[i]}, ensure_ascii=False) + "\n")

live = lambda d: (d.get("force_status") or "").startswith(("Còn", "Hết hiệu lực một phần", "Chưa có"))
c = collections.Counter((tier[i], live(d), d["status"]) for i, d in docs.items())
print("tier  in-force  parsed / quarantined / metadata")
for t in (1, 2, 3):
    print(f"  {t}   live   {c[(t, True, 'parsed')]:6d} / {c[(t, True, 'quarantined')]:5d} / {c[(t, True, 'metadata')]:4d}"
          f"     expired {sum(v for (tt, l, _), v in c.items() if tt == t and not l):6d}")
print("\npromoted by citation (top 15):")
for n, j in sorted(promoted, reverse=True)[:15]:
    print(f"  {n:4d}  {docs[j]['official_number']:16s} {docs[j].get('force_status',''):22s} {docs[j]['title'][:70]}")

print("\nQUARANTINED, tier 1 or 2, in force:")
q = [d for i, d in docs.items() if d["status"] == "quarantined" and tier[i] <= 2 and live(d)]
qt = collections.Counter((tier[d["id"]], d["doc_type"]) for d in q)
print("  ", len(q), "documents;", dict(qt.most_common(12)))
qr = collections.Counter(re.sub(r"\d[\d/A-ZĐa-z-]*", "#", d.get("quarantine") or "") for d in q)
print("  reasons:", dict(qr.most_common(4)))
rank = {"Luật": 0, "Bộ luật": 0, "Pháp lệnh": 1, "Nghị quyết": 2, "Nghị định": 2, "Thông tư": 3, "Thông tư liên tịch": 3,
        "Quyết định": 4, "Chỉ thị": 5, "Lệnh": 6}
q.sort(key=lambda d: (tier[d["id"]], rank.get(d["doc_type"], 9), d["id"]))
for d in q[:30]:
    print(f"   t{tier[d['id']]} {d['doc_type']:12s} {d['official_number']:18s} {d.get('issuing_body','')[:22]:22s} {d['title'][:80]}")
json.dump([d["id"] for d in q], open(os.path.join(ROOT, "rag", "rescue_ids.json"), "w", encoding="utf-8"))
