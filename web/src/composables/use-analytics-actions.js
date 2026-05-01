import { WEBSITE_OPTIONAL_ROUTES } from "../lib/routes";

export function createAnalyticsActions(ctx) {
  const { state, isSuperAdmin, safeRequest, request, runSubmit } = ctx;

  function buildAnalyticsUrl(base) {
    return `${base}?website_id=${encodeURIComponent(state.websiteId)}&from=${state.from}&to=${state.to}`;
  }

  async function refreshActive() {
    if (!state.websiteId && state.mode === "app") {
      await ctx.loadWebsites();
      if (state.websites[0]?.id) {
        state.websiteId = state.websites[0].id;
      }
    }
    if (state.websiteId) {
      await loadOverview();
    }
    await loadRouteData();
  }

  async function loadRouteData() {
    if (state.route === "users" && !isSuperAdmin.value) {
      state.route = "overview";
      ctx.syncHashFromRoute();
    }
    if (!state.websiteId && !WEBSITE_OPTIONAL_ROUTES.includes(state.route)) {
      return;
    }
    const loaders = {
      overview: async () => {
        await Promise.all([loadPages(), loadReferrers(), loadDevices(), loadGeo()]);
      },
      pages: loadPages,
      events: loadEvents,
      referrers: loadReferrers,
      geo: loadGeo,
      devices: loadDevices,
      attribution: loadAttribution,
      funnels: async () => {
        await loadFunnels();
        if (state.funnels[0]?.id && !state.selectedFunnelId) {
          await runFunnel(state.funnels[0].id);
        }
      },
      retention: loadRetention,
      revenue: loadRevenue,
      pixels: ctx.loadPixels,
      shares: ctx.loadShares,
      websites: ctx.loadWebsites,
      users: ctx.loadUsers,
      settings: async () => {
        await Promise.all([ctx.loadPixels(), ctx.loadShares()]);
        if (isSuperAdmin.value) {
          await ctx.loadSettings();
        }
      },
    };
    const loader = loaders[state.route];
    if (loader) {
      await loader();
    }
  }

  async function loadOverview() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/overview"));
    if (data?.overview) {
      state.overview = data.overview;
    }
    state.overviewCompare = data?.compare || null;
    state.overviewTrend = data?.trend || [];
  }

  async function loadPages() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/pages"));
    state.pages = data?.items || [];
    state.pageEntries = data?.entries || [];
    state.pageExits = data?.exits || [];
  }

  async function loadEvents() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/events"));
    state.events = data?.items || [];
    state.eventTypes = data?.types || [];
  }

  async function loadReferrers() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/referrers"));
    state.referrers = data?.items || [];
  }

  async function loadDevices() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/devices"));
    state.devices = data?.items || { browsers: [], os: [], devices: [], matrix: [] };
  }

  async function loadGeo() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/geo"));
    state.geo = data?.items || [];
    state.geoRegions = data?.regions || [];
    state.geoCities = data?.cities || [];
  }

  async function loadAttribution() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/attribution"));
    state.attribution = data?.items || [];
  }

  async function loadRetention() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/retention"));
    state.retention = data?.items || [];
  }

  async function loadRevenue() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/revenue"));
    state.revenue = data?.items || [];
  }

  async function loadFunnels() {
    if (!state.websiteId) return;
    const data = await safeRequest(`/api/websites/${state.websiteId}/funnels`);
    state.funnels = data?.funnels || [];
  }

  async function runFunnel(funnelId) {
    state.selectedFunnelId = funnelId;
    const data = await safeRequest(`${buildAnalyticsUrl("/api/analytics/funnel")}&funnel_id=${encodeURIComponent(funnelId)}`);
    state.funnelReport = data?.report || null;
  }

  async function loadPublicShare() {
    const slug = window.location.pathname.split("/").pop();
    const data = await safeRequest(`/api/public/shares/${slug}?from=${state.from}&to=${state.to}`, { swallow: true });
    if (!data?.website) {
      state.publicShare = null;
      state.error = ctx.t("shareDisabled");
      return;
    }
    state.publicShare = data;
  }

  async function loadRealtime() {
    const data = await safeRequest(buildAnalyticsUrl("/api/analytics/realtime"), { swallow: true });
    state.realtime = data?.realtime || null;
  }

  return {
    buildAnalyticsUrl,
    refreshActive,
    loadRouteData,
    loadOverview,
    loadPages,
    loadEvents,
    loadReferrers,
    loadDevices,
    loadGeo,
    loadAttribution,
    loadRetention,
    loadRevenue,
    loadFunnels,
    runFunnel,
    loadPublicShare,
    loadRealtime,
  };
}
