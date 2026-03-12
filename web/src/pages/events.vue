<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { eventsApiService } from "../store/services";
import type { EventsResponse } from "../store/services/models";

type EventState = {
  date: string;
  sport: string;
  page: number;
  limit: number;
  loading: boolean;
  error: string;
  data: EventsResponse | null;
};

const state = reactive<EventState>({
  date: "",
  sport: "",
  page: 1,
  limit: 10,
  loading: false,
  error: "",
  data: null,
});

function formatTimestamp(unix: number): string {
  if (!unix) return "-";
  return new Date(unix * 1000).toLocaleString();
}

async function fetchEvents(): Promise<void> {
  state.loading = true;
  state.error = "";

  try {
    state.data = await eventsApiService.getEvents({
      date: state.date || undefined,
      sport: state.sport || undefined,
      page: state.page,
      limit: state.limit,
    });
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "Error cargando eventos";
  } finally {
    state.loading = false;
  }
}

function nextPage(): void {
  if (!state.data) return;
  if (state.page >= state.data.total_pages) return;
  state.page += 1;
  void fetchEvents();
}

function prevPage(): void {
  if (state.page <= 1) return;
  state.page -= 1;
  void fetchEvents();
}

function applyFilters(): void {
  state.page = 1;
  void fetchEvents();
}

onMounted(() => fetchEvents());
</script>

<template>
  <div class="card">
    <div
      class="card-header d-flex flex-wrap gap-2 justify-content-between align-items-center"
    >
      <div>
        <h5 class="mb-0">Eventos</h5>
      </div>
      <button
        class="btn btn-outline-primary"
        :disabled="state.loading"
        @click="fetchEvents"
      >
        Recargar
      </button>
    </div>

    <div class="card-body">
      <form class="row g-3 mb-4" @submit.prevent="applyFilters">
        <div class="col-md-3">
          <label class="form-label">Fecha</label>
          <input v-model="state.date" type="date" class="form-control" />
        </div>
        <div class="col-md-3">
          <label class="form-label">Sport</label>
          <input
            v-model="state.sport"
            type="text"
            class="form-control"
            placeholder="football"
          />
        </div>
        <div class="col-md-2">
          <label class="form-label">Page</label>
          <input
            v-model.number="state.page"
            type="number"
            min="1"
            class="form-control"
          />
        </div>
        <div class="col-md-2">
          <label class="form-label">Limit</label>
          <input
            v-model.number="state.limit"
            type="number"
            min="1"
            max="100"
            class="form-control"
          />
        </div>
        <div class="col-md-2 d-flex align-items-end">
          <button
            class="btn btn-primary w-100"
            type="submit"
            :disabled="state.loading"
          >
            Buscar
          </button>
        </div>
      </form>

      <div v-if="state.error" class="alert alert-danger">{{ state.error }}</div>
      <div v-if="state.loading" class="alert alert-info">
        Cargando eventos...
      </div>

      <div v-if="state.data" class="table-responsive text-nowrap">
        <table class="table table-sm table-striped align-middle">
          <thead>
            <tr>
              <th>Event ID</th>
              <th>League</th>
              <th>Sport</th>
              <th>Partido</th>
              <th>Score</th>
              <th>Inicio</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="event in state.data.data" :key="event.ID">
              <td>{{ event.SofaScoreEventId }}</td>
              <td>{{ event.league.name }}</td>
              <td>{{ event.Sport }}</td>
              <td>
                <img
                  :src="event.teamHome.LogoUrl"
                  :alt="event.HomeTeam"
                  class="me-2"
                  width="40px"
                />
                {{ event.HomeTeam }} vs {{ event.AwayTeam }}
                <img
                  :src="event.teamAway.LogoUrl"
                  :alt="event.AwayTeam"
                  class="me-2"
                  width="40px"
                />
              </td>
              <td>{{ event.HomeScore }} - {{ event.AwayScore }}</td>
              <td>{{ formatTimestamp(event.StartTimestamp) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="state.data" class="d-flex gap-2 mt-3 align-items-center">
        <button
          class="btn btn-outline-secondary btn-sm"
          @click="prevPage"
          :disabled="state.page <= 1 || state.loading"
        >
          Anterior
        </button>
        <button
          class="btn btn-outline-secondary btn-sm"
          @click="nextPage"
          :disabled="state.loading || state.page >= state.data.total_pages"
        >
          Siguiente
        </button>
        <span class="text-body-secondary small">
          Pagina {{ state.page }} / {{ state.data.total_pages }} - Total
          {{ state.data.total }}
        </span>
      </div>
    </div>
  </div>
</template>
