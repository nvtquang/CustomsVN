"""Build the customs RAG corpus.

Sources, in the order a retriever should trust them:
  consolidated  VBHN whose base instrument is still in force, latest consolidation per base
  original      in-force documents from the luatdo store, tier 1 and 2
  rescued       in-force documents luatdo quarantined on a false alarm, or that have no "Điều" structure

Writes D:\\DevCache\\luatdo\\rag\\customs\\{chunks,documents}.jsonl and summary.json.
Every chunk carries tier (1 core customs, 2 adjacent) so a retriever filters rather than forgets.
"""
import json, os, re, sys, collections

ROOT = r"D:\DevCache\luatdo"
OUT = os.path.join(ROOT, "rag", "customs")
TODAY = sys.argv[1] if len(sys.argv) > 1 else "2026-10-27"
MAX = 4000      # an article longer than this is split at clauses
SPLIT = 2500    # anything still longer is cut at line boundaries
KIND_VI = {"chapter": "Chương", "section": "Mục", "article": "Điều", "clause": "Khoản", "point": "Điểm", "annex": "Phụ lục"}
NUM = r"\d+[a-z]?/\d{4}/[A-ZĐa-zđ0-9&-]+"


def iso(d):
    m = re.match(r"(\d{1,2})/(\d{1,2})/(\d{4})", (d or "").strip())
    return f"{m[3]}-{int(m[2]):02d}-{int(m[1]):02d}" if m else (d or "")


def norm_num(n):
    return re.sub(r"[^A-Z0-9/]", "", n.upper().replace("Đ", "D"))


def force_of(d):
    st = (d.get("force_status") or "").strip()
    exp = iso(d.get("expired_on"))
    if exp and exp <= TODAY and st != "Hết hiệu lực một phần":
        return None
    if iso(d.get("effective_from")) > TODAY and st in ("Còn hiệu lực", "Chưa có hiệu lực"):
        return "not_yet_in_force"
    return {"Còn hiệu lực": "in_force", "Hết hiệu lực một phần": "partly_expired", "Chưa có hiệu lực": "not_yet_in_force"}.get(st)


tier = {}
for line in open(os.path.join(ROOT, "rag", "domain.jsonl"), encoding="utf-8"):
    r = json.loads(line)
    tier[r["id"]] = r["tier"]
subjects = {}
for line in open(os.path.join(ROOT, "subject", "assignments.jsonl"), encoding="utf-8"):
    r = json.loads(line)
    subjects[r["doc_id"]] = sorted({s["subject_id"] for s in r.get("subjects") or []})

os.makedirs(OUT, exist_ok=True)
chunks = open(os.path.join(OUT, "chunks.jsonl"), "w", encoding="utf-8")
manifest = open(os.path.join(OUT, "documents.jsonl"), "w", encoding="utf-8")
stats = collections.Counter()


def emit(doc, meta, cid, path, text, extra=None):
    text = text.strip()
    if not text:
        return
    pieces = [text]
    if len(text) > SPLIT * 1.6:  # tables and schedules: cut at line boundaries, repeat the path on every piece
        pieces, cur = [], ""
        for line in text.split("\n"):
            if cur and len(cur) + len(line) > SPLIT:
                pieces.append(cur); cur = ""
            cur += ("\n" if cur else "") + line
        if cur:
            pieces.append(cur)
        stats["chunks_cut_long"] += 1
    for n, piece in enumerate(pieces):
        stats["chunks"] += 1
        stats[f"chunks_{meta['source_kind']}"] += 1
        stats[f"chunks_tier{meta['tier']}"] += 1
        chunks.write(json.dumps({
            "id": cid if len(pieces) == 1 else f"{cid}#part-{n + 1}",
            "doc_id": doc["id"], "official_number": doc["official_number"], "doc_title": doc["title"],
            "doc_type": doc.get("doc_type", ""), "issuing_body": doc.get("issuing_body", ""),
            "effective_from": iso(doc.get("effective_from")), "signed_on": iso(doc.get("signed_on")),
            "path": path, "text": piece, "source_url": doc.get("source_url", ""), **meta, **(extra or {}),
        }, ensure_ascii=False) + "\n")


