<template>
  <p v-if="!rows.length" class="empty-note">{{ emptyText }}</p>
  <div v-else class="bar-list">
    <div v-for="(item, index) in rows.slice(0, 8)" :key="`${item[labelKey]}-${index}`" class="bar-row">
      <div class="bar-caption">
        <strong>{{ item[labelKey] || "-" }}</strong>
        <span>{{ Number(item[valueKey] || 0).toLocaleString() }}</span>
      </div>
      <div class="bar-track">
        <div class="bar-fill" :style="{ width: `${Math.max(12, (Number(item[valueKey] || 0) / maxValue) * 100)}%` }"></div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from "vue";

const props = defineProps({
  rows: { type: Array, default: () => [] },
  labelKey: { type: String, required: true },
  valueKey: { type: String, required: true },
  emptyText: { type: String, default: "" },
});

const maxValue = computed(() => Math.max(...props.rows.map(item => Number(item[props.valueKey] || 0)), 1));
</script>

<style scoped>
.bar-list {
  display: grid;
  gap: 0.85rem;
  padding: 0.25rem 0;
}

.bar-row {
  display: grid;
  gap: 0.4rem;
}

.bar-caption {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
}

.bar-caption strong {
  font-size: 0.84rem;
  color: var(--ink-soft);
  font-weight: 600;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.bar-caption span {
  color: var(--muted);
  font-size: 0.8rem;
  font-weight: 500;
  font-variant-numeric: tabular-nums;
}

.bar-track {
  height: 8px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(226,232,240,0.9), rgba(241,245,249,0.95));
  overflow: hidden;
  position: relative;
  box-shadow: inset 0 1px 2px rgba(0, 0, 0, 0.04);
}

.bar-fill {
  height: 100%;
  border-radius: inherit;
  transition: width 0.6s cubic-bezier(0.4, 0, 0.2, 1);
  position: relative;
  background: linear-gradient(90deg, var(--accent), var(--accent-teal));
}

.bar-row:nth-child(2) .bar-fill { background: linear-gradient(90deg, var(--accent-teal), var(--accent-emerald)); }
.bar-row:nth-child(3) .bar-fill { background: linear-gradient(90deg, var(--accent-sky), var(--accent)); }
.bar-row:nth-child(4) .bar-fill { background: linear-gradient(90deg, var(--accent-warm), var(--accent-rose)); }
.bar-row:nth-child(5) .bar-fill { background: linear-gradient(90deg, var(--accent-violet), var(--accent-rose)); }
.bar-row:nth-child(6) .bar-fill { background: linear-gradient(90deg, var(--accent-emerald), var(--accent-teal)); }
.bar-row:nth-child(7) .bar-fill { background: linear-gradient(90deg, var(--accent-rose), var(--accent-warm)); }
.bar-row:nth-child(8) .bar-fill { background: linear-gradient(90deg, var(--accent), var(--accent-violet)); }

.bar-fill::after {
  content: '';
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background: linear-gradient(90deg, transparent 0%, rgba(255,255,255,0.25) 50%, transparent 100%);
  animation: barShimmer 2s ease-in-out infinite;
}

@keyframes barShimmer {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(100%); }
}
</style>
