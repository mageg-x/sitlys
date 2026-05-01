import { computed, inject, provide, proxyRefs, reactive } from "vue";
import { defaultFunnelSteps } from "../lib/defaults";
import { dateOffset, formatMoney, formatNumber, formatPercent } from "../lib/formatters";
import { VALID_ROUTES } from "../lib/routes";
import { apiRequest } from "../services/api";
import { createAdminActions } from "./use-admin-actions";
import { createAnalyticsActions } from "./use-analytics-actions";
import { createAuthActions } from "./use-auth-actions";
import { createRouteActions } from "./use-route-actions";

const AppControllerKey = Symbol("sitlys-app-controller");

export function provideAppController(controller) {
  provide(AppControllerKey, controller);
}

export function useAppController() {
  const controller = inject(AppControllerKey, null);
  if (!controller) {
    throw new Error("sitlys app controller not found");
  }
  return controller;
}

export function createAppController({ t, localeRef }) {
  const validRoutes = new Set(VALID_ROUTES);
  const state = reactive({
    mode: "loading",
    initialized: false,
    version: "",
    submitting: false,
    loading: false,
    notice: "",
    error: "",
    route: "overview",
    from: dateOffset(-29),
    to: dateOffset(0),
    me: null,
    websites: [],
    websiteId: "",
    overview: { pageviews: 0, visitors: 0, sessions: 0, events: 0, revenue: 0 },
    overviewCompare: null,
    overviewTrend: [],
    pages: [],
    pageEntries: [],
    pageExits: [],
    events: [],
    eventTypes: [],
    referrers: [],
    devices: { browsers: [], os: [], devices: [], matrix: [] },
    geo: [],
    geoRegions: [],
    geoCities: [],
    attribution: [],
    retention: [],
    revenue: [],
    funnels: [],
    funnelReport: null,
    selectedFunnelId: "",
    pixels: [],
    shares: [],
    users: [],
    publicShare: null,
    settings: null,
    botAudit: {},
    backupPath: "",
    cleanupResult: null,
    realtime: null,
  });

  const initForm = reactive({ username: "", password: "", confirmPassword: "" });
  const loginForm = reactive({ username: "", password: "" });
  const websiteForm = reactive({ id: "", name: "", domain: "" });
  const pixelForm = reactive({ name: "" });
  const funnelForm = reactive({
    name: "",
    steps: defaultFunnelSteps(),
  });
  const userForm = reactive({
    id: "",
    username: "",
    password: "",
    role: "viewer",
    enabled: true,
    permissions: [],
  });
  const passwordForm = reactive({
    currentPassword: "",
    newPassword: "",
    confirmPassword: "",
  });
  const settingsForm = reactive({
    listen_addr: "",
    database_path: "",
    log_level: "info",
    data_retention_days: 365,
    bot_filter_mode: "balanced",
  });
  const pageFilter = reactive({ query: "" });
  const eventFilter = reactive({ query: "" });
  const referrerFilter = reactive({ query: "" });
  const revenueFilter = reactive({ query: "" });
  const attributionFilter = reactive({ query: "" });

  const locale = computed({
    get: () => localeRef.value,
    set: value => {
      localeRef.value = value;
      localStorage.setItem("sitlys.locale", value);
    },
  });

  const origin = window.location.origin;
  const selectedWebsite = computed(() => state.websites.find(item => item.id === state.websiteId) || null);
  const editingWebsite = computed(() => state.websites.find(item => item.id === websiteForm.id) || selectedWebsite.value || null);
  const isSuperAdmin = computed(() => state.me?.role === "super_admin");
  const canCreateWebsite = computed(() => state.me?.role === "super_admin" || state.me?.role === "admin");
  const canManageSelectedWebsite = computed(() => canManageWebsite(state.websiteId));
  const canReviewWebsiteMembers = computed(() => isSuperAdmin.value);
  const analyticsRoutes = new Set(["overview", "pages", "events", "referrers", "geo", "devices", "attribution", "funnels", "retention", "revenue"]);
  const managementRoutes = new Set(["pixels", "shares", "websites", "users"]);
  const isAnalyticsRoute = computed(() => analyticsRoutes.has(state.route));
  const isManagementRoute = computed(() => managementRoutes.has(state.route));

  const navItems = computed(() => {
    const items = [
      { key: "overview", label: t("overview"), note: t("analyticsDeck") },
      { key: "pages", label: t("pages"), note: t("analyticsDeck") },
      { key: "events", label: t("events"), note: t("analyticsDeck") },
      { key: "referrers", label: t("referrers"), note: t("analyticsDeck") },
      { key: "geo", label: t("geo"), note: t("analyticsDeck") },
      { key: "devices", label: t("devices"), note: t("analyticsDeck") },
      { key: "attribution", label: t("attribution"), note: t("analyticsDeck") },
      { key: "funnels", label: t("funnels"), note: t("analyticsDeck") },
      { key: "retention", label: t("retention"), note: t("analyticsDeck") },
      { key: "revenue", label: t("revenue"), note: t("analyticsDeck") },
      { key: "pixels", label: t("pixels"), note: t("managementDeck") },
      { key: "shares", label: t("shares"), note: t("managementDeck") },
      { key: "websites", label: t("websites"), note: t("managementDeck") },
      { key: "settings", label: t("settings"), note: t("managementDeck") },
    ];
    if (isSuperAdmin.value) {
      items.splice(items.length - 1, 0, { key: "users", label: t("users"), note: t("managementDeck") });
    }
    return items;
  });

  const routeMeta = computed(() => {
    const map = {
      overview: { title: t("overview"), note: t("quickInsights"), description: t("consoleHint") },
      pages: { title: t("pages"), note: t("analyticsDeck"), description: t("topPages") },
      events: { title: t("events"), note: t("analyticsDeck"), description: t("eventCount") },
      referrers: { title: t("referrers"), note: t("analyticsDeck"), description: t("topSources") },
      geo: { title: t("geo"), note: t("analyticsDeck"), description: t("regions") },
      devices: { title: t("devices"), note: t("analyticsDeck"), description: t("environment") },
      attribution: { title: t("attribution"), note: t("analyticsDeck"), description: t("attributionText") },
      funnels: { title: t("funnels"), note: t("analyticsDeck"), description: t("funnelText") },
      retention: { title: t("retention"), note: t("analyticsDeck"), description: t("retentionText") },
      revenue: { title: t("revenue"), note: t("analyticsDeck"), description: t("revenueText") },
      pixels: { title: t("pixels"), note: t("managementDeck"), description: t("pixelSnippet") },
      shares: { title: t("shares"), note: t("managementDeck"), description: t("publicShare") },
      websites: { title: t("websites"), note: t("managementDeck"), description: t("websiteListText") },
      users: { title: t("users"), note: t("managementDeck"), description: t("permissionText") },
      settings: { title: t("settings"), note: t("managementDeck"), description: t("settingsText") },
    };
    return map[state.route] || map.overview;
  });

  const overviewCards = computed(() => [
    { key: "pageviews", label: t("pageviews"), value: formatNumber(state.overview.pageviews), compare: state.overviewCompare?.metrics?.pageviews || null },
    { key: "visitors", label: t("visitors"), value: formatNumber(state.overview.visitors), compare: state.overviewCompare?.metrics?.visitors || null },
    { key: "sessions", label: t("sessions"), value: formatNumber(state.overview.sessions), compare: state.overviewCompare?.metrics?.sessions || null },
    { key: "events", label: t("totalEvents"), value: formatNumber(state.overview.events), compare: state.overviewCompare?.metrics?.events || null },
    { key: "revenue", label: t("revenueTotal"), value: formatMoney(state.overview.revenue), compare: state.overviewCompare?.metrics?.revenue || null },
  ]);

  const compareSummary = computed(() => {
    if (!state.overviewCompare?.metrics) {
      return [];
    }
    return [
      { key: "pageviews", label: t("pageviews"), metric: state.overviewCompare.metrics.pageviews },
      { key: "visitors", label: t("visitors"), metric: state.overviewCompare.metrics.visitors },
      { key: "sessions", label: t("sessions"), metric: state.overviewCompare.metrics.sessions },
      { key: "events", label: t("totalEvents"), metric: state.overviewCompare.metrics.events },
      { key: "revenue", label: t("revenueTotal"), metric: state.overviewCompare.metrics.revenue },
    ];
  });

  const realtimeHighlights = computed(() => {
    const realtime = state.realtime || {};
    return [
      { key: "visitors", label: t("activeVisitors"), value: formatNumber(realtime.active_visitors || 0) },
      { key: "sessions", label: t("activeSessions"), value: formatNumber(realtime.active_sessions || 0) },
      { key: "window", label: t("realtimeWindow"), value: `${realtime.window_minutes || 5}m` },
    ];
  });

  const shareMetrics = computed(() => {
    const overview = state.publicShare?.overview || {};
    return [
      { key: "pageviews", label: t("pageviews"), value: formatNumber(overview.pageviews || 0) },
      { key: "visitors", label: t("visitors"), value: formatNumber(overview.visitors || 0) },
      { key: "sessions", label: t("sessions"), value: formatNumber(overview.sessions || 0) },
      { key: "events", label: t("totalEvents"), value: formatNumber(overview.events || 0) },
      { key: "revenue", label: t("revenueTotal"), value: formatMoney(overview.revenue || 0) },
    ];
  });

  const roleCards = computed(() => [
    { key: "super_admin", label: t("superAdmin"), text: t("roleSuperAdminText") },
    { key: "admin", label: t("admin"), text: t("roleAdminText") },
    { key: "analyst", label: t("analyst"), text: t("roleAnalystText") },
    { key: "viewer", label: t("viewer"), text: t("roleViewerText") },
  ]);

  const userPermissionSummary = computed(() => {
    const manage = userForm.permissions.filter(item => item.access_level === "manage").length;
    const view = userForm.permissions.filter(item => item.access_level === "view").length;
    const assigned = manage + view;
    return {
      assigned,
      manage,
      view,
      none: Math.max(state.websites.length - assigned, 0),
    };
  });

  const websiteMembers = computed(() => {
    if (!editingWebsite.value || !state.users.length) {
      return [];
    }
    const ranking = { super_admin: 0, admin: 1, analyst: 2, viewer: 3 };
    return state.users
      .map(user => {
        const permission = (user.permissions || []).find(item => item.website_id === editingWebsite.value.id);
        const accessLevel = user.role === "super_admin" ? "manage" : (permission?.access_level || "none");
        return {
          id: user.id,
          username: user.username,
          role: user.role,
          enabled: Boolean(user.enabled),
          accessLevel,
        };
      })
      .filter(user => user.accessLevel !== "none")
      .sort((left, right) => {
        if (left.accessLevel !== right.accessLevel) {
          return left.accessLevel === "manage" ? -1 : 1;
        }
        return (ranking[left.role] ?? 99) - (ranking[right.role] ?? 99) || left.username.localeCompare(right.username);
      });
  });

  const websiteMemberSummary = computed(() => ({
    total: websiteMembers.value.length,
    manage: websiteMembers.value.filter(user => user.accessLevel === "manage").length,
    view: websiteMembers.value.filter(user => user.accessLevel === "view").length,
  }));

  const topPage = computed(() => state.pages[0] || null);
  const topEvent = computed(() => state.events[0] || null);
  const topReferrer = computed(() => state.referrers[0] || null);
  const topCountry = computed(() => state.geo[0] || null);
  const topRegion = computed(() => state.geoRegions[0] || null);
  const topCity = computed(() => state.geoCities[0] || null);
  const topBrowser = computed(() => state.devices.browsers?.[0] || null);
  const topOS = computed(() => state.devices.os?.[0] || null);
  const topDeviceType = computed(() => state.devices.devices?.[0] || null);
  const eventTypeMix = computed(() => {
    const totals = new Map();
    for (const item of state.events) {
      const key = item.type || "custom";
      totals.set(key, (totals.get(key) || 0) + Number(item.events || 0));
    }
    return [...totals.entries()]
      .map(([type, events]) => ({ type, events }))
      .sort((left, right) => right.events - left.events);
  });
  const attributionTop = computed(() => state.attribution[0] || null);
  const filteredPages = computed(() => {
    const query = pageFilter.query.trim().toLowerCase();
    if (!query) {
      return state.pages;
    }
    return state.pages.filter(item => String(item.path || "").toLowerCase().includes(query));
  });
  const filteredEvents = computed(() => {
    const query = eventFilter.query.trim().toLowerCase();
    if (!query) {
      return state.events;
    }
    return state.events.filter(item => String(item.name || "").toLowerCase().includes(query) || String(item.type || "").toLowerCase().includes(query));
  });
  const filteredReferrers = computed(() => {
    const query = referrerFilter.query.trim().toLowerCase();
    if (!query) {
      return state.referrers;
    }
    return state.referrers.filter(item => String(item.referrer || "").toLowerCase().includes(query));
  });
  const filteredRevenue = computed(() => {
    const query = revenueFilter.query.trim().toLowerCase();
    if (!query) {
      return state.revenue;
    }
    return state.revenue.filter(item => String(item.source || "").toLowerCase().includes(query) || String(item.currency || "").toLowerCase().includes(query));
  });
  const filteredAttribution = computed(() => {
    const query = attributionFilter.query.trim().toLowerCase();
    if (!query) {
      return state.attribution;
    }
    return state.attribution.filter(item =>
      String(item.source || "").toLowerCase().includes(query) ||
      String(item.medium || "").toLowerCase().includes(query) ||
      String(item.campaign || "").toLowerCase().includes(query)
    );
  });
  const overviewTrendPeaks = computed(() => ({
    pageviews: Math.max(...state.overviewTrend.map(item => Number(item.pageviews || 0)), 1),
    events: Math.max(...state.overviewTrend.map(item => Number(item.events || 0)), 1),
    revenue: Math.max(...state.overviewTrend.map(item => Number(item.revenue || 0)), 1),
  }));

  const overviewHighlights = computed(() => {
    const pageviews = Number(state.overview.pageviews || 0);
    const sessions = Number(state.overview.sessions || 0);
    const events = Number(state.overview.events || 0);
    const topSourceVisits = Number(topReferrer.value?.visits || 0);
    const topCountryVisits = Number(topCountry.value?.visits || 0);
    return [
      {
        key: "sessions-per-pageview",
        label: t("sessionsPerPageview"),
        value: sessions ? Number(pageviews / sessions).toFixed(2) : "0.00",
        tone: "indigo",
      },
      {
        key: "events-per-session",
        label: t("avgEventsPerSession"),
        value: sessions ? Number(events / sessions).toFixed(2) : "0.00",
        tone: "teal",
      },
      {
        key: "source-share",
        label: t("topSourceShare"),
        value: sessions ? formatPercent(topSourceVisits / sessions) : "0.0%",
        tone: "amber",
      },
      {
        key: "country-share",
        label: t("topCountryShare"),
        value: sessions ? formatPercent(topCountryVisits / sessions) : "0.0%",
        tone: "rose",
      },
    ];
  });

  const pageHighlights = computed(() => {
    const top = topPage.value;
    const pageviews = Number(state.overview.pageviews || 0);
    const sessions = Number(state.overview.sessions || 0);
    return [
      {
        key: "focus-page",
        label: t("focusPages"),
        value: top?.path || "/",
        hint: top ? `${formatNumber(top.pageviews)} PV` : t("noData"),
      },
      {
        key: "content-entry",
        label: t("contentEntry"),
        value: top ? formatPercent(Number(top.pageviews || 0) / Math.max(pageviews, 1)) : "0.0%",
        hint: t("pageviews"),
      },
      {
        key: "session-depth",
        label: t("sessionDepth"),
        value: top ? formatPercent(Number(top.sessions || 0) / Math.max(sessions, 1)) : "0.0%",
        hint: t("sessions"),
      },
    ];
  });

  const eventHighlights = computed(() => {
    const top = topEvent.value;
    const events = Number(state.overview.events || 0);
    const sessions = Number(state.overview.sessions || 0);
    return [
      {
        key: "focus-event",
        label: t("focusEvents"),
        value: top?.name || t("unnamedEvent"),
        hint: top ? formatNumber(top.events) : t("noData"),
      },
      {
        key: "event-coverage",
        label: t("eventCoverage"),
        value: top ? formatPercent(Number(top.sessions || 0) / Math.max(sessions, 1)) : "0.0%",
        hint: t("sessions"),
      },
      {
        key: "revenue-signal",
        label: t("revenueSignals"),
        value: top ? formatMoney(top.revenue || 0) : formatMoney(0),
        hint: top ? formatPercent(Number(top.events || 0) / Math.max(events, 1)) : "0.0%",
      },
    ];
  });

  const referrerHighlights = computed(() => {
    const top = topReferrer.value;
    const sessions = Number(state.overview.sessions || 0);
    const revenue = Number(state.overview.revenue || 0);
    const attributedRevenue = Number(state.revenue.reduce((sum, row) => sum + Number(row.revenue || 0), 0));
    return [
      {
        key: "focus-source",
        label: t("focusSources"),
        value: top?.referrer || t("directTraffic"),
        hint: top ? formatNumber(top.visits) : t("noData"),
      },
      {
        key: "source-share",
        label: t("acquisitionHealth"),
        value: top ? formatPercent(Number(top.visits || 0) / Math.max(sessions, 1)) : "0.0%",
        hint: t("sessions"),
      },
      {
        key: "revenue-share",
        label: t("revenueSignals"),
        value: revenue ? formatPercent(attributedRevenue / Math.max(revenue, 1)) : "0.0%",
        hint: formatMoney(attributedRevenue),
      },
    ];
  });

  const geoHighlights = computed(() => {
    const top = topCountry.value;
    const sessions = Number(state.overview.sessions || 0);
    return [
      {
        key: "focus-region",
        label: t("focusRegions"),
        value: top?.country || t("unknownRegion"),
        hint: top ? formatNumber(top.visits) : t("noData"),
      },
      {
        key: "regional-share",
        label: t("regionalFocus"),
        value: top ? formatPercent(Number(top.visits || 0) / Math.max(sessions, 1)) : "0.0%",
        hint: t("visits"),
      },
      {
        key: "geo-coverage",
        label: t("geoCoverage"),
        value: formatNumber(state.geo.length),
        hint: t("topCountries"),
      },
    ];
  });

  const deviceHighlights = computed(() => {
    const sessions = Number(state.overview.sessions || 0);
    return [
      {
        key: "browser",
        label: t("topBrowsers"),
        value: topBrowser.value?.value || "-",
        hint: topBrowser.value ? formatPercent(Number(topBrowser.value.visits || 0) / Math.max(sessions, 1)) : t("noData"),
      },
      {
        key: "os",
        label: t("topOperatingSystems"),
        value: topOS.value?.value || "-",
        hint: topOS.value ? formatNumber(topOS.value.visits) : t("noData"),
      },
      {
        key: "device",
        label: t("topDevices"),
        value: topDeviceType.value?.value || "-",
        hint: topDeviceType.value ? formatNumber(topDeviceType.value.visits) : t("noData"),
      },
    ];
  });

  const attributionHighlights = computed(() => {
    const top = attributionTop.value;
    const sessions = Number(state.overview.sessions || 0);
    const revenue = Number(state.overview.revenue || 0);
    return [
      {
        key: "source",
        label: t("focusSources"),
        value: top?.source || t("directTraffic"),
        hint: top ? formatNumber(top.sessions) : t("noData"),
      },
      {
        key: "medium",
        label: t("sourceMediums"),
        value: top?.medium || "-",
        hint: top ? formatPercent(Number(top.sessions || 0) / Math.max(sessions, 1)) : "0.0%",
      },
      {
        key: "campaign",
        label: t("topCampaigns"),
        value: top?.campaign || "-",
        hint: revenue ? formatPercent(Number(top?.revenue || 0) / Math.max(revenue, 1)) : "0.0%",
      },
    ];
  });

  const revenueHighlights = computed(() => {
    const top = state.revenue[0] || null;
    const totalRevenue = Number(state.overview.revenue || 0);
    return [
      {
        key: "source",
        label: t("focusSources"),
        value: top?.source || t("directTraffic"),
        hint: top ? formatMoney(top.revenue || 0, top.currency) : t("noData"),
      },
      {
        key: "currency",
        label: t("topCurrencies"),
        value: top?.currency || "-",
        hint: top ? formatNumber(top.events || 0) : t("noData"),
      },
      {
        key: "mix",
        label: t("revenueMix"),
        value: top ? formatPercent(Number(top.revenue || 0) / Math.max(totalRevenue, 1)) : "0.0%",
        hint: t("revenueTotal"),
      },
    ];
  });

  const retentionHighlights = computed(() => {
    const top = state.retention[0] || null;
    return [
      {
        key: "cohort",
        label: t("cohort"),
        value: top?.cohort || "-",
        hint: top ? formatNumber(top.size || 0) : t("noData"),
      },
      {
        key: "day1",
        label: t("day1"),
        value: top ? formatPercent(top.day_1) : "0.0%",
        hint: t("cohortHealth"),
      },
      {
        key: "day7",
        label: t("day7"),
        value: top ? formatPercent(top.day_7) : "0.0%",
        hint: t("retention"),
      },
    ];
  });

  const funnelHighlights = computed(() => {
    const steps = state.funnelReport?.steps || [];
    const strongest = steps.reduce((best, step) => {
      if (!best || Number(step.conversion || 0) > Number(best.conversion || 0)) {
        return step;
      }
      return best;
    }, null);
    const last = steps[steps.length - 1] || null;
    return [
      {
        key: "strongest",
        label: t("strongestStep"),
        value: strongest?.label || "-",
        hint: strongest ? formatPercent(strongest.conversion) : t("noData"),
      },
      {
        key: "dropoff",
        label: t("dropOff"),
        value: last ? formatPercent(1 - Number(last.conversion || 0)) : "0.0%",
        hint: last ? formatNumber(last.sessions || 0) : t("noData"),
      },
      {
        key: "momentum",
        label: t("funnelMomentum"),
        value: steps.length ? formatNumber(steps.length) : "0",
        hint: t("runReport"),
      },
    ];
  });

  const trackerSnippet = computed(() => {
    if (!state.websiteId) return t("websiteRequired");
    return `<script async data-website-id="${state.websiteId}" src="${origin}/tracker.js"></script>`;
  });

  const firstPixelSnippet = computed(() => {
    if (!state.pixels.length) return t("noData");
    return `${origin}/collect/p/${state.pixels[0].slug}`;
  });

  const firstShareLink = computed(() => {
    if (!state.shares.length) return t("noData");
    return `${origin}/share/${state.shares[0].slug}`;
  });

  const pageColumns = computed(() => [
    { key: "path", label: t("path") },
    { key: "pageviews", label: t("pageviews"), format: row => formatNumber(row.pageviews) },
    { key: "sessions", label: t("sessions"), format: row => formatNumber(row.sessions) },
  ]);

  const entryColumns = computed(() => [
    { key: "path", label: t("path") },
    { key: "sessions", label: t("sessions"), format: row => formatNumber(row.sessions) },
  ]);

  const eventColumns = computed(() => [
    { key: "name", label: t("eventName") },
    { key: "type", label: t("eventType") },
    { key: "events", label: t("eventCount"), format: row => formatNumber(row.events) },
    { key: "sessions", label: t("sessionCount"), format: row => formatNumber(row.sessions) },
    { key: "revenue", label: t("revenueTotal"), format: row => formatMoney(row.revenue) },
  ]);

  const referrerColumns = computed(() => [
    { key: "referrer", label: t("referrer") },
    { key: "visits", label: t("visits"), format: row => formatNumber(row.visits) },
    { key: "revenue", label: t("revenueTotal"), format: row => formatMoney(row.revenue || 0) },
  ]);

  const geoColumns = computed(() => [
    { key: "country", label: t("country") },
    { key: "visits", label: t("visits"), format: row => formatNumber(row.visits) },
  ]);

  const regionColumns = computed(() => [
    { key: "region", label: t("regions") },
    { key: "visits", label: t("visits"), format: row => formatNumber(row.visits) },
  ]);

  const cityColumns = computed(() => [
    { key: "city", label: t("city") },
    { key: "visits", label: t("visits"), format: row => formatNumber(row.visits) },
  ]);

  const deviceMatrixColumns = computed(() => [
    { key: "browser", label: t("browser") },
    { key: "os", label: t("os") },
    { key: "device", label: t("device") },
    { key: "visits", label: t("visits"), format: row => formatNumber(row.visits) },
  ]);

  const attributionColumns = computed(() => [
    { key: "source", label: t("source") },
    { key: "medium", label: t("medium") },
    { key: "campaign", label: t("campaign") },
    { key: "sessions", label: t("sessions"), format: row => formatNumber(row.sessions) },
    { key: "revenue", label: t("revenueTotal"), format: row => formatMoney(row.revenue) },
  ]);

  const retentionColumns = computed(() => [
    { key: "cohort", label: t("cohort") },
    { key: "size", label: t("size"), format: row => formatNumber(row.size) },
    { key: "day_1", label: t("day1"), format: row => formatPercent(row.day_1) },
    { key: "day_7", label: t("day7"), format: row => formatPercent(row.day_7) },
    { key: "day_30", label: t("day30"), format: row => formatPercent(row.day_30) },
  ]);

  const revenueColumns = computed(() => [
    { key: "source", label: t("source") },
    { key: "currency", label: t("currency") },
    { key: "events", label: t("eventCount"), format: row => formatNumber(row.events) },
    { key: "revenue", label: t("revenueTotal"), format: row => formatMoney(row.revenue, row.currency) },
  ]);

  const sharePageColumns = computed(() => [
    { key: "label", label: t("path") },
    { key: "count", label: t("pageviews"), format: row => formatNumber(row.count) },
  ]);

  const shareReferrerColumns = computed(() => [
    { key: "label", label: t("referrer") },
    { key: "count", label: t("visits"), format: row => formatNumber(row.count) },
  ]);

  const shareAttributionColumns = computed(() => [
    { key: "source", label: t("source") },
    { key: "medium", label: t("medium") },
    { key: "campaign", label: t("campaign") },
    { key: "sessions", label: t("sessions"), format: row => formatNumber(row.sessions) },
    { key: "revenue", label: t("revenueTotal"), format: row => formatMoney(row.revenue) },
  ]);

  const shareRevenueColumns = computed(() => [
    { key: "source", label: t("source") },
    { key: "currency", label: t("currency") },
    { key: "revenue", label: t("revenueTotal"), format: row => formatMoney(row.revenue, row.currency) },
  ]);

  function canManageWebsite(websiteId) {
    if (!websiteId || !state.me) return false;
    if (state.me.role === "super_admin") return true;
    return (state.me.permissions || []).some(item => item.website_id === websiteId && item.access_level === "manage");
  }

  function roleLabel(role) {
    switch (role) {
      case "super_admin":
        return t("superAdmin");
      case "admin":
        return t("admin");
      case "analyst":
        return t("analyst");
      case "viewer":
        return t("viewer");
      default:
        return role || "-";
    }
  }

  function accessLevelLabel(level) {
    switch (level) {
      case "manage":
        return t("canManage");
      case "view":
        return t("canView");
      default:
        return t("noAccess");
    }
  }

  function clearFeedback() {
    state.notice = "";
    state.error = "";
  }

  async function request(url, options = {}) {
    return apiRequest(url, options, t("requestFailed"));
  }

  async function safeRequest(url, options = {}) {
    try {
      return await request(url, options);
    } catch (error) {
      if (!options.swallow) {
        state.error = error.message || t("loadFailed");
      }
      return null;
    }
  }

  async function runSubmit(fn) {
    state.submitting = true;
    clearFeedback();
    try {
      await fn();
    } catch (error) {
      state.error = error.message || t("requestFailed");
    } finally {
      state.submitting = false;
    }
  }

  let adminActions;
  let analyticsActions;
  let routeActions;

  adminActions = createAdminActions({
    t,
    state,
    isSuperAdmin,
    websiteForm,
    pixelForm,
    funnelForm,
    userForm,
    settingsForm,
    runSubmit,
    request,
    safeRequest,
    refreshActive: () => analyticsActions.refreshActive(),
    loadFunnels: () => analyticsActions.loadFunnels(),
  });

  routeActions = createRouteActions({
    state,
    isSuperAdmin,
    validRoutes,
    loadRouteData: () => analyticsActions.loadRouteData(),
    refreshActive: () => analyticsActions.refreshActive(),
    loadRealtime: () => analyticsActions.loadRealtime(),
    loadWebsites: () => adminActions.loadWebsites(),
    editUser: user => adminActions.editUser(user),
    userForm,
    syncHashFromRoute: () => routeActions.syncHashFromRoute(),
    clearFeedback,
    loadUsers: () => adminActions.loadUsers(),
    loadSettings: () => adminActions.loadSettings(),
  });

  analyticsActions = createAnalyticsActions({
    t,
    state,
    isSuperAdmin,
    safeRequest,
    request,
    runSubmit,
    loadPixels: () => adminActions.loadPixels(),
    loadShares: () => adminActions.loadShares(),
    loadWebsites: () => adminActions.loadWebsites(),
    loadUsers: () => adminActions.loadUsers(),
    loadSettings: () => adminActions.loadSettings(),
    syncHashFromRoute: () => routeActions.syncHashFromRoute(),
  });

  const authActions = createAuthActions({
    t,
    state,
    initForm,
    loginForm,
    passwordForm,
    localeRef,
    runSubmit,
    request,
    safeRequest,
    syncRouteFromHash: () => routeActions.syncRouteFromHash(),
    hydrateWorkspace: () => routeActions.hydrateWorkspace(),
    loadPublicShare: () => analyticsActions.loadPublicShare(),
    clearFeedback,
  });

  return proxyRefs({
    t,
    locale,
    origin,
    state,
    initForm,
    loginForm,
    websiteForm,
    pixelForm,
    funnelForm,
    userForm,
    passwordForm,
    settingsForm,
    pageFilter,
    eventFilter,
    referrerFilter,
    revenueFilter,
    attributionFilter,
    selectedWebsite,
    editingWebsite,
    isSuperAdmin,
    canCreateWebsite,
    canManageSelectedWebsite,
    canReviewWebsiteMembers,
    isAnalyticsRoute,
    isManagementRoute,
    navItems,
    routeMeta,
    overviewCards,
    compareSummary,
    shareMetrics,
    roleCards,
    userPermissionSummary,
    websiteMembers,
    websiteMemberSummary,
    topPage,
    topEvent,
    topReferrer,
    topCountry,
    topRegion,
    topCity,
    topBrowser,
    topOS,
    topDeviceType,
    eventTypeMix,
    attributionTop,
    filteredPages,
    filteredEvents,
    filteredReferrers,
    filteredRevenue,
    filteredAttribution,
    overviewTrendPeaks,
    realtimeHighlights,
    overviewHighlights,
    pageHighlights,
    eventHighlights,
    referrerHighlights,
    geoHighlights,
    deviceHighlights,
    attributionHighlights,
    revenueHighlights,
    retentionHighlights,
    funnelHighlights,
    trackerSnippet,
    firstPixelSnippet,
    firstShareLink,
    pageColumns,
    entryColumns,
    eventColumns,
    referrerColumns,
    geoColumns,
    regionColumns,
    cityColumns,
    deviceMatrixColumns,
    attributionColumns,
    retentionColumns,
    revenueColumns,
    sharePageColumns,
    shareReferrerColumns,
    shareAttributionColumns,
    shareRevenueColumns,
    bootstrap: authActions.bootstrap,
    submitInit: authActions.submitInit,
    submitLogin: authActions.submitLogin,
    submitLogout: authActions.submitLogout,
    changePassword: authActions.changePassword,
    handleWebsiteChange: routeActions.handleWebsiteChange,
    applyRange: routeActions.applyRange,
    refreshActive: analyticsActions.refreshActive,
    selectRoute: routeActions.selectRoute,
    loadPublicShare: analyticsActions.loadPublicShare,
    saveWebsite: adminActions.saveWebsite,
    deleteWebsite: adminActions.deleteWebsite,
    savePixel: adminActions.savePixel,
    togglePixel: adminActions.togglePixel,
    saveShare: adminActions.saveShare,
    toggleShare: adminActions.toggleShare,
    saveFunnel: adminActions.saveFunnel,
    runFunnel: analyticsActions.runFunnel,
    saveUser: adminActions.saveUser,
    saveSettings: adminActions.saveSettings,
    createBackup: adminActions.createBackup,
    runCleanup: adminActions.runCleanup,
    exportData: adminActions.exportData,
    editWebsite: adminActions.editWebsite,
    editUser: adminActions.editUser,
    resetWebsiteForm: adminActions.resetWebsiteForm,
    resetUserForm: adminActions.resetUserForm,
    addStep: adminActions.addStep,
    removeStep: adminActions.removeStep,
    hasPermission: adminActions.hasPermission,
    accessLevel: adminActions.accessLevel,
    setAccessLevel: adminActions.setAccessLevel,
    hasManage: adminActions.hasManage,
    setPermission: adminActions.setPermission,
    setManage: adminActions.setManage,
    canManageWebsite,
    roleLabel,
    accessLevelLabel,
    formatNumber,
    formatPercent,
    formatMoney,
    syncRouteFromHash: routeActions.syncRouteFromHash,
    handleHashChange: routeActions.handleHashChange,
  });
}
