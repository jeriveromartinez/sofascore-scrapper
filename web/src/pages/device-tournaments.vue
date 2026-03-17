<script setup lang="ts">
import { reactive, onMounted } from "vue";
import {
  deviceTournamentsApiService,
  tournamentsApiService,
  devicesApiService,
} from "../store/services";
import type {
  DeviceTournament,
  Tournament,
  Device,
} from "../store/services/models";

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
    const [tournaments, devices] = await Promise.all([
      tournamentsApiService.getAllTournaments(),
      devicesApiService.getAllDevices(),
    ]);
    state.devices = devices.data;
    state.tournaments = tournaments;
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudieron cargar los datos";
  } finally {
    state.loading = false;
  }
}

async function selectDevice(deviceId: number): Promise<void> {
  state.loading = true;
  state.selectedDeviceId = deviceId;
  const deviceTournaments = await deviceTournamentsApiService.getDeviceTournaments(state.selectedDeviceId);
  state.selectedTournamentIds = deviceTournaments.map((dt) => dt.tournament_id);
  state.success = "";
  state.error = "";
  state.loading = false;
}

async function saveDeviceTournaments(): Promise<void> {
  if (!state.selectedDeviceId) return;

  state.loading = true;
  state.error = "";
  state.success = "";
  try {
    await deviceTournamentsApiService.setDeviceTournaments(
      state.selectedDeviceId,
      { tournament_ids: state.selectedTournamentIds },
    );
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

function cancelChanges(): void {
    state.selectedTournamentIds = [];
    state.selectedDeviceId = null;
}

onMounted(() => {
  loadData();
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Gestión de Torneos por Dispositivo</h5>
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
                <label
                  class="form-check-label"
                  :for="`tournament-${tournament.ID}`"
                >
                  {{ tournament.name }}
                  <small class="text-muted">({{ tournament.slug }})</small>
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

            <button
              class="btn btn-warning ms-2"
              :disabled="state.loading"
              @click="cancelChanges"
            >
              Cancelar
            </button>
          </div>
          <div v-else class="text-center text-muted">
            Seleccione un dispositivo para gestionar sus torneos
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
