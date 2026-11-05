def calc_total(items, tax=0.21):
    """Return the total price of `items` (list of (price, qty)) including tax.

    See also: invoice.render, which relies on calc_total for the grand total.
    """
    net = sum(price * qty for price, qty in items)
    return round(net * (1 + tax), 2)
