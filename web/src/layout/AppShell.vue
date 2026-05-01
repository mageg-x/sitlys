<template>
  <main class="app-frame">
    <div class="ambient ambient-a"></div>
    <div class="ambient ambient-b"></div>
    <div class="ambient ambient-c"></div>
    <section class="workspace">
      <aside class="sidebar">
        <div class="brand-card">
          <div class="brand-icon">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="url(#brandGrad)" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
              <defs><linearGradient id="brandGrad" x1="0%" y1="0%" x2="100%" y2="100%"><stop offset="0%" stop-color="#6366f1"/><stop offset="100%" stop-color="#14b8a6"/></linearGradient></defs>
              <path d="M3 3v18h18"/><path d="m19 9-5 5-4-4-3 3"/>
            </svg>
          </div>
          <div class="brand-copy">
            <h1>{{ app.t("appName") }}</h1>
            <span>{{ app.t("appTagline") }}</span>
          </div>
        </div>

        <nav class="sidebar-nav">
          <button
            v-for="item in app.navItems"
            :key="item.key"
            class="nav-button"
            :class="{ active: app.state.route === item.key }"
            :title="item.label"
            @click="app.selectRoute(item.key)"
          >
            <span class="nav-icon-wrap" v-html="navIcons[item.key] || navIcons.overview"></span>
            <span class="nav-label">{{ item.label }}</span>
          </button>
        </nav>

        <article class="sidebar-foot panel">
          <div class="sidebar-user-avatar">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
          </div>
          <div class="sidebar-user-copy">
            <strong>{{ app.state.me?.username }}</strong>
            <span>{{ app.roleLabel(app.state.me?.role) }}</span>
          </div>
        </article>
      </aside>

      <div class="main-shell">
        <header class="shell-header panel">
          <div class="shell-headline">
            <h2>{{ app.routeMeta.title }}</h2>
          </div>
          <div class="shell-actions">
            <span class="status-pill" :class="{ 'status-active': app.state.initialized }">
              <span class="status-dot"></span>
              {{ app.t("status") }}: {{ app.state.initialized ? app.t("initialized") : app.t("pending") }}
            </span>
            <LocaleSwitch v-model="app.locale" :t="app.t" />
            <button class="ghost-button icon-button" :title="app.t('refresh')" @click="app.refreshActive">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 0 0-9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M3 12a9 9 0 0 0 9 9 9.75 9.75 0 0 0 6.74-2.74L21 16"/><path d="M16 16h5v5"/></svg>
            </button>
            <button class="ghost-button icon-button" :title="app.t('signOut')" @click="app.submitLogout">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
            </button>
          </div>
        </header>

        <section v-if="showContextPanel" class="context-bar panel">
          <div v-if="showWebsiteSelector" class="context-item">
            <select v-model="app.state.websiteId" @change="app.handleWebsiteChange">
              <option v-if="!app.state.websites.length" value="">{{ app.t("chooseWebsite") }}</option>
              <option v-for="site in app.state.websites" :key="site.id" :value="site.id">{{ site.name }}</option>
            </select>
          </div>

          <div v-if="showDateRange" class="context-item">
            <input v-model="app.state.from" type="date" />
            <span class="range-sep">—</span>
            <input v-model="app.state.to" type="date" />
            <button class="ghost-button icon-button" :title="app.t('apply')" @click="app.applyRange">
              <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"/></svg>
            </button>
          </div>
        </section>

        <section v-if="showMetrics" class="metric-grid">
          <article v-for="item in app.overviewCards" :key="item.key" class="metric-card panel">
            <div class="metric-head">
              <div class="metric-icon" v-html="metricIcons[item.key] || metricIcons.pageviews"></div>
              <span>{{ item.label }}</span>
            </div>
            <strong>{{ item.value }}</strong>
          </article>
        </section>

        <section v-if="app.state.notice || app.state.error" class="notice-strip">
          <div v-if="app.state.notice" class="notice success">{{ app.state.notice }}</div>
          <div v-if="app.state.error" class="notice danger">{{ app.state.error }}</div>
        </section>

        <section class="content-stack">
          <component :is="pageComponent" />
        </section>
      </div>
    </section>
  </main>
</template>

<script setup>
import { computed } from "vue";
import LocaleSwitch from "../components/LocaleSwitch.vue";
import { useAppController } from "../composables/use-app-controller";

defineProps({
  pageComponent: { type: [Object, Function], required: true },
});

const app = useAppController();

const analyticsRoutes = new Set([
  "overview",
  "pages",
  "events",
  "referrers",
  "geo",
  "devices",
  "attribution",
  "funnels",
  "retention",
  "revenue",
]);

const websiteScopedRoutes = new Set([
  "overview",
  "pages",
  "events",
  "referrers",
  "geo",
  "devices",
  "attribution",
  "funnels",
  "retention",
  "revenue",
  "pixels",
  "shares",
]);

const dateScopedRoutes = new Set([
  "overview",
  "pages",
  "events",
  "referrers",
  "geo",
  "devices",
  "attribution",
  "funnels",
  "retention",
  "revenue",
]);

