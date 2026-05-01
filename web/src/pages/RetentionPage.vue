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
      <div v-if="app.state.retention.length" class="retention-matrix">
        <div class="retention-side retention-side-head">
          <span>{{ app.t("cohort") }}</span>
          <span>{{ app.t("size") }}</span>
        </div>
        <div class="retention-matrix-head">
          <span>{{ app.t("day1") }}</span>
          <span>{{ app.t("day7") }}</span>
          <span>{{ app.t("day30") }}</span>
        </div>
        <template v-for="row in app.state.retention" :key="row.cohort">
          <div class="retention-side">
            <strong>{{ row.cohort }}</strong>
            <span>{{ app.formatNumber(row.size) }}</span>
          </div>
          <div class="retention-cells">
            <span class="heatmap-cell" :style="{ '--heat': row.day_1 }">{{ app.formatPercent(row.day_1) }}</span>
            <span class="heatmap-cell" :style="{ '--heat': row.day_7 }">{{ app.formatPercent(row.day_7) }}</span>
            <span class="heatmap-cell" :style="{ '--heat': row.day_30 }">{{ app.formatPercent(row.day_30) }}</span>
          </div>
        </template>
      </div>
      <div v-if="app.state.retention.length" class="retention-table">
        <div class="heatmap-row heatmap-head">
          <span>{{ app.t("cohort") }}</span>
          <span>{{ app.t("size") }}</span>
          <span>{{ app.t("day1") }}</span>
          <span>{{ app.t("day7") }}</span>
          <span>{{ app.t("day30") }}</span>
        </div>
        <div v-for="row in app.state.retention" :key="`${row.cohort}-table`" class="heatmap-row">
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

<style scoped>
.retention-matrix {
  display: grid;
  grid-template-columns: 10rem minmax(0, 1fr);
  gap: 0.65rem 0.85rem;
  align-items: stretch;
}

.retention-side,
.retention-matrix-head,
.retention-cells {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.6rem;
}

.retention-side {
  grid-template-columns: 1fr auto;
  align-items: center;
  padding: 0.8rem 0.9rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.74);
}

.retention-side-head,
.retention-matrix-head {
  color: var(--muted);
  font-size: 0.74rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.retention-cells .heatmap-cell {
  min-height: 4.2rem;
  display: flex;
  align-items: center;
  justify-content: center;
}

.retention-table {
  display: none;
}

@media (max-width: 900px) {
  .retention-matrix {
    display: none;
  }

  .retention-table {
    display: grid;
    gap: 0.65rem;
  }
}
</style>
