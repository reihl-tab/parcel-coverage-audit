# Parcel coverage audit / בודק התקדמות סריקת חלקות

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
