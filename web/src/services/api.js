const DEFAULT_TIMEOUT_MS = 10000;
const RETRYABLE_METHODS = new Set(["GET", "HEAD"]);

function wait(ms) {
  return new Promise(resolve => window.setTimeout(resolve, ms));
}

function isRetryableError(error) {
  return error?.name === "AbortError" || /network/i.test(String(error?.message || ""));
}

export async function apiRequest(url, options = {}, fallbackMessage = "Request failed") {
  const method = String(options.method || "GET").toUpperCase();
  const timeoutMs = Number(options.timeout_ms || DEFAULT_TIMEOUT_MS);
  const retries = Number(options.retries ?? (RETRYABLE_METHODS.has(method) ? 1 : 0));
  const config = {
    method,
    credentials: "include",
    headers: {},
  };
  if (options.body) {
    config.headers["Content-Type"] = "application/json";
    config.body = JSON.stringify(options.body);
  }
  let attempt = 0;
  while (true) {
    const controller = new AbortController();
    const timer = window.setTimeout(() => controller.abort(), timeoutMs);
    try {
      const response = await fetch(url, { ...config, signal: controller.signal });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) {
        throw new Error(data.error || data.message || fallbackMessage);
      }
      return data;
    } catch (error) {
      if (attempt < retries && isRetryableError(error)) {
        attempt += 1;
        await wait(300 * attempt);
        continue;
      }
      if (error?.name === "AbortError") {
        throw new Error("Request timeout");
      }
      throw error;
    } finally {
      window.clearTimeout(timer);
    }
  }
}
