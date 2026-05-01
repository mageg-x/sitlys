<template>
  <AppShell v-if="app.state.mode === 'app'" :page-component="workspaceComponent" />
  <InitPage v-else-if="app.state.mode === 'init'" />
  <LoginPage v-else-if="app.state.mode === 'login'" />
  <PublicSharePage v-else-if="app.state.mode === 'share'" />
  <LoadingPage v-else />
</template>

<script setup>
import { computed, onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";
import AppShell from "./layout/AppShell.vue";
import { createAppController, provideAppController } from "./composables/use-app-controller";
import LoadingPage from "./pages/LoadingPage.vue";
import InitPage from "./pages/InitPage.vue";
import LoginPage from "./pages/LoginPage.vue";
import PublicSharePage from "./pages/PublicSharePage.vue";
import OverviewPage from "./pages/OverviewPage.vue";
import PagesReportPage from "./pages/PagesReportPage.vue";
import EventsReportPage from "./pages/EventsReportPage.vue";
import ReferrersPage from "./pages/ReferrersPage.vue";
import DevicesPage from "./pages/DevicesPage.vue";
import GeoPage from "./pages/GeoPage.vue";
import AttributionPage from "./pages/AttributionPage.vue";
import FunnelsPage from "./pages/FunnelsPage.vue";
import RetentionPage from "./pages/RetentionPage.vue";
import RevenuePage from "./pages/RevenuePage.vue";
import PixelsPage from "./pages/PixelsPage.vue";
import SharesPage from "./pages/SharesPage.vue";
import WebsitesPage from "./pages/WebsitesPage.vue";
import UsersPage from "./pages/UsersPage.vue";
import SettingsPage from "./pages/SettingsPage.vue";

const { t, locale } = useI18n();

const app = createAppController({ t, localeRef: locale });
provideAppController(app);

const workspaceRegistry = {
  overview: OverviewPage,
  pages: PagesReportPage,
  events: EventsReportPage,
  referrers: ReferrersPage,
  devices: DevicesPage,
  geo: GeoPage,
  attribution: AttributionPage,
  funnels: FunnelsPage,
  retention: RetentionPage,
  revenue: RevenuePage,
  pixels: PixelsPage,
  shares: SharesPage,
  websites: WebsitesPage,
  users: UsersPage,
  settings: SettingsPage,
};

const workspaceComponent = computed(() => workspaceRegistry[app.state.route] || OverviewPage);

onMounted(app.bootstrap);
onMounted(() => window.addEventListener("hashchange", app.handleHashChange));
onUnmounted(() => window.removeEventListener("hashchange", app.handleHashChange));
</script>