def chunk_tree(doc, meta, notes=None):
    ps = doc.get("provisions") or []
    byid = {p["id"]: p for p in ps}

    def path_of(pid):
        parts, labels = pid[len(doc["id"]) + 1:].split(":"), []
        for i in range(1, len(parts) + 1):
            p = byid.get(doc["id"] + ":" + ":".join(parts[:i]))
            if p and p["kind"] in KIND_VI:
                labels.append(f"{KIND_VI[p['kind']]} {p.get('number', '')}".strip() + (f". {p['heading']}" if p.get("heading") else ""))
        return labels

    def note_refs(text):
        if not notes:
            return {}
        found = [notes[k] for k in re.findall(r"\[(\d+)\]", text) if k in notes]
        return {"amendment_notes": found[:12]} if found else {}

    tops = [p for p in ps if p["kind"] in ("article", "annex")]
    if not tops:  # a structure with chapters only, or clauses with no article
        tops = [p for p in ps if not p.get("parent_id") or p.get("parent_id") == doc["id"]]
    for top in tops:
        kids = [p for p in ps if p["id"].startswith(top["id"] + ":")]
        mark = lambda p: f"{p.get('number')}. " if p["kind"] == "clause" else f"{p.get('number')}) " if p["kind"] == "point" else ""
        whole = "\n".join(filter(None, [top.get("text") or ""] + [mark(p) + (p.get("text") or "") for p in kids]))
        if len(whole) <= MAX:
            emit(doc, meta, top["id"], path_of(top["id"]), whole, note_refs(whole))
            continue
        if top.get("text"):
            emit(doc, meta, top["id"], path_of(top["id"]), top["text"], note_refs(top["text"]))
        for c in [p for p in kids if p["kind"] == "clause"] or kids:
            sub = "\n".join(filter(None, [c.get("text") or ""] + [mark(p) + (p.get("text") or "") for p in kids if p["id"].startswith(c["id"] + ":")]))
            emit(doc, meta, c["id"], path_of(c["id"]), sub, note_refs(sub))


def chunk_text(doc, meta, text):
    """A document with no Điều structure: cut at paragraphs, drop the national header and signature block."""
    body = re.split(r"\n(?:Nơi nhận:|TM\. |KT\. )", text)[0]
    paras, cur, n = [p.strip() for p in body.split("\n") if p.strip()], "", 0
    for p in paras + [None]:
        if p is None or (cur and len(cur) + len(p) > 1500):
            n += 1
            emit(doc, meta, f"{doc['id']}:para-{n}", [doc["title"][:120]], cur)
            cur = ""
        if p:
            cur += ("\n" if cur else "") + p


# ---- 1. consolidated texts
store_by_num = {}
for f in os.listdir(os.path.join(ROOT, "docs")):
    if f.endswith(".json"):
        pass
docs_dir = os.path.join(ROOT, "docs")


def load_store(doc_id):
    p = os.path.join(docs_dir, doc_id.replace(":", "_") + ".json")
    return json.load(open(p, encoding="utf-8")) if os.path.exists(p) else None


def store_id(number):
    m = re.match(r"(\d+[a-z]?)/(\d{4})/(.+)$", number)
    if not m:
        return None
    return f"vn:law:{m[2]}:{m[1].lower()}-{m[2]}-" + re.sub(r"[^a-z0-9]+", "-", m[3].lower().replace("đ", "d")).strip("-")


vbhn = []
for f in os.listdir(os.path.join(ROOT, "extra", "vbhn")):
    o = json.load(open(os.path.join(ROOT, "extra", "vbhn", f), encoding="utf-8"))
    if len(o["text"]) < 200:
        stats["vbhn_empty_at_source"] += 1
        continue
    t, d = o["text"], o["doc"]
    head = t[: t.find("Điều 1") if "Điều 1" in t else 3000]
    nums = re.findall(NUM, d["title"]) + re.findall(NUM, head)
    split = re.split(r"được sửa đổi, bổ sung bởi|được sửa đổi bởi|được bổ sung bởi", head, maxsplit=1)
    base = (re.findall(NUM, d["title"]) or re.findall(NUM, split[0]) or [None])[0]
    amenders = [n for n in dict.fromkeys(re.findall(NUM, re.split(r"\n\s*Căn cứ", split[1] if len(split) > 1 else "")[0])) if not base or norm_num(n) != norm_num(base)] if len(split) > 1 else []
    notes = {}
    for m in re.finditer(r"(?m)^\s*\[(\d+)\]\s*(.+)$", t):
        if re.search(r"sửa đổi|bổ sung|bãi bỏ|thay thế|hợp nhất|hiệu lực", m[2]):
            notes[m[1]] = m[2].strip()[:400]
    vbhn.append((o, base, amenders, notes))

