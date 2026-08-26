<script setup lang="ts">
import { onMounted } from "vue";
import { useCursorPagination } from "../composables/useCursorPagination";
import { eventsApiService } from "../store/services";
import type { EventPageResponse, SofaScoreEvent } from "../store/services/models";
import { formatUnixTimestamp } from "../utils/time";

const pagination = useCursorPagination<SofaScoreEvent>({
  routeName: "Events",
  defaultSize: 20,
  fetchPage: async (cursor, size) => {
    const page: EventPageResponse = await eventsApiService.getEventPage(cursor, size, "desc");
    return {
      data: page.data,
      nextCursor: page.page?.nextCursor ?? "",
      hasMore: page.page?.hasMore ?? false,
    };
  },
});

onMounted(() => pagination.loadPage());
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
        :disabled="pagination.state.loading"
        @click="pagination.reload()"
      >
        Recargar
      </button>
    </div>

    <div class="card-body">
      <div v-if="pagination.state.error" class="alert alert-danger">
        {{ pagination.state.error }}
      </div>
      <div v-if="pagination.state.loading" class="alert alert-info">
        Cargando eventos...
      </div>

      <div v-if="pagination.state.data.length > 0" class="table-responsive">
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
            <tr v-for="event in pagination.state.data" :key="event.id">
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
                  <span class="text-nowrap">{{ event.teamHome?.name ?? "Home" }}</span>
                  <span class="mx-1">vs</span>
                  <span class="text-nowrap">{{ event.teamAway?.name ?? "Away" }}</span>
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
            :disabled="!pagination.state.hasPrev || pagination.state.loading"
            @click="pagination.goPrev()"
          >
            Anterior
          </button>
          <button
            class="btn btn-outline-secondary btn-sm"
            :disabled="!pagination.state.hasNext || pagination.state.loading"
            @click="pagination.goNext()"
          >
            Siguiente
          </button>
        </div>
      </div>

      <div v-else-if="!pagination.state.loading" class="text-center text-muted">
        No hay eventos disponibles
      </div>
    </div>
  </div>
</template>
