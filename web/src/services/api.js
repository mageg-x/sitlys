export async function apiRequest(url, options = {}, fallbackMessage = "Request failed") {
  const config = {
    method: options.method || "GET",
    credentials: "include",
    headers: {},
  };
  if (options.body) {
    config.headers["Content-Type"] = "application/json";
    config.body = JSON.stringify(options.body);
  }

  const response = await fetch(url, config);
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || data.message || fallbackMessage);
  }
  return data;
}
