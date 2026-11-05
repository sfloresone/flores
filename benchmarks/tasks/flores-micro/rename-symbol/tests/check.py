import pathlib, subprocess, sys
left = [str(p) for p in pathlib.Path("/app").rglob("*.py") if "calc_total" in p.read_text()]
r = subprocess.run([sys.executable, "-m", "unittest", "discover", "-s", "tests", "-t", "."], cwd="/app")
sys.path.insert(0, "/app")
from shop import compute_total
from shop.cart import compute_total as c2
ok = not left and r.returncode == 0 and compute_total([(1, 1)]) == 1.21 and c2 is compute_total
print("leftovers:", left)
sys.exit(0 if ok else 1)
