Add a required keyword-only parameter `channel` to `notify.send` (stored in each
SENT entry under the key "channel"). Then update every call site in
/app/services: billing notifications (services/billing.py and services/digest.py)
use channel="email", alerts (services/alerts.py) use channel="sms".
