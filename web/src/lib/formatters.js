export function dateOffset(diff) {
  const value = new Date();
  value.setDate(value.getDate() + diff);
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, "0");
  const day = String(value.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

export function formatNumber(value) {
  return Number(value || 0).toLocaleString();
}

export function formatPercent(value) {
  return `${(Number(value || 0) * 100).toFixed(1)}%`;
}

export function formatMoney(value, currency = "") {
  const normalized = String(currency || "").trim().toUpperCase();
  if (!normalized || normalized === "N/A") {
    return Number(value || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  }
  try {
    return new Intl.NumberFormat(undefined, {
      style: "currency",
      currency: normalized,
      minimumFractionDigits: 2,
      maximumFractionDigits: 2,
    }).format(Number(value || 0));
  } catch {
    const amount = Number(value || 0).toLocaleString(undefined, { minimumFractionDigits: 2, maximumFractionDigits: 2 });
    return `${normalized} ${amount}`;
  }
}

export function formatDurationSeconds(value) {
  const total = Math.max(0, Math.round(Number(value || 0)));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  if (!minutes) {
    return `${seconds}s`;
  }
  return `${minutes}m ${String(seconds).padStart(2, "0")}s`;
}
