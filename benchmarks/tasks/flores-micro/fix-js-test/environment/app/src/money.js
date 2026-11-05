// Format an amount of cents as a string with two decimals: 1999 -> "19.99".
export function formatCents(cents) {
  const sign = cents < 0 ? "-" : "";
  const abs = Math.abs(cents);
  const euros = Math.floor(abs / 100);
  const rest = abs % 100;
  return `${sign}${euros}.${rest}`;
}
