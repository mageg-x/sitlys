import { defaultFunnelSteps } from "../lib/defaults";

export function createAdminActions(ctx) {
  const {
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
  } = ctx;

  async function loadPixels() {
    if (!state.websiteId) return;
    const data = await safeRequest(`/api/websites/${state.websiteId}/pixels`);
    state.pixels = data?.pixels || [];
  }

  async function loadShares() {
    if (!state.websiteId) return;
    const data = await safeRequest(`/api/websites/${state.websiteId}/shares`);
    state.shares = data?.shares || [];
  }

  async function loadWebsites() {
    const data = await safeRequest("/api/websites");
    state.websites = data?.websites || [];
    const savedWebsite = localStorage.getItem("sitlys.website_id");
    if (!state.websiteId && savedWebsite && state.websites.some(site => site.id === savedWebsite)) {
      state.websiteId = savedWebsite;
    }
    if (!state.websiteId && state.websites[0]?.id) {
      state.websiteId = state.websites[0].id;
    }
  }

  async function loadUsers() {
    if (!isSuperAdmin.value) return;
    const data = await safeRequest("/api/users");
    state.users = data?.users || [];
  }

  async function loadSettings() {
    if (!isSuperAdmin.value) return;
    const data = await safeRequest("/api/settings");
    state.settings = data?.settings || null;
    state.botAudit = data?.bot_audit || {};
    if (state.settings) {
      settingsForm.listen_addr = state.settings.listen_addr || "";
      settingsForm.database_path = state.settings.database_path || "";
      settingsForm.log_level = state.settings.log_level || "info";
      settingsForm.data_retention_days = state.settings.data_retention_days || 365;
      settingsForm.bot_filter_mode = state.settings.bot_filter_mode || "balanced";
    }
  }

  async function saveWebsite() {
    await runSubmit(async () => {
      if (websiteForm.id) {
        await request(`/api/websites/${websiteForm.id}`, {
          method: "PUT",
          body: { name: websiteForm.name, domain: websiteForm.domain },
        });
        state.websiteId = websiteForm.id;
        state.notice = t("saveSuccess");
      } else {
        const response = await request("/api/websites", {
          method: "POST",
          body: { name: websiteForm.name, domain: websiteForm.domain },
        });
        state.websiteId = response?.id || state.websiteId;
        state.notice = t("createSuccess");
      }
      resetWebsiteForm();
      await loadWebsites();
      localStorage.setItem("sitlys.website_id", state.websiteId);
      await ctx.refreshActive();
    });
  }

  async function deleteWebsite() {
    if (!websiteForm.id) return;
    await runSubmit(async () => {
      await request(`/api/websites/${websiteForm.id}`, { method: "DELETE" });
      if (state.websiteId === websiteForm.id) {
        state.websiteId = "";
      }
      resetWebsiteForm();
      state.notice = t("saveSuccess");
      await loadWebsites();
      await ctx.refreshActive();
    });
  }

  async function savePixel() {
    await runSubmit(async () => {
      await request(`/api/websites/${state.websiteId}/pixels`, {
        method: "POST",
        body: { name: pixelForm.name },
      });
      pixelForm.name = "";
      state.notice = t("createSuccess");
      await loadPixels();
    });
  }

  async function togglePixel(pixel, enabled) {
    await runSubmit(async () => {
      await request(`/api/pixels/${pixel.id}`, {
        method: "PUT",
        body: { name: pixel.name, enabled },
      });
      await loadPixels();
    });
  }

  async function saveShare() {
    await runSubmit(async () => {
      await request(`/api/websites/${state.websiteId}/shares`, { method: "POST" });
      state.notice = t("createSuccess");
      await loadShares();
    });
  }

  async function toggleShare(share, enabled) {
    await runSubmit(async () => {
      await request(`/api/shares/${share.id}`, {
        method: "PUT",
        body: { enabled },
      });
      await loadShares();
    });
  }

  async function saveFunnel() {
    await runSubmit(async () => {
      await request(`/api/websites/${state.websiteId}/funnels`, {
        method: "POST",
        body: {
          name: funnelForm.name,
          steps: funnelForm.steps.map(step => ({
            label: step.label,
            type: step.type,
            value: step.value,
          })),
        },
      });
      resetFunnelForm();
      state.notice = t("createSuccess");
      await ctx.loadFunnels();
    });
  }

  async function saveUser() {
    await runSubmit(async () => {
      const payload = {
        username: userForm.username,
        password: userForm.password,
        role: userForm.role,
        enabled: userForm.enabled,
        permissions: userForm.role === "super_admin" ? [] : userForm.permissions,
      };
      if (userForm.id) {
        await request(`/api/users/${userForm.id}`, { method: "PUT", body: payload });
        state.notice = t("saveSuccess");
      } else {
        await request("/api/users", { method: "POST", body: payload });
        state.notice = t("createSuccess");
      }
      await loadUsers();
      resetUserForm();
    });
  }

  async function saveSettings() {
    if (!isSuperAdmin.value) return;
    await runSubmit(async () => {
      await request("/api/settings", {
        method: "PUT",
        body: {
          listen_addr: settingsForm.listen_addr,
          database_path: settingsForm.database_path,
          log_level: settingsForm.log_level,
          data_retention_days: Number(settingsForm.data_retention_days || 365),
          bot_filter_mode: settingsForm.bot_filter_mode || "balanced",
        },
      });
      await loadSettings();
      state.notice = t("settingsSaved");
    });
  }

  async function createBackup() {
    if (!isSuperAdmin.value) return;
    await runSubmit(async () => {
      const response = await request("/api/settings/backup", { method: "POST" });
      state.backupPath = response.backup_path || "";
      state.notice = t("backupCreated");
    });
  }

  async function runCleanup() {
    if (!isSuperAdmin.value) return;
    await runSubmit(async () => {
      const response = await request("/api/settings/cleanup", { method: "POST" });
      state.cleanupResult = response.result || null;
      if (state.cleanupResult?.last_cleanup_at) {
        settingsForm.last_cleanup_at = state.cleanupResult.last_cleanup_at;
      }
      await loadSettings();
      state.notice = t("cleanupFinished");
    });
  }

  async function exportData(kind = "events", format = "csv") {
    if (!state.websiteId) {
      state.error = t("websiteRequired");
      return;
    }
    const url = `/api/analytics/export?website_id=${encodeURIComponent(state.websiteId)}&from=${state.from}&to=${state.to}&kind=${encodeURIComponent(kind)}&format=${encodeURIComponent(format)}`;
    const controller = new AbortController();
    const timer = window.setTimeout(() => controller.abort(), 15000);
    const response = await fetch(url, { credentials: "include", signal: controller.signal }).finally(() => {
      window.clearTimeout(timer);
    });
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      throw new Error(data.error || t("requestFailed"));
    }
    const blob = await response.blob();
    const objectURL = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    const extension = format === "json" ? "json" : "csv";
    anchor.href = objectURL;
    anchor.download = `sitlys-${kind}-${state.from}-${state.to}.${extension}`;
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(objectURL);
  }

  function editWebsite(site) {
    websiteForm.id = site.id;
    websiteForm.name = site.name;
    websiteForm.domain = site.domain;
  }

  function editUser(user) {
    userForm.id = user.id;
    userForm.username = user.username;
    userForm.password = "";
    userForm.role = user.role;
    userForm.enabled = Boolean(user.enabled);
    userForm.permissions = (user.permissions || []).map(item => ({
      website_id: item.website_id,
      access_level: item.access_level || "view",
    }));
  }

  function resetWebsiteForm() {
    websiteForm.id = "";
    websiteForm.name = "";
    websiteForm.domain = "";
  }

  function resetUserForm() {
    userForm.id = "";
    userForm.username = "";
    userForm.password = "";
    userForm.role = "viewer";
    userForm.enabled = true;
    userForm.permissions = [];
  }

  function resetFunnelForm() {
    funnelForm.name = "";
    funnelForm.steps = defaultFunnelSteps();
  }

  function addStep() {
    funnelForm.steps.push({ label: "", type: "page", value: "" });
  }

  function removeStep(index) {
    funnelForm.steps.splice(index, 1);
  }

  function hasPermission(websiteId) {
    return userForm.permissions.some(item => item.website_id === websiteId && (item.access_level === "view" || item.access_level === "manage"));
  }

  function hasManage(websiteId) {
    return userForm.permissions.some(item => item.website_id === websiteId && item.access_level === "manage");
  }

  function accessLevel(websiteId) {
    const item = userForm.permissions.find(entry => entry.website_id === websiteId);
    return item?.access_level || "none";
  }

  function setAccessLevel(websiteId, level) {
    const index = userForm.permissions.findIndex(item => item.website_id === websiteId);
    if (level === "none") {
      if (index >= 0) {
        userForm.permissions.splice(index, 1);
      }
      return;
    }
    if (index === -1) {
      userForm.permissions.push({ website_id: websiteId, access_level: level });
      return;
    }
    userForm.permissions[index].access_level = level;
  }

  function setPermission(websiteId, enabled) {
    if (enabled) {
      setAccessLevel(websiteId, "view");
      return;
    }
    const index = userForm.permissions.findIndex(item => item.website_id === websiteId);
    if (index >= 0) {
      userForm.permissions.splice(index, 1);
    }
  }

  function setManage(websiteId, enabled) {
    if (enabled) {
      setAccessLevel(websiteId, "manage");
      return;
    }
    setAccessLevel(websiteId, "view");
  }

  return {
    loadPixels,
    loadShares,
    loadWebsites,
    loadUsers,
    loadSettings,
    saveWebsite,
    deleteWebsite,
    savePixel,
    togglePixel,
    saveShare,
    toggleShare,
    saveFunnel,
    saveUser,
    saveSettings,
    createBackup,
    runCleanup,
    exportData,
    editWebsite,
    editUser,
    resetWebsiteForm,
    resetUserForm,
    resetFunnelForm,
    addStep,
    removeStep,
    hasPermission,
    hasManage,
    accessLevel,
    setAccessLevel,
    setPermission,
    setManage,
  };
}