latest = {}
for o, base, amenders, notes in vbhn:
    key = norm_num(base) if base else o["doc"]["id"]
    if key not in latest or iso(o["doc"]["signed_on"]) > iso(latest[key][0]["doc"]["signed_on"]):
        latest[key] = (o, base, amenders, notes)
stats["vbhn_superseded_by_newer_consolidation"] = len(vbhn) - len(latest)

consolidated_base = {}
for key, (o, base, amenders, notes) in latest.items():
    d = o["doc"]
    bdoc = load_store(store_id(base)) if base else None
    bforce = force_of(bdoc) if bdoc else None
    if bdoc and not bforce:
        stats["vbhn_base_expired"] += 1
        manifest.write(json.dumps({"id": d["id"], "kept": False, "reason": f"base {base} no longer in force"}, ensure_ascii=False) + "\n")
        continue
    if bdoc:
        consolidated_base[bdoc["id"]] = d["id"]
    meta = {"source_kind": "consolidated", "tier": o.get("tier") or 2, "force": bforce or "unknown_base",
            "subjects": subjects.get(bdoc["id"], []) if bdoc else [],
            "consolidates": {"base": base, "base_doc_id": bdoc["id"] if bdoc else None, "amended_by": amenders}}
    stats["vbhn_kept"] += 1
    stats["vbhn_base_resolved" if bdoc else "vbhn_base_unresolved"] += 1
    manifest.write(json.dumps({"id": d["id"], "kept": True, "official_number": d["official_number"], "title": d["title"],
                               "base": base, "amended_by": amenders, "base_force": bforce, "notes": len(notes)}, ensure_ascii=False) + "\n")
    if d["status"] == "parsed":
        chunk_tree(d, meta, notes)
    else:
        chunk_text(d, meta, o["text"])

# ---- 2. originals from the store
for f in sorted(os.listdir(docs_dir)):
    i = f[:-5].replace("_", ":", 3)
    d = json.load(open(os.path.join(docs_dir, f), encoding="utf-8"))
    t = tier.get(d["id"], 3)
    force = force_of(d)
    if t > 2 or not force or d["status"] != "parsed":
        continue
    meta = {"source_kind": "original", "tier": t, "force": force, "subjects": subjects.get(d["id"], [])}
    if d["id"] in consolidated_base:
        meta["consolidated_in"] = consolidated_base[d["id"]]  # prefer the VBHN; this wording may be stale
    stats[f"docs_original_tier{t}"] += 1
    manifest.write(json.dumps({"id": d["id"], "kept": True, "tier": t, "force": force, "source_kind": "original",
                               "consolidated_in": meta.get("consolidated_in")}, ensure_ascii=False) + "\n")
    chunk_tree(d, meta)

# ---- 3. rescued from quarantine
for f in os.listdir(os.path.join(ROOT, "extra", "rescue")):
    o = json.load(open(os.path.join(ROOT, "extra", "rescue", f), encoding="utf-8"))
    d = o["doc"]
    force = force_of(d)
    if not force:
        continue
    meta = {"source_kind": "rescued", "tier": tier.get(d["id"], 2), "force": force, "subjects": subjects.get(d["id"], []),
            "rescue_reason": o.get("verdict"), "needs_review": o.get("verdict") == "year-typo"}
    stats[f"docs_rescued_{d['status']}"] += 1
    manifest.write(json.dumps({"id": d["id"], "kept": True, "source_kind": "rescued", "verdict": o.get("verdict")}, ensure_ascii=False) + "\n")
    if d["status"] == "parsed":
        chunk_tree(d, meta)
    else:
        chunk_text(d, meta, o["text"])

chunks.close(); manifest.close()
summary = {"as_of": TODAY, **dict(sorted(stats.items()))}
json.dump(summary, open(os.path.join(OUT, "summary.json"), "w", encoding="utf-8"), ensure_ascii=False, indent=2)
print(json.dumps(summary, ensure_ascii=False, indent=2))
