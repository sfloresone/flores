SENT = []


def send(user, message):
    """Queue a notification for `user`."""
    SENT.append({"user": user, "message": message})
