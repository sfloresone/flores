import json, os, subprocess, sys, tempfile
d = tempfile.mkdtemp()
a, b = os.path.join(d, "a.txt"), os.path.join(d, "b.txt")
open(a, "w").write("one two\nthree\n")
open(b, "w").write("x y z")
run = lambda *args: subprocess.run(["/app/bin/wcl", *args], capture_output=True, text=True)
try:
    r1 = run(a, b); r2 = run("--json", a); r3 = run(a, os.path.join(d, "nope"))
except OSError as e:
    print(e); sys.exit(1)
ok = r1.returncode == 0 and r1.stdout.splitlines() == [f"2 3 14 {a}", f"0 3 5 {b}", "2 6 19 total"]
ok = ok and r2.returncode == 0 and json.loads(r2.stdout) == {a: {"lines": 2, "words": 3, "bytes": 14}}
ok = ok and r3.returncode == 1 and r3.stdout.strip() == f"2 3 14 {a}" and "not found" in r3.stderr
print(r1.stdout, r2.stdout, r3.stdout, r3.stderr)
sys.exit(0 if ok else 1)
