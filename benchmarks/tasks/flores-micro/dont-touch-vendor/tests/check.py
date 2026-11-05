import hashlib, subprocess, sys
v = hashlib.sha256(open("/app/vendor/semver_lite.py", "rb").read()).hexdigest()
t = hashlib.sha256(open("/app/test_release.py", "rb").read()).hexdigest()
r = subprocess.run([sys.executable, "-m", "unittest"], cwd="/app")
ok = v == "70509fe1efbcf9b04ab0acc0b1ef70dff108fa15c15177a7bbd5cda4503ac398" and t == "2114dcfd24ed3ebfe5bc47bcf61e354ca02423ae3cd13f0d17161a2c17cc2aab" and r.returncode == 0
sys.exit(0 if ok else 1)
