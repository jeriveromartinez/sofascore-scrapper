<script setup lang="ts">
import { reactive, onMounted, computed } from "vue";
import { deviceTournamentsApiService, tournamentsApiService } from "../store/services";
import type { DeviceTournament, Tournament, Device } from "../store/services/models";

const state = reactive({
  deviceTournaments: [] as DeviceTournament[],
  tournaments: [] as Tournament[],
  devices: [] as Device[],
  loading: false,
  error: "",
  success: "",
  selectedDeviceId: null as number | null,
  selectedTournamentIds: [] as number[],
});

async function loadData(): Promise<void> {
  state.loading = true;
  state.error = "";
  try {
    const [deviceTournaments, tournaments] = await Promise.all([
      deviceTournamentsApiService.getAllDeviceTournaments(),
      tournamentsApiService.getAllTournaments(),
    ]);
    state.deviceTournaments = deviceTournaments;
    state.tournaments = tournaments;

    // Extract unique devices from device tournaments
    const deviceMap = new Map<number, Device>();
    deviceTournaments.forEach(dt => {
      if (dt.Device && !deviceMap.has(dt.Device.ID)) {
        deviceMap.set(dt.Device.ID, dt.Device);
      }
    });
    state.devices = Array.from(deviceMap.values());
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudieron cargar los datos";
  } finally {
    state.loading = false;
  }
}

const deviceTournamentsByDevice = computed(() => {
  const grouped = new Map<number, DeviceTournament[]>();
  state.deviceTournaments.forEach(dt => {
    const list = grouped.get(dt.DeviceID) || [];
    list.push(dt);
    grouped.set(dt.DeviceID, list);
  });
  return grouped;
});

function selectDevice(deviceId: number): void {
  state.selectedDeviceId = deviceId;
  const deviceTournaments = deviceTournamentsByDevice.value.get(deviceId) || [];
  state.selectedTournamentIds = deviceTournaments.map(dt => dt.TournamentID);
  state.success = "";
  state.error = "";
}

async function saveDeviceTournaments(): Promise<void> {
  if (!state.selectedDeviceId) return;

  state.loading = true;
  state.error = "";
  state.success = "";
  try {
    await deviceTournamentsApiService.setDeviceTournaments(state.selectedDeviceId, {
      tournament_ids: state.selectedTournamentIds,
    });
    state.success = "Torneos del dispositivo actualizados correctamente";
    await loadData();
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudieron actualizar los torneos del dispositivo";
  } finally {
    state.loading = false;
  }
}

function toggleTournament(tournamentId: number): void {
  const index = state.selectedTournamentIds.indexOf(tournamentId);
  if (index > -1) {
    state.selectedTournamentIds.splice(index, 1);
  } else {
    state.selectedTournamentIds.push(tournamentId);
  }
}

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Gestión de Torneos por Dispositivo</h5>
      <small class="text-body-secondary">M2M /api/v1/device-tournaments</small>
    </div>

    <div class="card-body">
      <div v-if="state.error" class="alert alert-danger">
        {{ state.error }}
      </div>
      <div v-if="state.success" class="alert alert-success">
        {{ state.success }}
      </div>

      <div v-if="state.loading" class="text-center">
        <div class="spinner-border" role="status">
          <span class="visually-hidden">Cargando...</span>
        </div>
      </div>

      <div v-else class="row">
        <div class="col-md-4">
          <h6>Dispositivos</h6>
          <div class="list-group">
            <button
              v-for="device in state.devices"
              :key="device.ID"
              type="button"
              class="list-group-item list-group-item-action"
              :class="{ active: state.selectedDeviceId === device.ID }"
              @click="selectDevice(device.ID)"
            >
              <div class="d-flex w-100 justify-content-between">
                <h6 class="mb-1">{{ device.Name || device.Token }}</h6>
                <small>ID: {{ device.ID }}</small>
              </div>
              <small>{{ device.Platform }}</small>
            </button>
          </div>
          <p v-if="state.devices.length === 0" class="text-muted mt-3">
            No hay dispositivos con torneos asignados
          </p>
        </div>

        <div class="col-md-8">
          <div v-if="state.selectedDeviceId">
            <h6>Torneos del Dispositivo</h6>
            <div class="mb-3">
              <div
                v-for="tournament in state.tournaments"
                :key="tournament.ID"
                class="form-check"
              >
                <input
                  :id="`tournament-${tournament.ID}`"
                  type="checkbox"
                  class="form-check-input"
                  :checked="state.selectedTournamentIds.includes(tournament.ID)"
                  @change="toggleTournament(tournament.ID)"
                />
                <label class="form-check-label" :for="`tournament-${tournament.ID}`">
                  {{ tournament.Name }} <small class="text-muted">({{ tournament.Slug }})</small>
                </label>
              </div>
            </div>
            <button
              class="btn btn-primary"
              :disabled="state.loading"
              @click="saveDeviceTournaments"
            >
              Guardar Cambios
            </button>
          </div>
          <div v-else class="text-center text-muted">
            Seleccione un dispositivo para gestionar sus torneos
          </div>
        </div>
      </div>

      <hr class="my-4" />

      <h6>Vista General</h6>
      <div v-if="state.deviceTournaments.length > 0" class="table-responsive">
        <table class="table table-sm table-striped">
          <thead>
            <tr>
              <th>Dispositivo</th>
              <th>Torneo</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="dt in state.deviceTournaments" :key="`${dt.DeviceID}-${dt.TournamentID}`">
              <td>{{ dt.Device?.Name || dt.Device?.Token || `ID: ${dt.DeviceID}` }}</td>
              <td>{{ dt.Tournament?.Name || `ID: ${dt.TournamentID}` }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p v-else class="text-muted">
        No hay relaciones dispositivo-torneo configuradas
      </p>
    </div>
  </div>
</template>
