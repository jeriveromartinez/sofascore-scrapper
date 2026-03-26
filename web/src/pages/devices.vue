<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { devicesApiService } from "../store/services";
import type { DeviceResponse } from "../store/services/models";

const state = reactive({
  token: "",
  platform: "android",
  name: "",
  loading: false,
  error: "",
  success: "",
  result: {} as DeviceResponse,
});

async function fetchEvents(): Promise<void> {
  state.loading = true;
  state.error = "";

  try {
    state.result = await devicesApiService.getDevices({
      page: state.result.page || 1,
      limit: state.result.limit || 10,
    });
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "Error cargando eventos";
  } finally {
    state.loading = false;
  }
}

async function submit(): Promise<void> {
  state.loading = true;
  state.error = "";
  state.success = "";

  try {
    await devicesApiService.registerDevice({
      token: state.token,
      platform: state.platform,
      name: state.name,
    });
    state.success = "Dispositivo registrado/actualizado";
    await fetchEvents();
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudo registrar el dispositivo";
  } finally {
    state.loading = false;
  }
}

function nextPage(): void {
  if (!state.result) return;
  if (state.result.page >= state.result.totalPages) return;
  state.result.page += 1;
  void fetchEvents();
}

function prevPage(): void {
  if (!state.result) return;
  if (state.result.page <= 1) return;
  state.result.page -= 1;
  void fetchEvents();
}

onMounted(() => {
  void fetchEvents();
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Dispositivos</h5>
    </div>

    <div class="card-body">
      <form class="row g-3 mb-4" @submit.prevent="submit">
        <div class="col-12 col-md-6 col-lg-5">
          <label class="form-label">Token *</label>
          <input
            v-model="state.token"
            type="text"
            class="form-control"
            required
          />
        </div>
        <div class="col-6 col-md-3 col-lg-3">
          <label class="form-label">Plataforma</label>
          <input v-model="state.platform" type="text" class="form-control" />
        </div>
        <div class="col-6 col-md-3 col-lg-4">
          <label class="form-label">Nombre</label>
          <input v-model="state.name" type="text" class="form-control" />
        </div>

        <div class="col-12">
          <button class="btn btn-primary" :disabled="state.loading">
            Guardar dispositivo
          </button>
        </div>
      </form>

      <div v-if="state.error" class="alert alert-danger">{{ state.error }}</div>
      <div v-if="state.loading" class="alert alert-info">
        Cargando eventos...
      </div>

      <div v-if="state.result" class="table-responsive">
        <table class="table table-sm table-striped align-middle">
          <thead>
            <tr>
              <th class="text-truncate" style="max-width: 150px">Token</th>
              <th class="d-none d-md-table-cell">Platform</th>
              <th>Name</th>
              <th class="d-none d-lg-table-cell">Last Seen</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="device in state.result.data" :key="device.id">
              <td class="text-truncate" style="max-width: 150px" :title="device.token">
                {{ device.token }}
              </td>
              <td class="d-none d-md-table-cell">{{ device.platform }}</td>
              <td>{{ device.name }}</td>
              <td class="d-none d-lg-table-cell">{{ device.lastSeen }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="state.result" class="d-flex flex-wrap gap-2 mt-3 align-items-center justify-content-between">
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
          Pág. {{ state.result.page }} / {{ state.result.totalPages }} ({{ state.result.total }})
        </span>
      </div>
    </div>
  </div>
</template>
