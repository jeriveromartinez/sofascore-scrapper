<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { devicesApiService } from "../store/services";
import type { DeviceResponse } from "../store/services/models";

const state = reactive({
  loading: false,
  error: "",
  success: "",
  result: {} as DeviceResponse,
});

async function loadDevices(): Promise<void> {
  state.loading = true;
  state.error = "";

  try {
    state.result = await devicesApiService.getDevices({
      page: state.result.page || 1,
      limit: state.result.limit || 10,
    });
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "Error cargando dispositivos";
  } finally {
    state.loading = false;
  }
}

function nextPage(): void {
  if (!state.result) return;
  if (state.result.page >= state.result.totalPages) return;
  state.result.page += 1;
  void loadDevices();
}

function prevPage(): void {
  if (!state.result) return;
  if (state.result.page <= 1) return;
  state.result.page -= 1;
  void loadDevices();
}

onMounted(() => {
  void loadDevices();
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Dispositivos</h5>
    </div>

    <div class="card-body">
      <div v-if="state.error" class="alert alert-danger">{{ state.error }}</div>
      <div v-if="state.loading" class="alert alert-info">
        Cargando dispositivos...
      </div>

      <div v-if="state.result" class="table-responsive">
        <table class="table table-sm table-striped align-middle">
          <thead>
            <tr>
              <th class="text-truncate" style="max-width: 150px">Token</th>
              <th class="d-none d-md-table-cell">Platform</th>
              <th>Name</th>
              <th>Version</th>
              <th class="d-none d-lg-table-cell">Last Seen</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="device in state.result.data" :key="device.id">
              <td
                class="text-truncate"
                style="max-width: 150px"
                :title="device.token"
              >
                {{ device.token }}
              </td>
              <td class="d-none d-md-table-cell">{{ device.platform }}</td>
              <td>{{ device.name }}</td>
              <td>{{ device.version }}</td>
              <td class="d-none d-lg-table-cell">{{ device.lastSeen }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div
        v-if="state.result"
        class="d-flex flex-wrap gap-2 mt-3 align-items-center justify-content-between"
      >
        <div class="d-flex gap-2">
          <button
            class="btn btn-outline-secondary btn-sm"
            @click="prevPage"
            :disabled="state.result.page <= 1 || state.loading"
          >
            <span class="d-none d-sm-inline">Anterior</span>
            <span class="d-inline d-sm-none">&lt;</span>
          </button>
          <button
            class="btn btn-outline-secondary btn-sm"
            @click="nextPage"
            :disabled="
              state.loading || state.result.page >= state.result.totalPages
            "
          >
            <span class="d-none d-sm-inline">Siguiente</span>
            <span class="d-inline d-sm-none">&gt;</span>
          </button>
        </div>
        <span class="text-body-secondary small">
          Pág. {{ state.result.page }} / {{ state.result.totalPages }} ({{
            state.result.total
          }})
        </span>
      </div>
    </div>
  </div>
</template>
