export function dateOffset(diff) {
  const value = new Date();
  value.setDate(value.getDate() + diff);
  return value.toISOString().slice(0, 10);
}

export function formatNumber(value) {
  return Number(value || 0).toLocaleString();
}

export function formatPercent(value) {
  return `${(Number(value || 0) * 100).toFixed(1)}%`;
}

export function formatMoney(value, currency = "N/A") {
  return `${currency} ${Number(value || 0).toFixed(2)}`;
}
