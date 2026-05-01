<template>
  <section class="analytics-stack">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("overviewPulse") }}</h3>
          <p>{{ app.t("overviewPulseText") }}</p>
        </div>
      </div>

      <div class="signal-grid">
        <div v-for="item in app.overviewHighlights" :key="item.key" class="signal-card" :class="item.tone">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
        </div>
      </div>
    </article>

    <div class="analytics-grid analytics-grid-wide">
      <article class="panel spotlight-card">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("trafficHealth") }}</h3>
            <p>{{ app.t("quickRead") }}</p>
          </div>
          <button class="ghost-button" @click="app.selectRoute('pages')">{{ app.t("drillDown") }}</button>
        </div>
        <div class="spotlight-value">{{ app.topPage?.path || "/" }}</div>
        <div class="spotlight-meta">
          <span>{{ app.t("pageviews") }} · {{ app.formatNumber(app.topPage?.pageviews || 0) }}</span>
          <span>{{ app.t("sessions") }} · {{ app.formatNumber(app.topPage?.sessions || 0) }}</span>
        </div>
      </article>

      <article class="panel spotlight-card alt">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("acquisitionPulse") }}</h3>
            <p>{{ app.t("trafficComposition") }}</p>
          </div>
          <button class="ghost-button" @click="app.selectRoute('referrers')">{{ app.t("drillDown") }}</button>
        </div>
        <div class="spotlight-value">{{ app.topReferrer?.referrer || app.t("directTraffic") }}</div>
        <div class="spotlight-meta">
          <span>{{ app.t("visits") }} · {{ app.formatNumber(app.topReferrer?.visits || 0) }}</span>
          <span>{{ app.t("country") }} · {{ app.topCountry?.country || "-" }}</span>
        </div>
      </article>

      <article class="panel spotlight-card warm">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("monetizationPulse") }}</h3>
            <p>{{ app.t("keySignals") }}</p>
          </div>
          <button class="ghost-button" @click="app.selectRoute('revenue')">{{ app.t("drillDown") }}</button>
        </div>
        <div class="spotlight-value">{{ app.formatMoney(app.state.overview.revenue || 0) }}</div>
        <div class="spotlight-meta">
          <span>{{ app.t("focusEvents") }} · {{ app.topEvent?.name || app.t("unnamedEvent") }}</span>
          <span>{{ app.t("eventCount") }} · {{ app.formatNumber(app.topEvent?.events || 0) }}</span>
        </div>
      </article>
    </div>

    <article class="panel">
      <div class="panel-head">
        <div>
          <h3>{{ app.t("trendWindow") }}</h3>
          <p>{{ app.t("quickRead") }}</p>
        </div>
      </div>
      <div v-if="app.state.overviewTrend.length" class="trend-grid">
        <div class="trend-card">
          <span>{{ app.t("trendPageviews") }}</span>
          <strong>{{ app.formatNumber(app.state.overviewTrend.reduce((sum, row) => sum + Number(row.pageviews || 0), 0)) }}</strong>
        </div>
        <div class="trend-card">
          <span>{{ app.t("trendEvents") }}</span>
          <strong>{{ app.formatNumber(app.state.overviewTrend.reduce((sum, row) => sum + Number(row.events || 0), 0)) }}</strong>
        </div>
        <div class="trend-card">
          <span>{{ app.t("trendRevenue") }}</span>
          <strong>{{ app.formatMoney(app.state.overviewTrend.reduce((sum, row) => sum + Number(row.revenue || 0), 0)) }}</strong>
        </div>
      </div>
      <div v-if="app.state.overviewTrend.length" class="mini-series">
        <div v-for="row in app.state.overviewTrend" :key="row.date" class="mini-series-col">
          <div class="mini-series-bar" :style="{ height: `${Math.max(10, (Number(row.pageviews || 0) / app.overviewTrendMax) * 100)}%` }"></div>
          <span>{{ row.date.slice(5) }}</span>
        </div>
      </div>
      <p v-else class="empty-note">{{ app.t("noData") }}</p>
    </article>

    <div class="analytics-grid analytics-grid-split">
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("topPages") }}</h3>
            <p>{{ app.t("contentEntry") }}</p>
          </div>
        </div>
        <SimpleTable :rows="app.state.pages.slice(0, 6)" :columns="app.pageColumns" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("topSources") }}</h3>
            <p>{{ app.t("sourceMix") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.referrers" label-key="referrer" value-key="visits" :empty-text="app.t('noData')" />
      </article>
    </div>

    <div class="analytics-grid analytics-grid-split">
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("regions") }}</h3>
            <p>{{ app.t("regionalFocus") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.geo" label-key="country" value-key="visits" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("environmentSummary") }}</h3>
            <p>{{ app.t("techSegments") }}</p>
          </div>
        </div>
        <div class="stacked-groups">
          <section class="stacked-group">
            <h4>{{ app.t("browser") }}</h4>
            <DataBars :rows="app.state.devices.browsers" label-key="value" value-key="visits" :empty-text="app.t('noData')" />
          </section>
          <section class="stacked-group">
            <h4>{{ app.t("device") }}</h4>
            <DataBars :rows="app.state.devices.devices" label-key="value" value-key="visits" :empty-text="app.t('noData')" />
          </section>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup>
import DataBars from "../components/DataBars.vue";
import SimpleTable from "../components/SimpleTable.vue";
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>
