<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { eventsApiService } from "../store/services";
import type { SofaScoreEvent } from "../store/services/models";
import { formatUnixTimestamp } from "../utils/time";

const PAGE_LIMIT = 20;

const state = reactive({
  events: [] as SofaScoreEvent[],
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
    const page = await eventsApiService.getEventPage(cursor, PAGE_LIMIT);
    state.events = page.data;
    state.nextCursor = page.page?.nextCursor ?? "";
    state.hasNext = page.page?.hasMore ?? false;
    state.currentCursor = cursor ?? "";
    state.hasPrev = state.prevCursors.length > 0;
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "Error cargando eventos";
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

onMounted(() => loadPage());
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
        @click="loadPage(state.currentCursor || undefined)"
      >
        Recargar
      </button>
    </div>

    <div class="card-body">
      <div v-if="state.error" class="alert alert-danger">{{ state.error }}</div>
      <div v-if="state.loading" class="alert alert-info">
        Cargando eventos...
      </div>

      <div v-if="state.events.length > 0" class="table-responsive">
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
            <tr v-for="event in state.events" :key="event.id">
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
                {{ formatUnixTimestamp(event.startTimestamp) }}
              </td>
            </tr>
          </tbody>
        </table>

        <div class="d-flex justify-content-between align-items-center mt-3">
          <button
            class="btn btn-outline-secondary btn-sm"
            :disabled="!state.hasPrev || state.loading"
            @click="goPrev"
          >
            Anterior
          </button>
          <button
            class="btn btn-outline-secondary btn-sm"
            :disabled="!state.hasNext || state.loading"
            @click="goNext"
          >
            Siguiente
          </button>
        </div>
      </div>

      <div v-else-if="!state.loading" class="text-center text-muted">
        No hay eventos disponibles
      </div>
    </div>
  </div>
</template>