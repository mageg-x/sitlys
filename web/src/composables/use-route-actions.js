export function createRouteActions(ctx) {
  const {
    state,
    isSuperAdmin,
    validRoutes,
    loadRouteData,
    refreshActive,
    loadWebsites,
    editUser,
    userForm,
    syncHashFromRoute,
    clearFeedback,
  } = ctx;

  async function handleWebsiteChange() {
    localStorage.setItem("sitlys.website_id", state.websiteId);
    await refreshActive();
  }

  async function applyRange() {
    await refreshActive();
  }

  async function selectRoute(route) {
    state.route = route;
    syncHashFromRoute();
    clearFeedback();
    await loadRouteData();
  }

  function syncRouteFromHash() {
    let route = window.location.hash.replace(/^#/, "").trim();
    if (route === "roles") {
      // Keep old shared links/bookmarks working after collapsing role management into users.
      route = "users";
    }
    if (!route || !validRoutes.has(route)) {
      return;
    }
    if (route === "users" && !isSuperAdmin.value) {
      return;
    }
    state.route = route;
  }

  function syncHashFromRouteSafe() {
    const nextHash = `#${state.route}`;
    if (window.location.hash !== nextHash) {
      window.location.hash = nextHash;
    }
  }

  function handleHashChange() {
    const previousRoute = state.route;
    syncRouteFromHash();
    if (state.mode === "app" && state.route !== previousRoute) {
      clearFeedback();
      void loadRouteData();
    }
  }

  async function hydrateWorkspace() {
    await loadWebsites();
    if (!state.websiteId && state.websites.length) {
      state.websiteId = state.websites[0].id;
    }
    await refreshActive();
    if (isSuperAdmin.value) {
      await ctx.loadUsers();
      await ctx.loadSettings();
    }
  }

  return {
    handleWebsiteChange,
    applyRange,
    selectRoute,
    syncRouteFromHash,
    syncHashFromRoute: syncHashFromRouteSafe,
    handleHashChange,
    hydrateWorkspace,
  };
}
