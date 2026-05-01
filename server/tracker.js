(function () {
  var script = document.currentScript;
  if (!script) return;

  var website = script.getAttribute("data-website-id");
  if (!website) return;

  var origin = new URL(script.src, window.location.href).origin;
  var storageKey = "sitlys.visitor." + website;
  var visitorId = localStorage.getItem(storageKey);
  if (!visitorId) {
    visitorId = (crypto && crypto.randomUUID ? crypto.randomUUID() : Math.random().toString(16).slice(2) + Date.now().toString(16)).replace(/-/g, "");
    localStorage.setItem(storageKey, visitorId);
  }

  function collect(type, payload) {
    var body = JSON.stringify({ type: type, payload: payload });
    if (navigator.sendBeacon) {
      var blob = new Blob([body], { type: "application/json" });
      if (navigator.sendBeacon(origin + "/api/send", blob)) {
        return;
      }
    }
    fetch(origin + "/api/send", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: body,
      keepalive: true
    }).catch(function () {});
  }

  function basePayload(extra) {
    var payload = {
      website: website,
      url: window.location.href,
      hostname: window.location.hostname,
      title: document.title,
      referrer: document.referrer || "",
      language: navigator.language || "",
      screen: window.screen ? window.screen.width + "x" + window.screen.height : "",
      id: visitorId,
      timestamp: Date.now()
    };
    return Object.assign(payload, extra || {});
  }

  window.sitlys = {
    track: function (name, data) {
      collect("event", basePayload({ name: name, data: data || {} }));
    },
    revenue: function (name, amount, currency, data) {
      collect("revenue", basePayload({
        name: name,
        data: data || {},
        revenue: { amount: Number(amount || 0), currency: currency || "USD" }
      }));
    }
  };

  var lastURL = window.location.href;

  function trackPageviewIfChanged() {
    var nextURL = window.location.href;
    if (nextURL === lastURL) return;
    lastURL = nextURL;
    collect("pageview", basePayload());
  }

  collect("pageview", basePayload());

  window.addEventListener("hashchange", trackPageviewIfChanged);
  window.addEventListener("popstate", trackPageviewIfChanged);

  var originalPushState = window.history && window.history.pushState;
  if (originalPushState) {
    window.history.pushState = function () {
      originalPushState.apply(window.history, arguments);
      trackPageviewIfChanged();
    };
  }

  var originalReplaceState = window.history && window.history.replaceState;
  if (originalReplaceState) {
    window.history.replaceState = function () {
      originalReplaceState.apply(window.history, arguments);
      trackPageviewIfChanged();
    };
  }
})();
