<template>
  <button :class="['status-toggle', { on: modelValue, off: !modelValue }]" @click="$emit('update:modelValue', !modelValue)">
    {{ modelValue ? 'ON' : 'OFF' }}
  </button>
</template>

<script setup lang="ts">
defineProps<{ modelValue: boolean }>()
defineEmits<{ (e: 'update:modelValue', v: boolean): void }>()
</script>
