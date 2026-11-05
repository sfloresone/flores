from notify import send


def invoice_ready(user, number):
    send(user, f"Invoice {number} is ready")


def payment_failed(user):
    send(user, "Your payment failed")
