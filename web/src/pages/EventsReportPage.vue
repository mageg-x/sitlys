<template>
  <section class="analytics-stack">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("eventHighlights") }}</h3>
          <p>{{ app.t("eventHighlightsText") }}</p>
        </div>
      </div>
      <div class="insight-band">
        <div v-for="item in app.eventHighlights" :key="item.key" class="insight-pill">
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
            <h3>{{ app.t("focusEvents") }}</h3>
            <p>{{ app.t("eventCount") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.events" label-key="name" value-key="events" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("eventTypes") }}</h3>
            <p>{{ app.t("eventCoverage") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.eventTypes" label-key="type" value-key="events" :empty-text="app.t('noData')" />
      </article>
    </div>

    <div class="analytics-grid analytics-grid-split">
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("revenueSignals") }}</h3>
            <p>{{ app.t("revenue") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.events" label-key="name" value-key="revenue" :empty-text="app.t('noData')" />
      </article>

      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("eventCoverage") }}</h3>
            <p>{{ app.t("sessions") }}</p>
          </div>
        </div>
        <DataBars :rows="app.state.events" label-key="name" value-key="sessions" :empty-text="app.t('noData')" />
      </article>
    </div>

    <article class="panel">
      <div class="panel-head">
        <div>
          <h3>{{ app.t("eventTable") }}</h3>
          <p>{{ app.t("keySignals") }}</p>
        </div>
      </div>
      <label class="analytics-filter">
        <span>{{ app.t("filterRows") }}</span>
        <input v-model.trim="app.eventFilter.query" :placeholder="app.t('eventName')" />
      </label>
      <SimpleTable :rows="app.filteredEvents" :columns="app.eventColumns" :empty-text="app.t('noData')" />
    </article>
  </section>
</template>

<script setup>
import DataBars from "../components/DataBars.vue";
import SimpleTable from "../components/SimpleTable.vue";
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>
