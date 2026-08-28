<script setup lang="ts" generic="T">
import type { CursorPaginationState } from "../composables/useCursorPagination";

// PaginationControls renders the standard prev / next / (optional)
// reload triplet shared by every paginated admin page. The previous
// shape was three <button> elements duplicated in events.vue,
// users.vue, tournaments.vue, domains.vue, devices.vue, playback.vue,
// and apkAdmin.vue — ~150 LOC of repetition.
//
// The parent owns the pagination state via useCursorPagination and
// passes it in; the SFC only knows how to render the buttons and
// emit the navigation events. A single prop (`with-reload`) toggles
// the reload button so pages that do not need it (e.g. the modal-form
// pages that reload on submit themselves) can opt out.

const { state, withReload = false } = defineProps<{
  state: CursorPaginationState<T>;
  withReload?: boolean;
}>();

const emit = defineEmits<{
  prev: [];
  next: [];
  reload: [];
}>();

function onPrev() {
  emit("prev");
}

function onNext() {
  emit("next");
}

function onReload() {
  emit("reload");
}
</script>

<template>
  <div
    class="d-flex gap-2 mt-3 align-items-center justify-content-between"
    data-test="pagination-controls"
  >
    <div class="d-flex gap-2">
      <button
        type="button"
        class="btn btn-outline-secondary btn-sm"
        :disabled="!state.hasPrev || state.loading"
        aria-label="Previous page"
        data-test="pagination-prev"
        @click="onPrev"
      >
        <span class="d-none d-sm-inline">Anterior</span>
        <span class="d-inline d-sm-none" aria-hidden="true">&lt;</span>
      </button>
      <button
        type="button"
        class="btn btn-outline-secondary btn-sm"
        :disabled="!state.hasNext || state.loading"
        aria-label="Next page"
        data-test="pagination-next"
        @click="onNext"
      >
        <span class="d-none d-sm-inline">Siguiente</span>
        <span class="d-inline d-sm-none" aria-hidden="true">&gt;</span>
      </button>
    </div>
    <button
      v-if="withReload"
      type="button"
      class="btn btn-outline-primary btn-sm"
      :disabled="state.loading"
      aria-label="Reload data"
      data-test="pagination-reload"
      @click="onReload"
    >
      Recargar
    </button>
  </div>
</template>
