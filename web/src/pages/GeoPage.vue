<template>
  <section class="analytics-stack">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("geoCoverage") }}</h3>
          <p>{{ app.t("geoCoverageText") }}</p>
        </div>
      </div>
      <div class="insight-band">
        <div v-for="item in app.geoHighlights" :key="item.key" class="insight-pill">
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
            <h3>{{ app.t("topCountries") }}</h3>
            <p>{{ app.t("regionalFocus") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.geo" label-key="country" value-key="visits" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("topRegions") }}</h3>
            <p>{{ app.t("regionalFocus") }}</p>
          </div>
        </div>
        <SimpleTable :rows="app.state.geoRegions" :columns="app.regionColumns" :empty-text="app.t('noData')" />
      </article>
    </div>

    <div class="analytics-grid analytics-grid-split">
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("topCities") }}</h3>
            <p>{{ app.t("focusRegions") }}</p>
          </div>
        </div>
        <SimpleTable :rows="app.state.geoCities" :columns="app.cityColumns" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("geoTable") }}</h3>
            <p>{{ app.t("quickRead") }}</p>
          </div>
        </div>
        <SimpleTable :rows="app.state.geo" :columns="app.geoColumns" :empty-text="app.t('noData')" />
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
