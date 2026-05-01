<template>
  <article v-if="!app.canManageSelectedWebsite" class="panel">
    <div class="panel-head">
      <h3>{{ app.t("readOnlyMode") }}</h3>
      <p>{{ app.t("managePermissionRequiredText") }}</p>
    </div>
  </article>

  <div class="panel-grid">
    <article class="panel">
      <div class="panel-head">
        <h3>{{ app.t("createFunnel") }}</h3>
        <p>{{ app.t("funnelText") }}</p>
      </div>
      <form class="stack-form" @submit.prevent="app.saveFunnel">
        <label>
          <span>{{ app.t("funnelName") }}</span>
          <input v-model.trim="app.funnelForm.name" :disabled="!app.canManageSelectedWebsite" required />
        </label>
        <div class="step-list">
          <div v-for="(step, index) in app.funnelForm.steps" :key="index" class="step-row">
            <input v-model.trim="step.label" :placeholder="app.t('stepLabel')" :disabled="!app.canManageSelectedWebsite" required />
            <select v-model="step.type" :disabled="!app.canManageSelectedWebsite">
              <option value="page">{{ app.t("pageStep") }}</option>
              <option value="event">{{ app.t("eventStep") }}</option>
            </select>
            <input v-model.trim="step.value" :placeholder="app.t('stepValue')" :disabled="!app.canManageSelectedWebsite" required />
            <button v-if="app.funnelForm.steps.length > 2 && app.canManageSelectedWebsite" type="button" class="mini-button" :title="app.t('deleteAction')" @click="app.removeStep(index)">×</button>
          </div>
        </div>
        <div class="form-actions">
          <button type="button" class="ghost-button" :disabled="!app.canManageSelectedWebsite" @click="app.addStep">{{ app.t("addStep") }}</button>
          <button class="primary-button" :disabled="!app.canManageSelectedWebsite">{{ app.t("create") }}</button>
        </div>
      </form>
    </article>

    <article class="panel">
      <div class="panel-head"><h3>{{ app.t("funnels") }}</h3></div>
      <div v-if="app.state.funnels.length" class="card-list">
        <button
          v-for="funnel in app.state.funnels"
          :key="funnel.id"
          class="entity-card"
          :class="{ active: app.state.selectedFunnelId === funnel.id }"
          @click="app.runFunnel(funnel.id)"
        >
          <strong>{{ funnel.name }}</strong>
          <span>{{ funnel.steps.map(step => step.label).join(" / ") }}</span>
        </button>
      </div>
      <p v-else class="empty-note">{{ app.t("noData") }}</p>
    </article>
  </div>

  <article class="panel analytics-hero">
    <div class="panel-head analytics-head">
      <div>
        <h3>{{ app.t("funnelMomentum") }}</h3>
        <p>{{ app.t("funnelText") }}</p>
      </div>
    </div>
    <div class="insight-band">
      <div v-for="item in app.funnelHighlights" :key="item.key" class="insight-pill">
        <span>{{ item.label }}</span>
        <strong>{{ item.value }}</strong>
        <small>{{ item.hint }}</small>
      </div>
    </div>
  </article>

  <article class="panel">
    <div class="panel-head"><h3>{{ app.t("runReport") }}</h3></div>
    <div v-if="app.state.funnelReport?.steps?.length" class="funnel-steps">
      <div v-for="step in app.state.funnelReport.steps" :key="step.index" class="funnel-step">
        <div>
          <strong>{{ step.index }}. {{ step.label }}</strong>
          <div class="footnote">{{ step.type }} · {{ step.value }}</div>
        </div>
        <div class="progress-line">
          <div class="progress-fill" :style="{ width: `${Math.max(8, step.conversion * 100)}%` }"></div>
        </div>
        <div class="funnel-stats">
          <span>{{ step.sessions }} {{ app.t("sessions") }}</span>
          <span>{{ app.formatPercent(step.conversion) }}</span>
          <span>{{ app.t("dropOff") }} · {{ app.formatNumber(step.drop_off_count || 0) }}</span>
          <span>{{ app.t("dropOffRate") }} · {{ app.formatPercent(step.drop_off_rate || 0) }}</span>
        </div>
      </div>
    </div>
    <p v-else class="empty-note">{{ app.t("noData") }}</p>
  </article>
</template>

<script setup>
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>
