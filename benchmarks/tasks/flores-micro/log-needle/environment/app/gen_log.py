"""Generate a large, deterministic access log (~300k lines)."""
import random

rng = random.Random(42)
statuses = [200] * 80 + [404] * 10 + [500] * 5 + [503] * 5
with open("/app/logs/access.log", "w") as f:
    for i in range(300_000):
        user = f"u-{rng.randint(1000, 9999)}"
        status = rng.choice(statuses)
        if user == "u-4242" and status == 503:
            status = 200  # keep the needle unique
        rid = f"req-{rng.getrandbits(48):012x}"
        f.write(f"2026-11-05T10:{i // 6000 % 60:02d}:{i % 60:02d}Z GET /api/items/{i % 977} "
                f"status={status} user={user} request_id={rid} ms={rng.randint(1, 900)}\n")
        if i == 187_311:
            f.write("2026-11-05T10:31:11Z POST /api/checkout status=503 user=u-4242 "
                    "request_id=req-7f3a9c0d1e2b ms=30001\n")
