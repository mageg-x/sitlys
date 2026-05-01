<template>
  <label class="locale-switch">
    <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/></svg>
    <select :value="modelValue" @change="$emit('update:modelValue', $event.target.value)">
      <option value="zh-CN">{{ t("zh") }}</option>
      <option value="en-US">{{ t("en") }}</option>
    </select>
  </label>
</template>

<script setup>
defineProps({
  modelValue: { type: String, required: true },
  t: { type: Function, required: true },
});

defineEmits(["update:modelValue"]);
</script>
