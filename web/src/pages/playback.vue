<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { playbackApiService } from "../store/services";
import type { PlaybackLog } from "../store/services/models";
import { formatUnixTimestamp } from "../utils/time";

const PAGE_LIMIT = 20;

const state = reactive({
  data: [] as PlaybackLog[],
  loading: false,
  error: "",
  currentCursor: "" as string,
  nextCursor: "" as string,
  prevCursors: [] as string[],
  hasNext: false,
  hasPrev: false,
});

async function load(cursor?: string) {
  state.error = "";
  state.data = [];
  state.loading = true;

  try {
    const page = await playbackApiService.getPlaybackPage(cursor, PAGE_LIMIT);
    state.data = page.data;
    state.nextCursor = page.page?.nextCursor ?? "";
    state.hasNext = page.page?.hasMore ?? false;
    state.currentCursor = cursor ?? "";
    state.hasPrev = state.prevCursors.length > 0;
  } catch (error) {
    console.error("Error fetching playback data:", error);
    state.error =
      "Ocurrió un error al cargar los datos. Por favor, inténtalo de nuevo.";
  } finally {
    state.loading = false;
  }
}

async function goNext(): Promise<void> {
  if (!state.hasNext || !state.nextCursor) return;
  state.prevCursors = [...state.prevCursors, state.currentCursor];
  await load(state.nextCursor);
}

async function goPrev(): Promise<void> {
  if (state.prevCursors.length === 0) return;
  const prev = state.prevCursors[state.prevCursors.length - 1];
  state.prevCursors = state.prevCursors.slice(0, -1);
  await load(prev || undefined);
}

onMounted(() => {
  load();
});
</script>

<template>
  <div class="card">
    <div
      class="card-header d-flex flex-wrap gap-2 justify-content-between align-items-center"
    >
      <div>
        <h5 class="mb-0">Playing Now</h5>
      </div>
      <button class="btn btn-primary" :disabled="state.loading" @click="load(state.currentCursor || undefined)">
        Consultar
      </button>
    </div>

    <div class="card-body">
      <div v-if="state.error" class="alert alert-danger">{{ state.error }}</div>
      <div v-if="state.loading" class="alert alert-info">
        Cargando estadisticas...
      </div>

      <div class="table-responsive text-nowrap" v-if="state.data.length">
        <table class="table table-striped">
          <thead>
            <tr>
              <th>#</th>
              <th>Content</th>
              <th>Started At</th>
              <th>Ended At</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, index) in state.data" :key="row.id">
              <td>{{ index + 1 }}</td>
              <td>{{ row.content }}</td>
              <td>{{ formatUnixTimestamp(row.startedAt) }}</td>
              <td>{{ row.endedAt > 0 ? formatUnixTimestamp(row.endedAt) : "" }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="text-body-secondary mb-0" v-else-if="!state.loading">
        Sin resultados.
      </p>

      <div
        v-if="state.data.length"
        class="d-flex flex-wrap gap-2 mt-3 align-items-center justify-content-between"
      >
        <div class="d-flex gap-2">
          <button
            class="btn btn-outline-secondary btn-sm"
            @click="goPrev"
            :disabled="!state.hasPrev || state.loading"
          >
            Anterior
          </button>
          <button
            class="btn btn-outline-secondary btn-sm"
            @click="goNext"
            :disabled="!state.hasNext || state.loading"
          >
            Siguiente
          </button>
        </div>
      </div>
    </div>
  </div>
</template>