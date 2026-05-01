<template>
  <main class="app-frame">
    <div class="ambient ambient-a"></div>
    <div class="ambient ambient-b"></div>
    <div class="ambient ambient-c"></div>
    <section class="share-shell">
      <article class="share-header panel">
        <div>
          <div class="share-logo">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="url(#shareGrad)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <defs><linearGradient id="shareGrad" x1="0%" y1="0%" x2="100%" y2="100%"><stop offset="0%" stop-color="#6366f1"/><stop offset="100%" stop-color="#14b8a6"/></linearGradient></defs>
              <path d="M3 3v18h18"/><path d="m19 9-5 5-4-4-3 3"/>
            </svg>
          </div>
          <p class="eyebrow">{{ app.t("publicShare") }}</p>
          <h1>{{ app.state.publicShare?.website?.name || app.t("shareDisabled") }}</h1>
          <p class="entry-copy">{{ app.t("shareSummary") }}</p>
        </div>
        <div class="share-range">
          <label>
            <span>{{ app.t("dateFrom") }}</span>
            <input v-model="app.state.from" type="date" @change="app.loadPublicShare" />
          </label>
          <label>
            <span>{{ app.t("dateTo") }}</span>
            <input v-model="app.state.to" type="date" @change="app.loadPublicShare" />
          </label>
        </div>
      </article>

      <template v-if="app.state.publicShare">
        <section class="metric-grid">
          <article v-for="(item, index) in app.shareMetrics" :key="item.key" class="metric-card panel" :class="'metric-card-' + ((index % 5) + 1)">
            <span>{{ item.label }}</span>
            <strong>{{ item.value }}</strong>
          </article>
        </section>

        <section class="panel-grid">
          <article class="panel">
            <div class="panel-head"><h2>{{ app.t("topPages") }}</h2></div>
            <SimpleTable :rows="app.state.publicShare.pages || []" :columns="app.sharePageColumns" :empty-text="app.t('noData')" />
          </article>
          <article class="panel">
            <div class="panel-head"><h2>{{ app.t("topSources") }}</h2></div>
            <SimpleTable :rows="app.state.publicShare.referrers || []" :columns="app.shareReferrerColumns" :empty-text="app.t('noData')" />
          </article>
          <article class="panel">
            <div class="panel-head"><h2>{{ app.t("attribution") }}</h2></div>
            <SimpleTable :rows="app.state.publicShare.attribution || []" :columns="app.shareAttributionColumns" :empty-text="app.t('noData')" />
          </article>
          <article class="panel">
            <div class="panel-head"><h2>{{ app.t("revenue") }}</h2></div>
            <SimpleTable :rows="app.state.publicShare.revenue || []" :columns="app.shareRevenueColumns" :empty-text="app.t('noData')" />
          </article>
        </section>
      </template>
    </section>
  </main>
</template>

<script setup>
import SimpleTable from "../components/SimpleTable.vue";
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>

<style scoped>
.share-logo {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.1), rgba(20, 184, 166, 0.08));
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 0.75rem;
  border: 1px solid rgba(99, 102, 241, 0.12);
}
</style>
