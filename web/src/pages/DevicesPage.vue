<template>
  <section class="analytics-stack">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("deviceMix") }}</h3>
          <p>{{ app.t("deviceMixText") }}</p>
        </div>
      </div>
      <div class="insight-band">
        <div v-for="item in app.deviceHighlights" :key="item.key" class="insight-pill">
          <span>{{ item.label }}</span>
          <strong>{{ item.value }}</strong>
          <small>{{ item.hint }}</small>
        </div>
      </div>
    </article>

    <div class="analytics-grid analytics-grid-wide">
      <article class="panel tech-card">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("topBrowsers") }}</h3>
            <p>{{ app.t("browserShare") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.devices.browsers" label-key="value" value-key="visits" :empty-text="app.t('noData')" />
      </article>

      <article class="panel tech-card">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("topOperatingSystems") }}</h3>
            <p>{{ app.t("environment") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.devices.os" label-key="value" value-key="visits" :empty-text="app.t('noData')" />
      </article>

      <article class="panel tech-card">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("topDevices") }}</h3>
            <p>{{ app.t("techSegments") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.devices.devices" label-key="value" value-key="visits" :empty-text="app.t('noData')" />
      </article>
    </div>

    <article class="panel">
      <div class="panel-head">
        <div>
          <h3>{{ app.t("deviceMatrix") }}</h3>
          <p>{{ app.t("quickRead") }}</p>
        </div>
      </div>
      <SimpleTable :rows="app.state.devices.matrix" :columns="app.deviceMatrixColumns" :empty-text="app.t('noData')" />
    </article>
  </section>
</template>

<script setup>
import DataBars from "../components/DataBars.vue";
import SimpleTable from "../components/SimpleTable.vue";
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>
