import sys
sys.path.insert(0, "/app")
import notify
from services import alerts, billing, digest
try:
    notify.send("x", "y")
    required = False
except TypeError:
    required = True
try:
    notify.send("x", "y", "email")
    kwonly = False
except TypeError:
    kwonly = True
notify.SENT.clear()
billing.invoice_ready("ana", 7); billing.payment_failed("ana"); digest.weekly("ana", [1, 2])
alerts.disk_full("bob", "h1"); alerts.service_down("bob", "db")
chans = [s.get("channel") for s in notify.SENT]
ok = required and kwonly and chans == ["email", "email", "email", "sms", "sms"]
print(required, kwonly, chans)
sys.exit(0 if ok else 1)
