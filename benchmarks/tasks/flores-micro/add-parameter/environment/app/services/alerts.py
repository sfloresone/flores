import notify


def disk_full(user, host):
    notify.send(user, f"Disk full on {host}")


def service_down(user, name):
    notify.send(user, message=f"{name} is down")
