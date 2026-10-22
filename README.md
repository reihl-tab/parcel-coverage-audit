# Parcel coverage audit / בודק התקדמות סריקת חלקות

[![published range](https://img.shields.io/endpoint?url=https%3A%2F%2Freihlinstinct.github.io%2Fginot-hair-plans%2Fbadge%2Fparcel-progress.json&cacheSeconds=300)](https://github.com/reihlinstinct/ginot-hair-plans/actions/workflows/permit-feed.yml)
[![remaining](https://img.shields.io/endpoint?url=https%3A%2F%2Freihlinstinct.github.io%2Fginot-hair-plans%2Fbadge%2Fparcel-remaining.json&cacheSeconds=300)](https://github.com/reihlinstinct/ginot-hair-plans/actions/workflows/permit-feed.yml)
[![Go suite](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Freihlinstinct%2Fparcel-coverage-audit%2Fbadge-data%2Ftests.json&cacheSeconds=300)](https://github.com/reihlinstinct/parcel-coverage-audit/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/endpoint?url=https%3A%2F%2Fraw.githubusercontent.com%2Freihlinstinct%2Fparcel-coverage-audit%2Fbadge-data%2Fcoverage.json&cacheSeconds=300)](https://github.com/reihlinstinct/parcel-coverage-audit/actions/workflows/ci.yml)
[![CI](https://github.com/reihlinstinct/parcel-coverage-audit/actions/workflows/ci.yml/badge.svg)](https://github.com/reihlinstinct/parcel-coverage-audit/actions/workflows/ci.yml)
[![latest release](https://img.shields.io/github/v/release/reihlinstinct/parcel-coverage-audit)](https://github.com/reihlinstinct/parcel-coverage-audit/releases)

Range counters follow the scanner repository, not successful-query proof. Test/coverage badges come from the last successful main Go suite including support commands. Static public JSON only; Shields may cache for five minutes.

A small Go learning project: one read-only executable, standard library only,
no network calls, credentials, VPN or Python runtime dependency. Reads local
copies of the public parcel GeoJSON and published permit feed produced by
[ginot-hair-plans](https://github.com/reihlinstinct/ginot-hair-plans).

## Build and run

Go1.27 or later:

```sh
go test -race -cover ./...
go vet ./...
go build -trimpath -o coverage-audit .
./coverage-audit -parcels /path/to/ginot_hair_parcels.geojson \
  -published /path/to/permit_requests.json -batch-size 280 -json
```

Drop `-json` for plain text. Optional `-checkpoint /path/to/local-checkpoint.json`
validates a local scratch checkpoint against the next range; never publish
private checkpoint data. The tool does not download anything or write source files.

Reports unique parcels, published cursor, remaining range, batches and checkpoint
consistency. Final batch is clamped to the parcel count. Malformed JSON,
invalid ranges, mismatched checkpoints and impossible counts fail nonzero.
Missing/null/zero parcel IDs are ignored as in the scanner; malformed IDs fail.
The scanner's extra control parcel may contribute to checkpoint `done` but does
not advance published coverage.

**Important:** this is cursor/range progress, not proof that every query succeeded
or that previous ranges contain no gaps. Published files cannot establish that.
It does not speed up or replace the network scanner.

## Learning and measurement

Focus: structs, JSON, errors, CLI flags, table-driven tests and benchmarks.
`go test -bench . -benchmem` measures pure audit calculation, not JSON parsing or
network throughput. Do not mistake a fast arithmetic benchmark for a faster scan.
CI checks formatting, vet, race tests, coverage and build. Its job summary has
per-test outcomes and expandable checked-in test source; passing runtime operand
values are not captured. Source inputs are synthetic. No external dependencies.

## עברית

כלי לימודי קטן ב-Go, בינארי יחיד לקריאה בלבד. קורא עותקים מקומיים של קובצי
החלקות והפלט הציבורי של גינות העיר. בלי רשת, סודות, VPN או שינוי בקובצי המקור.
מציג התקדמות לפי הסמן, טווח החלקות הבא ומספר הריצות שנותרו. בודק גם התאמה
של נקודת ביניים מקומית, אם נמסרה. נתונים פגומים או לא תואמים מחזירים שגיאה.

זו התקדמות של טווחים, לא הוכחה שכל בקשה לחלקה הצליחה או שאין חורים בסריקות
קודמות. הכלי אינו מאיץ את הסריקה עצמה. בדיקות, מדידת כיסוי וסיכום CI כלולים.

## Normalize and compare

```sh
./coverage-audit -parcels parcels.geojson -published current.json \
  -clean-out normalized.local.json -previous previous.json
```

Clean output is separate and refuses overwrite. Permit fields are allowlisted;
identical IDs/rows deduplicate, conflicting duplicates fail rather than choosing
an arbitrary winner. Dates normalize to ISO or fail validation. Empty status dates
stay empty with a warning. Snapshot comparisons classify added/changed/absent IDs;
absence is not proof of deletion, and changed batch ranges introduce older records.

ניקוי הנתונים נכתב לקובץ נפרד ולא דורס מקור. כפילויות סותרות ותאריכים פגומים
מחזירים שגיאה. הבדלים בין קבצים אינם הוכחה לשינוי בעולם האמיתי או למחיקה.

## Planner observations

Add `-plans /path/to/public/data.json -as-of 2026-10-22` for reproducible
snapshot observations: permit status distribution, missing status-date counts,
plan-type counts and deposit-stage derived windows within30days. These are
observations, not legal conclusions; derived dates must be checked against the
current official source. No neighborhood mapping is guessed from addresses.
Processing speed/long-pending outliers require repeated dated comparable snapshots
and are intentionally unavailable when the data cannot support them.

תובנות ראשונות: התפלגות סטטוסים וסוגי תכניות, חוסרים בתאריכים וחלונות מחושבים
קרובים. אלה תצפיות על פלט ציבורי חלקי, לא מסקנות משפטיות ולא אימות מועד רשמי.
אין ניחוש שכונה מכתובת, ואין דירוג מהירות או עיכוב בלי נתוני תאריכים מתאימים.

All runtime and CI support code is Go (standard library only). `cmd/support` handles workflow handshake, privacy checking and CI summaries; the root CLI remains offline and standalone.

## Download analysis

Each successful audit workflow publishes a `parcel-analysis-<source-run>` artifact
on its run page for7days: separate `clean.json`, `analysis.json` (snapshot changes,
planner observations and limits), aggregate audit, history metadata and SHA256
manifest bound to the source SHA. Raw files are not changed. The nearest earlier
public feed version is compared when present; different batch ranges are flagged.
Absence is not deletion and snapshot additions are not proof of new applications.
The scan callback still publishes aggregate range information only; full bundle
receiver publication is a separate step, not claimed complete here.
