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

export function formatMoney(value, currency = "") {
  const normalized = String(currency || "").trim().toUpperCase();
  const amount = Number(value || 0).toFixed(2);
  if (!normalized || normalized === "N/A") {
    return amount;
  }
  return `${normalized} ${amount}`;
}
