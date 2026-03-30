<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { playbackApiService } from "../store/services";
import type { PlaybackLog } from "../store/services/models";

const state = reactive({
  page: 1,
  limit: 10,
  total: 0,
  data: [] as PlaybackLog[],
  loading: false,
  error: "",
});

function parseUnix(value: number): string {
  return new Date(value).toLocaleString("en-UK");
}

async function load() {
  state.error = "";
  state.data = [];
  state.loading = true;

  try {
    const { list, total } = await playbackApiService.getPlayingNow(
      state.page,
      state.limit,
    );
    state.data = list;
    state.total = total;
  } catch (error) {
    console.error("Error fetching playback data:", error);
    state.error =
      "Ocurrió un error al cargar los datos. Por favor, inténtalo de nuevo.";
  } finally {
    state.loading = false;
  }
}

function nextPage(): void {
  if (!state.data) return;
  if (state.page >= Math.ceil(state.total / state.limit)) return;
  state.page += 1;
  void load();
}

function prevPage(): void {
  if (state.page <= 1) return;
  state.page -= 1;
  void load();
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
      <div class="d-flex gap-2">
        <input
          v-model.number="state.limit"
          type="number"
          min="1"
          class="form-control"
          style="width: 120px"
        />
        <button class="btn btn-primary" :disabled="state.loading" @click="load">
          Consultar
        </button>
      </div>
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
              <td>{{ parseUnix(row.startedAt) }}</td>
              <td>{{ row.endedAt > 0 ? parseUnix(row.endedAt) : "" }}</td>
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
            @click="prevPage"
            :disabled="state.page <= 1 || state.loading"
          >
            <span class="d-none d-sm-inline">Anterior</span>
            <span class="d-inline d-sm-none">&lt;</span>
          </button>
          <button
            class="btn btn-outline-secondary btn-sm"
            @click="nextPage"
            :disabled="
              state.loading ||
              state.page >= Math.ceil(state.total / state.limit)
            "
          >
            <span class="d-none d-sm-inline">Siguiente</span>
            <span class="d-inline d-sm-none">&gt;</span>
          </button>
        </div>
        <span class="text-body-secondary small">
          Pág. {{ state.page }} / {{ Math.ceil(state.total / state.limit) }} ({{
            state.total
          }})
        </span>
      </div>
    </div>
  </div>
</template>