const showWebsiteSelector = computed(() => websiteScopedRoutes.has(app.state.route));
const showDateRange = computed(() => dateScopedRoutes.has(app.state.route));
const showContextPanel = computed(() => showWebsiteSelector.value || showDateRange.value);
const showMetrics = computed(() => analyticsRoutes.has(app.state.route));

const navIcons = {
  overview: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg>',
  pages: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5L14.5 2z"/><polyline points="14 2 14 8 20 8"/><line x1="16" y1="13" x2="8" y2="13"/><line x1="16" y1="17" x2="8" y2="17"/><line x1="10" y1="9" x2="8" y2="9"/></svg>',
  events: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 12 18 12 15 21 9 3 6 12 2 12"/></svg>',
  referrers: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>',
  geo: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z"/><circle cx="12" cy="10" r="3"/></svg>',
  devices: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>',
  attribution: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><line x1="6" y1="3" x2="6" y2="15"/><circle cx="18" cy="6" r="3"/><circle cx="6" cy="18" r="3"/><path d="M18 9a9 9 0 0 1-9 9"/></svg>',
  funnels: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4h18l-7 8v6l-4 2v-8z"/></svg>',
  retention: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="17 1 21 5 17 9"/><path d="M3 11V9a4 4 0 0 1 4-4h14"/><polyline points="7 23 3 19 7 15"/><path d="M21 13v2a4 4 0 0 1-4 4H3"/></svg>',
  revenue: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><path d="M12 17h.01"/></svg>',
  pixels: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/></svg>',
  shares: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" y1="13.51" x2="15.42" y2="17.49"/><line x1="15.41" y1="6.51" x2="8.59" y2="10.49"/></svg>',
  websites: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>',
  users: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>',
  settings: '<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/></svg>',
};

const metricIcons = {
  pageviews: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z"/><circle cx="12" cy="12" r="3"/></svg>',
  visitors: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><polyline points="16 11 18 13 22 9"/></svg>',
  sessions: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v8a4 4 0 0 0 4 4h8a4 4 0 0 0 4-4"/><path d="m12 8 4 4-4 4"/><line x1="2" y1="12" x2="16" y2="12"/></svg>',
  events: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/></svg>',
  revenue: '<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="22 7 13.5 15.5 8.5 10.5 2 17"/><polyline points="16 7 22 7 22 13"/></svg>',
};
</script>

<style scoped>
.brand-card,
.sidebar-foot {
  display: flex;
  align-items: center;
  gap: 0.8rem;
}

.brand-card {
  padding: 0.95rem 1rem;
}

.brand-icon {
  width: 34px;
  height: 34px;
  border-radius: 9px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.12), rgba(20, 184, 166, 0.09));
  border: 1px solid rgba(99, 102, 241, 0.14);
  box-shadow: var(--shadow-xs);
}

.brand-copy {
  display: grid;
  gap: 0.08rem;
  min-width: 0;
}

.brand-copy h1 {
  font-size: 1rem;
  line-height: 1.15;
}

.brand-copy span {
  color: var(--muted);
  font-size: 0.76rem;
}

.sidebar-user-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--accent);
  background: linear-gradient(135deg, rgba(20, 184, 166, 0.12), rgba(99, 102, 241, 0.1));
  border: 1px solid rgba(20, 184, 166, 0.15);
}

.sidebar-foot {
  padding: 0.9rem 1rem;
}

.sidebar-user-copy {
  display: grid;
  gap: 0.08rem;
  min-width: 0;
}

.sidebar-user-copy strong {
  font-size: 0.9rem;
  line-height: 1.15;
}

.sidebar-user-copy span {
  color: var(--muted);
  font-size: 0.76rem;
}

.nav-button {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  min-height: 44px;
}

.nav-icon-wrap {
  flex-shrink: 0;
  width: 28px;
  height: 28px;
  border-radius: 7px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(99, 102, 241, 0.06);
  color: var(--muted);
  transition: all var(--transition) ease;
}

.nav-button.active .nav-icon-wrap {
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.16), rgba(20, 184, 166, 0.1));
  color: var(--accent-strong);
}

.nav-button:hover .nav-icon-wrap {
  background: rgba(99, 102, 241, 0.1);
  color: var(--accent);
}

.nav-label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 600;
  font-size: 0.84rem;
}

.shell-header {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  align-items: center;
  padding-top: 2px !important;
  padding-bottom: 2px !important;
}

.shell-headline,
.shell-actions {
  min-width: 0;
}

.shell-headline h2 {
  font-size: 1.25rem;
  line-height: 1.2;
}

.shell-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 0.5rem;
  align-items: center;
  max-width: 100%;
}

.shell-actions .ghost-button {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
}

.icon-button {
  padding: 0.45rem;
}

.metric-icon {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.1), rgba(20, 184, 166, 0.08));
  color: var(--accent-strong);
}

.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
}

.status-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--muted-light);
  flex-shrink: 0;
}

.status-active .status-dot {
  background: var(--success);
  box-shadow: 0 0 8px rgba(16, 185, 129, 0.4);
}

@media (max-width: 1080px) {
  .shell-header,
  .context-panel {
    grid-template-columns: 1fr;
  }

  .shell-header {
    flex-direction: column;
    align-items: stretch;
  }

  .shell-actions {
    justify-content: flex-start;
    width: 100%;
  }
}
</style>
