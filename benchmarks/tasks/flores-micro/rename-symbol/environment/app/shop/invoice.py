from shop.cart import calc_total


def render(items):
    return f"TOTAL {calc_total(items):.2f}"
