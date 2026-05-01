export function createAuthActions(ctx) {
  const {
    t,
    state,
    initForm,
    loginForm,
    passwordForm,
    localeRef,
    runSubmit,
    request,
    safeRequest,
    syncRouteFromHash,
    hydrateWorkspace,
    loadPublicShare,
  } = ctx;

  async function bootstrap() {
    const savedLocale = localStorage.getItem("sitlys.locale");
    if (savedLocale) {
      localeRef.value = savedLocale;
    } else {
      localeRef.value = navigator.language?.toLowerCase().startsWith("zh") ? "zh-CN" : "en-US";
    }
    if (window.location.pathname.startsWith("/share/")) {
      state.mode = "share";
      await loadPublicShare();
      return;
    }

    ctx.clearFeedback();
    const status = await safeRequest("/api/status");
    state.initialized = Boolean(status?.initialized);
    state.version = status?.version || "";
    if (!state.initialized) {
      state.mode = "init";
      return;
    }

    const meResponse = await safeRequest("/api/auth/me", { swallow: true });
    if (!meResponse?.user) {
      state.mode = "login";
      return;
    }
    state.me = meResponse.user;
    syncRouteFromHash();
    state.mode = "app";
    await hydrateWorkspace();
  }

  async function submitInit() {
    if (initForm.password !== initForm.confirmPassword) {
      state.error = t("passwordMismatch");
      return;
    }
    await runSubmit(async () => {
      await request("/api/init", {
        method: "POST",
        body: { username: initForm.username, password: initForm.password },
      });
      state.notice = t("initSuccess");
      await bootstrap();
    });
  }

  async function submitLogin() {
    await runSubmit(async () => {
      await request("/api/auth/login", {
        method: "POST",
        body: { username: loginForm.username, password: loginForm.password },
      });
      await bootstrap();
    });
  }

  async function submitLogout() {
    await request("/api/auth/logout", { method: "POST" }).catch(() => null);
    state.me = null;
    state.mode = "login";
  }

  async function changePassword() {
    if (passwordForm.newPassword !== passwordForm.confirmPassword) {
      state.error = t("passwordMismatch");
      return;
    }
    await runSubmit(async () => {
      await request("/api/auth/password", {
        method: "POST",
        body: {
          current_password: passwordForm.currentPassword,
          new_password: passwordForm.newPassword,
        },
      });
      passwordForm.currentPassword = "";
      passwordForm.newPassword = "";
      passwordForm.confirmPassword = "";
      state.notice = t("passwordChanged");
    });
  }

  return {
    bootstrap,
    submitInit,
    submitLogin,
    submitLogout,
    changePassword,
  };
}
