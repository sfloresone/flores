import sys
original = '# Deployment for the shop stack.\n# Owner: platform team. Reviewed 2026-10.\nservices:\n  api:\n    image: shop/api:1.8.2\n    # scaled for the autumn campaign\n    replicas: 2\n    env:\n      PORT: "8080"\n  worker:\n    image: shop/worker:1.8.2\n    replicas: 1  # one is enough, jobs are idempotent\n    env:\n      QUEUE: jobs\n# end of file\n'
got = open("/app/deploy.yaml").read()
norm = lambda s: [l.rstrip().replace('"', "").replace("'", "") for l in s.splitlines() if l.strip()]
g = norm(got)
# every original line except api replicas must still be there, in order
keep = [l for l in norm(original) if l.strip() != "replicas: 2"]
it = iter(g)
in_order = all(any(x == want for x in it) for want in keep)
api = g[g.index("  api:"): g.index("  worker:")]
worker = g[g.index("  worker:"):]
ok = in_order and "    replicas: 5" in api and any(l.strip() == "LOG_LEVEL: debug" for l in worker)
ok = ok and not any("LOG_LEVEL" in l for l in api)
print(in_order, api, worker)
sys.exit(0 if ok else 1)
