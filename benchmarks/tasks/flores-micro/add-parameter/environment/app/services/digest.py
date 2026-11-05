from notify import send as notify_user


def weekly(user, items):
    # billing summary, goes by email like the rest of billing
    notify_user(user, f"{len(items)} invoices this week")
