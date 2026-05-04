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
  var lastPageviewAt = 0;
  var leaveSentForURL = "";
  var heartbeatId = 0;
  var lastHeartbeatAt = 0;
  var HEARTBEAT_MS = 30000;

  function resetHeartbeat() {
    if (heartbeatId) {
      window.clearInterval(heartbeatId);
      heartbeatId = 0;
    }
    lastHeartbeatAt = Date.now();
    heartbeatId = window.setInterval(function () {
      if (document.visibilityState === "hidden" || !lastPageviewAt) return;
      var now = Date.now();
      var delta = Math.max(0, now - lastHeartbeatAt);
      if (!delta) return;
      lastHeartbeatAt = now;
      collect("event", basePayload({
        url: lastURL,
        hostname: new URL(lastURL, window.location.href).hostname,
        name: "page_ping",
        data: {
          duration_ms: delta
        }
      }));
    }, HEARTBEAT_MS);
  }

  function trackPageLeave(forceURL) {
    var currentURL = forceURL || lastURL || window.location.href;
    if (!lastPageviewAt || leaveSentForURL === currentURL) return;
    leaveSentForURL = currentURL;
    if (heartbeatId) {
      window.clearInterval(heartbeatId);
      heartbeatId = 0;
    }
    collect("event", basePayload({
      url: currentURL,
      hostname: new URL(currentURL, window.location.href).hostname,
      name: "page_leave",
      data: {
        duration_ms: Math.max(0, Date.now() - lastPageviewAt)
      }
    }));
  }

  function shouldTrackPageview() {
    if (document.visibilityState === "hidden") return false;
    if (navigator.doNotTrack === "1" || window.doNotTrack === "1" || navigator.msDoNotTrack === "1") {
      return false;
    }
    return true;
  }

  function trackPageviewIfChanged() {
    var nextURL = window.location.href;
    if (nextURL === lastURL) return;
    trackPageLeave(lastURL);
    lastURL = nextURL;
    if (!shouldTrackPageview()) return;
    lastPageviewAt = Date.now();
    lastHeartbeatAt = lastPageviewAt;
    leaveSentForURL = "";
    resetHeartbeat();
    collect("pageview", basePayload());
  }

  if (shouldTrackPageview()) {
    lastPageviewAt = Date.now();
    lastHeartbeatAt = lastPageviewAt;
    leaveSentForURL = "";
    resetHeartbeat();
    collect("pageview", basePayload());
  }

  window.addEventListener("hashchange", trackPageviewIfChanged);
  window.addEventListener("popstate", trackPageviewIfChanged);

  var originalPushState = window.history && window.history.pushState;
  if (originalPushState) {
    window.history.pushState = function () {
      originalPushState.apply(window.history, arguments);
      trackPageviewIfChanged();
    };
  }

  window.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "hidden") {
      trackPageLeave();
      return;
    }
    if (document.visibilityState === "visible") {
      trackPageviewIfChanged();
    }
  });

  window.addEventListener("pagehide", function () {
    trackPageLeave();
  });

  window.addEventListener("beforeunload", function () {
    trackPageLeave();
  });
})();
