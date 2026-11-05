from shop import cart


def daily(orders):
    # each order is a list of (price, qty); calc_total adds VAT
    return sum(cart.calc_total(o) for o in orders)
