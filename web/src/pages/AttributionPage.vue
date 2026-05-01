<template>
  <section class="analytics-stack">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("attributionMix") }}</h3>
          <p>{{ app.t("attributionText") }}</p>
        </div>
      </div>
      <div class="insight-band">
        <div v-for="item in app.attributionHighlights" :key="item.key" class="insight-pill">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <small>{{ item.hint }}</small>
        </div>
      </div>
    </article>

    <div class="analytics-grid analytics-grid-split">
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("focusSources") }}</h3>
            <p>{{ app.t("sourceMediums") }}</p>
          </div>
        </div>
        <SimpleTable :rows="app.state.attribution.slice(0, 8)" :columns="app.attributionColumns" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("topCampaigns") }}</h3>
            <p>{{ app.t("revenueSignals") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.attribution" label-key="campaign" value-key="revenue" :empty-text="app.t('noData')" />
      </article>
    </div>

    <article class="panel">
      <div class="panel-head">
        <div>
          <h3>{{ app.t("attribution") }}</h3>
          <p>{{ app.t("quickRead") }}</p>
        </div>
      </div>
      <label class="analytics-filter">
        <span>{{ app.t("filterRows") }}</span>
        <input v-model.trim="app.attributionFilter.query" :placeholder="app.t('campaign')" />
      </label>
      <SimpleTable :rows="app.filteredAttribution" :columns="app.attributionColumns" :empty-text="app.t('noData')" />
    </article>
  </section>
</template>

<script setup>
import DataBars from "../components/DataBars.vue";
import SimpleTable from "../components/SimpleTable.vue";
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>
