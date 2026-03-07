<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { statsApiService } from "../store/services";
import type { EventStats } from "../store/services/models";

const state = reactive({
  limit: 10,
  loading: false,
  error: "",
  data: [] as EventStats[],
});

async function load(): Promise<void> {
  state.loading = true;
  state.error = "";

  try {
    state.data = await statsApiService.getTopEvents(state.limit);
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudieron cargar las estadisticas";
  } finally {
    state.loading = false;
  }
}

onMounted(() => {
  void load();
});
</script>

<template>
  <div class="card">
    <div
      class="card-header d-flex flex-wrap gap-2 justify-content-between align-items-center"
    >
      <div>
        <h5 class="mb-0">Top Eventos</h5>
        <small class="text-body-secondary">GET /api/v1/stats/top-events</small>
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
              <th>SofaScore Event ID</th>
              <th>Views</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, index) in state.data" :key="row.SofaScoreEventId">
              <td>{{ index + 1 }}</td>
              <td>{{ row.SofaScoreEventId }}</td>
              <td>{{ row.ViewCount }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="text-body-secondary mb-0" v-else-if="!state.loading">
        Sin resultados.
      </p>
    </div>
  </div>
</template>
