<script setup lang="ts">
import { reactive, onMounted } from "vue";
import { globalConfigApiService, tournamentsApiService } from "../store/services";
import type { GlobalTournamentConfig, Tournament } from "../store/services/models";

const state = reactive({
  globalConfig: [] as GlobalTournamentConfig[],
  tournaments: [] as Tournament[],
  loading: false,
  error: "",
  success: "",
  selectedTournamentIds: [] as number[],
});

async function loadData(): Promise<void> {
  state.loading = true;
  state.error = "";
  try {
    const [globalConfig, tournaments] = await Promise.all([
      globalConfigApiService.getGlobalConfig(),
      tournamentsApiService.getAllTournaments(),
    ]);
    state.globalConfig = globalConfig;
    state.tournaments = tournaments;
    state.selectedTournamentIds = globalConfig.map(gc => gc.TournamentID);
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudieron cargar los datos";
  } finally {
    state.loading = false;
  }
}

async function saveGlobalConfig(): Promise<void> {
  state.loading = true;
  state.error = "";
  state.success = "";
  try {
    await globalConfigApiService.setGlobalConfig({
      tournament_ids: state.selectedTournamentIds,
    });
    state.success = "Configuración global actualizada correctamente";
    await loadData();
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudo actualizar la configuración global";
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
      <h5 class="mb-0">Configuración Global de Torneos</h5>
      <small class="text-body-secondary">/api/v1/global-tournament-config</small>
    </div>

    <div class="card-body">
      <div class="alert alert-info">
        <strong>Nota:</strong> Los torneos seleccionados aquí serán visibles para los dispositivos
        que no tengan torneos específicos asignados.
      </div>

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

      <div v-else>
        <h6 class="mb-3">Seleccione los torneos globales</h6>
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
              <strong>{{ tournament.Name }}</strong>
              <small class="text-muted ms-2">({{ tournament.Slug }})</small>
            </label>
          </div>
        </div>

        <button
          class="btn btn-primary"
          :disabled="state.loading"
          @click="saveGlobalConfig"
        >
          Guardar Configuración
        </button>

        <hr class="my-4" />

        <h6>Torneos Configurados Actualmente</h6>
        <div v-if="state.globalConfig.length > 0" class="list-group">
          <div
            v-for="config in state.globalConfig"
            :key="config.ID"
            class="list-group-item"
          >
            <div class="d-flex w-100 justify-content-between">
              <h6 class="mb-1">{{ config.Tournament?.Name || `ID: ${config.TournamentID}` }}</h6>
              <small>{{ config.Tournament?.Slug }}</small>
            </div>
          </div>
        </div>
        <p v-else class="text-muted">
          No hay torneos en la configuración global. Los dispositivos sin torneos asignados no verán ningún torneo.
        </p>
      </div>
    </div>
  </div>
</template>
