import hashlib, re, sys
sys.path.insert(0, "/app")
text = open("/app/rates.py").read()
import rates
ok = rates.shipping_cost(1) == 10 and rates.shipping_cost(4) == 18 and rates.shipping_cost(10) == 45
# Everything outside shipping_cost must be byte-identical.
rest = re.sub('def shipping_cost\\(.*?(?=\\ndef |\\Z)', "", text, flags=re.S)
same = hashlib.sha256(rest.encode()).hexdigest() == "a3eb37bfc6575161811c59d3b11793ea7b0d03de891db921ddbdd16ffaa6865f"
print("behaviour ok:", ok, "rest unchanged:", same)
sys.exit(0 if ok and same else 1)
