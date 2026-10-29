# Scheduled update from vbpl.vn

The corpus starts from the th1nhng0 dataset on Hugging Face, pinned at `prep.Revision` (see NOTICE). The dataset is refreshed upstream only now and then (last on 2026-07-23), so on a schedule (monthly by default, or weekly) CustomsVN fetches what vbpl.vn has added or changed since, with the crawler the dataset itself publishes, and merges it into a new revision of the same data. luatdo, prep and ingest then read that revision as they read the snapshot.

In scope for v1.0 since 2026-10-29 (owner's decision).

## What a run does

`internal/crawl`, run as the `crawl` job (Data screen, or by the server on its schedule) or by `customsvn crawl` (`-rotate=false` fetches again only the documents the new ones act on, for a quick run by hand):

1. **New documents.** The spider walks the newest catalogue pages of vbpl.vn (`-max-pages`, 100 documents a page, default 20) and fetches every document the latest revision does not have. The catalogue lists the newest first.
2. **Scope.** A new document is kept when its title, field or sector carries a customs or related cue (the tier 1 and tier 2 cues of `internal/corpus/tier.go`), or when it relates to a document already in tiers 1-2 (an amendment of a customs decree need not say "hải quan"). The rest are dropped.
3. **Status.** Documents in tiers 1-2 are fetched again: those the new documents replace, repeal, amend, consolidate or suspend, and the others: all of them on a monthly schedule, a quarter in turn on a weekly one (by a hash of the id and the ISO week), so every status the portal changed is picked up within about a month either way. A full pass over the scope is about 13,000 requests, a few hours at the rates below. A document counts as changed when its status, effective or expiry date, or title differs.
4. **Merge.** The kept and changed documents go through the dataset's `build_full_dataset.py`, then its `upsert_metadata` and `upsert_relationships` functions, into `raw/th1nhng0/vbpl-<UTC time>/data/`, a full copy of the latest revision with them merged, with a `manifest.json` written as luatdo's fetch writes one. The content table is merged by `internal/crawl/merge.go`, row by row with the same rule (a fetched document replaces its id's row): the dataset's `upsert_content` holds all 3.5 GB of HTML in memory and was killed in a 6 GB Docker VM on 2026-10-29. luatdo takes the revision with the latest `fetched_at`, and so does prep (`prep.LatestRevision`). The legacy configs are read from the snapshot only.
5. **The rest of the job.** `luatdo parse --dataset th1nhng0`, `cite`, `subjects`, then prep, ingest (a new corpus version, loaded at once) and embed for the new chunks, unless the knowledge screen is set to economy or no embedding model is loaded.

A run that finds nothing new or changed writes nothing and stops after step 3. Crawl revisions are pruned to the last two; the Hugging Face snapshot is never modified or removed. The work files of the last eight runs (ids, the spider's JSON lines and logs) stay in `customsvn/crawl/`.

## Politeness

The spider runs with 4 requests in flight, half a second between requests, AutoThrottle on, and a user agent naming CustomsVN. The dataset's own settings (100 requests in flight, rotating proxies) are not used. The rate measured on 2026-10-29 varied from 5 to 90 documents a minute with the portal's response time.

## Schedule

The Data screen sets the frequency (monthly on a day 1-28, or weekly on a weekday), the hour (server time, `TZ=Asia/Ho_Chi_Minh` in the image) and the number of catalogue pages; the default is the 1st of the month at 02:00, 20 pages (owner's choice, 2026-10-29). The server checks every five minutes whether a crawl job has been queued since the last slot, so a server that was down at the slot catches up when it starts. The very first crawl waits for the first slot after the server starts (or "Chạy ngay"): a new install does not crawl at once for a slot it never saw. "Chạy ngay" queues one at once.

## Installing the crawler

The Docker image downloads the dataset's `crawler/` directory at the pinned revision into `/opt/vbpl-crawler` and installs the versions its `uv.lock` pins (Scrapy, polars, pyarrow). Outside Docker:

```sh
mkdir vbpl-crawler && cd vbpl-crawler
# the files under crawler/ of th1nhng0/vietnamese-legal-documents at the pinned revision
uv export --frozen --no-dev --no-hashes --no-emit-project -o requirements.txt
python -m pip install -r requirements.txt
export CUSTOMSVN_CRAWLER=$PWD CUSTOMSVN_PYTHON=python
```

| Variable | Default | |
| --- | --- | --- |
| `CUSTOMSVN_CRAWLER` | `/opt/vbpl-crawler` | the crawler directory (with `scrapy.cfg`) |
| `CUSTOMSVN_PYTHON` | `python3` | an interpreter with its requirements |
| `CUSTOMSVN_LUATDO` | `luatdo` | the luatdo binary |
| `CUSTOMSVN_VBPL_ACTION` | the dataset's | the vbpl.vn catalogue action, when the portal is redeployed and the old one stops answering |

## When it fails

- **"the vbpl.vn catalogue did not parse"**: the portal changed its catalogue server action. Find the new `Next-Action` header of the catalogue request in the browser's network panel on https://vbpl.vn/van-ban/trung-uong and set `CUSTOMSVN_VBPL_ACTION`.
- Each step's output is in the job's log; the spider's own logs are in `customsvn/crawl/<run>/`.

## Measured

2026-10-29, one catalogue page against a scratch copy of the data: 82 of the 100 newest documents were new to the snapshot, 2 of them in scope.
