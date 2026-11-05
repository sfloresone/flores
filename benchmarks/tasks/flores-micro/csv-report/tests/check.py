import csv, glob, json, sys
by = {}
for f in glob.glob("/app/data/*.csv"):
    for r in csv.DictReader(open(f)):
        if r["status"] == "paid":
            by[r["region"]] = by.get(r["region"], 0) + float(r["amount"])
try:
    got = json.load(open("/app/report.json"))
except Exception as e:
    print(e); sys.exit(1)
close = lambda a, b: abs(float(a) - b) < 0.011
ok = set(got.get("by_region", {})) == set(by)
ok = ok and all(close(got["by_region"][k], v) for k, v in by.items())
ok = ok and close(got.get("total", -1), sum(by.values()))
sys.exit(0 if ok else 1)
