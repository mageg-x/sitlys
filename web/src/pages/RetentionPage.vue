<template>
  <section class="analytics-stack">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("cohortHeatmap") }}</h3>
          <p>{{ app.t("retentionText") }}</p>
        </div>
      </div>
      <div class="insight-band">
        <div v-for="item in app.retentionHighlights" :key="item.key" class="insight-pill">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <small>{{ item.hint }}</small>
        </div>
      </div>
    </article>

    <article class="panel">
      <div class="panel-head">
        <div>
          <h3>{{ app.t("cohortHeatmap") }}</h3>
          <p>{{ app.t("cohortHealth") }}</p>
        </div>
        <span class="heat-legend">{{ app.t("heatLegend") }}</span>
      </div>
      <div v-if="app.state.retention.length" class="heatmap-grid">
        <div class="heatmap-row heatmap-head">
          <span>{{ app.t("cohort") }}</span>
          <span>{{ app.t("size") }}</span>
          <span>{{ app.t("day1") }}</span>
          <span>{{ app.t("day7") }}</span>
          <span>{{ app.t("day30") }}</span>
        </div>
        <div v-for="row in app.state.retention" :key="row.cohort" class="heatmap-row">
          <span>{{ row.cohort }}</span>
          <span>{{ app.formatNumber(row.size) }}</span>
          <span class="heatmap-cell" :style="{ '--heat': row.day_1 }">{{ app.formatPercent(row.day_1) }}</span>
          <span class="heatmap-cell" :style="{ '--heat': row.day_7 }">{{ app.formatPercent(row.day_7) }}</span>
          <span class="heatmap-cell" :style="{ '--heat': row.day_30 }">{{ app.formatPercent(row.day_30) }}</span>
        </div>
      </div>
      <p v-else class="empty-note">{{ app.t("noData") }}</p>
    </article>
  </section>
</template>

<script setup>
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>
