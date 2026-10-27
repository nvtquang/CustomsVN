"""Build the in-force RAG corpus from the luatdo store.

Reads   D:\\DevCache\\luatdo\\docs\\*.json and subject\\assignments.jsonl
Writes  D:\\DevCache\\luatdo\\rag\\chunks.jsonl     one retrievable chunk per line
        D:\\DevCache\\luatdo\\rag\\documents.jsonl  every document and whether it was kept, and why
        D:\\DevCache\\luatdo\\rag\\summary.json
The store itself is not touched.
"""
import json, os, re, sys, collections

ROOT = r"D:\DevCache\luatdo"
TODAY = sys.argv[1] if len(sys.argv) > 1 else "2026-10-27"
MAX_CHARS = 4000  # an article longer than this is split at its clauses

KEEP = {"Còn hiệu lực": "in_force", "Hết hiệu lực một phần": "partly_expired", "Chưa có hiệu lực": "not_yet_in_force"}
DROP = {"Hết hiệu lực toàn bộ", "Ngưng hiệu lực", "Không còn phù hợp", "Chưa xác định", ""}


def iso(d):
    m = re.match(r"(\d{1,2})/(\d{1,2})/(\d{4})", d or "")
    return f"{m[3]}-{int(m[2]):02d}-{int(m[1]):02d}" if m else (d or "")


def decide(d):
    st = (d.get("force_status") or "").strip()
    exp = iso(d.get("expired_on"))
    if exp and exp <= TODAY and st != "Hết hiệu lực một phần":
        return None, f"expiry date {exp} has passed (source says '{st}')"
    if st in DROP or st not in KEEP:
        return None, f"force status '{st or 'none'}'"
    if d["status"] != "parsed":
        return None, f"in force but not usable: {d['status']}" + (f" ({d['quarantine']})" if d.get("quarantine") else "")
    eff = iso(d.get("effective_from"))
    if eff and eff > TODAY:
        return "not_yet_in_force", ""
    return KEEP[st], ""


subjects = {}
for line in open(os.path.join(ROOT, "subject", "assignments.jsonl"), encoding="utf-8"):
    r = json.loads(line)
    subjects[r["doc_id"]] = sorted({s["subject_id"] for s in r.get("subjects") or []})

out = os.path.join(ROOT, "rag")
os.makedirs(out, exist_ok=True)
chunks = open(os.path.join(out, "chunks.jsonl"), "w", encoding="utf-8")
manifest = open(os.path.join(out, "documents.jsonl"), "w", encoding="utf-8")
stats = collections.Counter()
reasons = collections.Counter()


def emit(doc, force, pid, path, text):
    if not text.strip():
        return
    stats["chunks"] += 1
    chunks.write(json.dumps({
        "id": pid, "doc_id": doc["id"], "official_number": doc["official_number"],
        "doc_title": doc["title"], "doc_type": doc["doc_type"], "issuing_body": doc.get("issuing_body", ""),
        "effective_from": iso(doc.get("effective_from")), "force": force,
        "subjects": subjects.get(doc["id"], []), "path": path, "text": text,
        "source_url": doc.get("source_url", ""),
    }, ensure_ascii=False) + "\n")


for f in sorted(os.listdir(os.path.join(ROOT, "docs"))):
    doc = json.load(open(os.path.join(ROOT, "docs", f), encoding="utf-8"))
    force, why = decide(doc)
    manifest.write(json.dumps({"id": doc["id"], "official_number": doc["official_number"], "title": doc["title"],
                               "force_status": doc.get("force_status"), "expired_on": iso(doc.get("expired_on")),
                               "kept": bool(force), "force": force, "reason": why}, ensure_ascii=False) + "\n")
    if not force:
        stats["docs_dropped"] += 1
        reasons[re.sub(r"\(.*", "", re.sub(r"\d{4}-\d{2}-\d{2}", "<date>", why)).strip()] += 1
        continue
    stats["docs_kept"] += 1
    stats[f"docs_{force}"] += 1

    ps = doc.get("provisions") or []
    byid = {p["id"]: p for p in ps}

    def heading_path(pid):
        # walk up the identifier: doc:chapter-1:article-3:clause-2 -> labels of each ancestor in the store
        parts, labels = pid[len(doc["id"]) + 1:].split(":"), []
        for i in range(1, len(parts) + 1):
            p = byid.get(doc["id"] + ":" + ":".join(parts[:i]))
            if p:
                kind = {"chapter": "Chương", "section": "Mục", "article": "Điều", "clause": "Khoản",
                        "point": "Điểm", "annex": "Phụ lục"}.get(p["kind"], p["kind"])
                labels.append(f"{kind} {p.get('number', '')}".strip() + (f". {p['heading']}" if p.get("heading") else ""))
        return labels

    # one chunk per article holding its clauses and points, split at clauses when too long
    tops = [p for p in ps if p["kind"] in ("article", "annex")]
    for top in tops:
        kids = [p for p in ps if p["id"].startswith(top["id"] + ":")]
        whole = "\n".join(filter(None, [top.get("text") or ""] + [
            (f"{p.get('number')}. " if p["kind"] == "clause" else f"{p.get('number')}) " if p["kind"] == "point" else "") + (p.get("text") or "")
            for p in kids]))
        path = heading_path(top["id"])
        if len(whole) <= MAX_CHARS:
            emit(doc, force, top["id"], path, whole)
            continue
        stats["articles_split"] += 1
        if top.get("text"):
            emit(doc, force, top["id"], path, top["text"])
        for c in [p for p in kids if p["kind"] == "clause"] or kids:
            sub = "\n".join(filter(None, [c.get("text") or ""] + [p.get("text") or "" for p in kids if p["id"].startswith(c["id"] + ":")]))
            emit(doc, force, c["id"], heading_path(c["id"]), sub)

chunks.close(); manifest.close()
summary = {"as_of": TODAY, **stats, "drop_reasons": dict(reasons.most_common())}
json.dump(summary, open(os.path.join(out, "summary.json"), "w", encoding="utf-8"), ensure_ascii=False, indent=2)
print(json.dumps(summary, ensure_ascii=False, indent=2))
