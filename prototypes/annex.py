import json, os, collections

D = r"D:\DevCache\luatdo\docs"
KEY = ["vn_law_2023_26-2023-nd-cp", "vn_law_2025_199-2025-nd-cp", "vn_law_2025_206-2025-nd-cp",
       "vn_law_2015_38-2015-tt-btc", "vn_law_2018_39-2018-tt-btc", "vn_law_2022_31-2022-tt-btc",
       "vn_law_2015_08-2015-nd-cp", "vn_law_2025_167-2025-nd-cp", "vn_law_2016_134-2016-nd-cp",
       "vn_law_2014_54-2014-qh13", "vn_law_2016_107-2016-qh13", "vn_law_2025_121-2025-tt-btc",
       "vn_law_2021_118-2021-nd-cp"]

def load(name):
    p = os.path.join(D, name + ".json")
    return json.load(open(p, encoding="utf-8")) if os.path.exists(p) else None

for k in KEY:
    d = load(k)
    if not d:
        print(f"{k:32s} MISSING"); continue
    ps = d.get("provisions") or []
    kinds = collections.Counter(p["kind"] for p in ps)
    annex = [p for p in ps if p["kind"] == "annex"]
    atext = sum(len(p.get("text") or "") for p in annex)
    total = sum(len(p.get("text") or "") for p in ps)
    print(f"{k:32s} status={d['status']:11s} provisions={len(ps):5d} kinds={dict(kinds)} annex_chars={atext} total_chars={total}")

# what does a tariff annex actually look like
d = load("vn_law_2023_26-2023-nd-cp")
if d:
    for p in [p for p in d["provisions"] if p["kind"] == "annex"][:4]:
        t = p.get("text") or ""
        print("\n---", p["id"], "|", p.get("heading", "")[:80], "| chars", len(t))
        print(t[:700])
    # does a known HS code survive as text?
    blob = "\n".join(p.get("text") or "" for p in d["provisions"])
    for code in ["0101.21.00", "01012100", "8703.23", "87032"]:
        i = blob.find(code)
        print(f"\nHS {code}: ", "found -> " + blob[max(0, i - 80):i + 160].replace("\n", " / ") if i >= 0 else "not found")
