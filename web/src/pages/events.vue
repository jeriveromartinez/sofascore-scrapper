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
  if (state.page >= state.data.totalPages) return;
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
        <div class="col-12 col-md-6 col-lg-3">
          <label class="form-label">Fecha</label>
          <input v-model="state.date" type="date" class="form-control" />
        </div>
        <div class="col-12 col-md-6 col-lg-3">
          <label class="form-label">Sport</label>
          <input
            v-model="state.sport"
            type="text"
            class="form-control"
            placeholder="football"
          />
        </div>
        <div class="col-6 col-md-4 col-lg-2">
          <label class="form-label">Page</label>
          <input
            v-model.number="state.page"
            type="number"
            min="1"
            class="form-control"
          />
        </div>
        <div class="col-6 col-md-4 col-lg-2">
          <label class="form-label">Limit</label>
          <input
            v-model.number="state.limit"
            type="number"
            min="1"
            max="100"
            class="form-control"
          />
        </div>
        <div class="col-12 col-md-4 col-lg-2 d-flex align-items-end">
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

      <div v-if="state.data" class="table-responsive">
        <table class="table table-sm table-striped align-middle">
          <thead>
            <tr>
              <th class="d-none d-md-table-cell">Event ID</th>
              <th class="d-none d-lg-table-cell">League</th>
              <th class="d-none d-lg-table-cell">Sport</th>
              <th>Partido</th>
              <th>Score</th>
              <th class="d-none d-md-table-cell">Inicio</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="event in state.data.data" :key="event.id">
              <td class="d-none d-md-table-cell">
                {{ event.sofaScoreEventId }}
              </td>
              <td class="d-none d-lg-table-cell">
                {{ event.league?.name || "-" }}
              </td>
              <td class="d-none d-lg-table-cell">{{ event.sport }}</td>
              <td>
                <div class="d-flex align-items-center gap-2 flex-wrap">
                  <img
                    :src="event.teamHome?.logoUrl"
                    :alt="event.teamHome?.name ?? 'Home Team'"
                    class="me-1"
                    width="30px"
                    height="30px"
                    style="object-fit: contain"
                  />
                  <span class="text-nowrap">{{
                    event.teamHome?.name ?? "Home"
                  }}</span>
                  <span class="mx-1">vs</span>
                  <span class="text-nowrap">{{
                    event.teamAway?.name ?? "Away"
                  }}</span>
                  <img
                    :src="event.teamAway?.logoUrl"
                    :alt="event.teamAway?.name ?? 'Away Team'"
                    class="ms-1"
                    width="30px"
                    height="30px"
                    style="object-fit: contain"
                  />
                </div>
                <small class="d-md-none text-body-secondary d-block mt-1">
                  {{ event.league?.name || "-" }} | {{ event.sport }}
                </small>
              </td>
              <td class="text-center">
                <strong>{{ event.homeScore }} - {{ event.awayScore }}</strong>
              </td>
              <td class="d-none d-md-table-cell">
                {{ formatTimestamp(event.startTimestamp) }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div
        v-if="state.data"
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
            :disabled="state.loading || state.page >= state.data.totalPages"
          >
            <span class="d-none d-sm-inline">Siguiente</span>
            <span class="d-inline d-sm-none">&gt;</span>
          </button>
        </div>
        <span class="text-body-secondary small">
          Pág. {{ state.page }} / {{ state.data.totalPages }} ({{
            state.data.total
          }})
        </span>
      </div>
    </div>
  </div>
</template>
