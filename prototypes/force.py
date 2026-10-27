import json, os, re, collections

D = r"D:\DevCache\luatdo\docs"

def iso(d):
    m = re.match(r"(\d{1,2})/(\d{1,2})/(\d{4})", d or "")
    return f"{m[3]}-{int(m[2]):02d}-{int(m[1]):02d}" if m else (d or "")

by_status = collections.Counter()
by_status_parsed = collections.Counter()
empty_text = collections.Counter()
lengths = []
future = 0
for f in os.listdir(D):
    d = json.load(open(os.path.join(D, f), encoding="utf-8"))
    st = d.get("force_status") or "(trống)"
    by_status[st] += 1
    by_status_parsed[(st, d["status"])] += 1
    if st.startswith("Còn") or st.startswith("Hết hiệu lực một phần") or st.startswith("Chưa"):
        if d["status"] == "parsed":
            for p in d.get("provisions") or []:
                t = p.get("text") or ""
                if p["kind"] in ("clause", "point", "article", "annex"):
                    lengths.append(len(t))
                    if not t and p["kind"] != "article":
                        empty_text[p["kind"]] += 1
        if iso(d.get("effective_from")) > "2026-10-27":
            future += 1

print("force_status over all documents:")
for k, v in by_status.most_common():
    print(f"  {v:7d}  {k}")
print("\nby force_status x parse status:")
for (st, ps), v in sorted(by_status_parsed.items(), key=lambda x: -x[1])[:16]:
    print(f"  {v:7d}  {st:28s} {ps}")
lengths.sort()
n = len(lengths)
pct = lambda q: lengths[int(q * (n - 1))]
print(f"\nliving provisions (article/clause/point/annex): {n}")
print(f"  text length p10={pct(.1)} p50={pct(.5)} p90={pct(.9)} p99={pct(.99)} max={lengths[-1]}")
print(f"  empty text: {sum(1 for x in lengths if x == 0)}   over 4000 chars: {sum(1 for x in lengths if x > 4000)}   over 20000: {sum(1 for x in lengths if x > 20000)}")
print("  not yet effective (after today):", future)
