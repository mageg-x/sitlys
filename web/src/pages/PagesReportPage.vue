<template>
  <section class="analytics-stack">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("pagePerformance") }}</h3>
          <p>{{ app.t("pagePerformanceText") }}</p>
        </div>
      </div>
      <div class="insight-band">
        <div v-for="item in app.pageHighlights" :key="item.key" class="insight-pill">
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
            <h3>{{ app.t("focusPages") }}</h3>
            <p>{{ app.t("contentEntry") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.pages" label-key="path" value-key="pageviews" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("entryPages") }}</h3>
            <p>{{ app.t("contentEntry") }}</p>
          </div>
        </div>
        <SimpleTable :rows="app.state.pageEntries" :columns="app.entryColumns" :empty-text="app.t('noData')" />
      </article>
    </div>

    <div class="analytics-grid analytics-grid-split">
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("sessionDepth") }}</h3>
            <p>{{ app.t("sessions") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.pages" label-key="path" value-key="sessions" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("exitPages") }}</h3>
            <p>{{ app.t("quickRead") }}</p>
          </div>
        </div>
        <SimpleTable :rows="app.state.pageExits" :columns="app.entryColumns" :empty-text="app.t('noData')" />
      </article>
    </div>

    <article class="panel">
      <div class="panel-head">
        <div>
          <h3>{{ app.t("contentTable") }}</h3>
          <p>{{ app.t("quickRead") }}</p>
        </div>
      </div>
      <label class="analytics-filter">
        <span>{{ app.t("filterRows") }}</span>
        <input v-model.trim="app.pageFilter.query" :placeholder="app.t('path')" />
      </label>
      <SimpleTable :rows="app.filteredPages" :columns="app.pageColumns" :empty-text="app.t('noData')" />
    </article>
  </section>
</template>

<script setup>
import DataBars from "../components/DataBars.vue";
import SimpleTable from "../components/SimpleTable.vue";
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>
