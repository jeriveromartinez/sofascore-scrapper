<script setup lang="ts">
import { computed, ref } from "vue";

const props = withDefaults(
  defineProps<{
    src?: string;
    alt: string;
    size?: number;
  }>(),
  { size: 30 },
);

const failed = ref(false);

function handleError(): void {
  failed.value = true;
}

const initials = computed(() => {
  const words = props.alt.trim().split(/\s+/).filter((w) => w.length > 0);
  if (words.length === 0) return "?";
  const first = words[0] ?? "";
  if (words.length === 1) return first.slice(0, 2).toUpperCase();
  const last = words[words.length - 1] ?? "";
  return ((first[0] ?? "") + (last[0] ?? "")).toUpperCase();
});

const style = computed(() => ({
  width: `${props.size}px`,
  height: `${props.size}px`,
  fontSize: `${Math.max(10, Math.floor(props.size * 0.4))}px`,
}));
</script>

<template>
  <span class="team-badge-wrapper">
    <img
      v-if="!failed && src"
      :src="src"
      :alt="alt"
      :width="size"
      :height="size"
      style="object-fit: contain"
      class="team-badge-img"
      @error="handleError"
    />
    <span
      v-else
      class="team-badge-fallback d-inline-flex align-items-center justify-content-center"
      :style="style"
      :aria-label="alt"
    >
      {{ initials }}
    </span>
  </span>
</template>

<style scoped>
.team-badge-wrapper {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.team-badge-fallback {
  border-radius: 50%;
  background-color: #e9ecef;
  color: #495057;
  font-weight: 600;
  line-height: 1;
}
</style>
