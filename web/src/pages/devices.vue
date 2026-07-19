<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { devicesApiService } from "../store/services";
import type { Device } from "../store/services/models";
import { formatUnixTimestamp } from "../utils/time";

const PAGE_LIMIT = 20;

const state = reactive({
  devices: [] as Device[],
  loading: false,
  error: "",
  currentCursor: "" as string,
  nextCursor: "" as string,
  prevCursors: [] as string[],
  hasNext: false,
  hasPrev: false,
});

async function loadPage(cursor?: string): Promise<void> {
  state.loading = true;
  state.error = "";

  try {
    const page = await devicesApiService.getDevicePage(cursor, PAGE_LIMIT);
    state.devices = page.data;
    state.nextCursor = page.page?.nextCursor ?? "";
    state.hasNext = page.page?.hasMore ?? false;
    state.currentCursor = cursor ?? "";
    state.hasPrev = state.prevCursors.length > 0;
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "Error cargando dispositivos";
  } finally {
    state.loading = false;
  }
}

async function goNext(): Promise<void> {
  if (!state.hasNext || !state.nextCursor) return;
  state.prevCursors = [...state.prevCursors, state.currentCursor];
  await loadPage(state.nextCursor);
}

async function goPrev(): Promise<void> {
  if (state.prevCursors.length === 0) return;
  const prev = state.prevCursors[state.prevCursors.length - 1];
  state.prevCursors = state.prevCursors.slice(0, -1);
  await loadPage(prev || undefined);
}

onMounted(() => {
  void loadPage();
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

      <div v-if="state.devices.length > 0" class="table-responsive">
        <table class="table table-sm table-striped align-middle">
          <thead>
            <tr>
              <th>ID</th>
              <th class="text-truncate" style="max-width: 150px">Token</th>
              <th class="d-none d-md-table-cell">Platform</th>
              <th>Name</th>
              <th>Version</th>
              <th class="d-none d-lg-table-cell">Last Seen</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="device in state.devices" :key="device.id">
              <td>{{ device.id }}</td>
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
              <td class="d-none d-lg-table-cell">{{ formatUnixTimestamp(device.lastSeen) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div
        v-else-if="!state.loading"
        class="text-center text-muted"
      >
        No hay dispositivos registrados
      </div>

      <div
        v-if="state.devices.length > 0"
        class="d-flex gap-2 mt-3 align-items-center justify-content-between"
      >
        <div class="d-flex gap-2">
          <button
            class="btn btn-outline-secondary btn-sm"
            :disabled="!state.hasPrev || state.loading"
            @click="goPrev"
          >
            <span class="d-none d-sm-inline">Anterior</span>
            <span class="d-inline d-sm-none">&lt;</span>
          </button>
          <button
            class="btn btn-outline-secondary btn-sm"
            :disabled="!state.hasNext || state.loading"
            @click="goNext"
          >
            <span class="d-none d-sm-inline">Siguiente</span>
            <span class="d-inline d-sm-none">&gt;</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>