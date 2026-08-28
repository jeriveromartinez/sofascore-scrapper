<script setup lang="ts">
import { onMounted } from "vue";
import { useCursorPagination } from "../composables/useCursorPagination";
import { playbackApiService } from "../store/services";
import type { PlaybackLog, PlaybackPageResponse } from "../store/services/models";
import { formatUnixTimestamp } from "../utils/time";
import PaginationControls from "../components/PaginationControls.vue";

const pagination = useCursorPagination<PlaybackLog>({
  routeName: "Playback",
  defaultSize: 20,
  fetchPage: async (cursor, size) => {
    const page: PlaybackPageResponse = await playbackApiService.getPlaybackPage(cursor, size);
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
        <h5 class="mb-0">Playing Now</h5>
      </div>
      <button
        class="btn btn-primary"
        :disabled="pagination.state.loading"
        @click="pagination.reload()"
      >
        Consultar
      </button>
    </div>

    <div class="card-body">
      <div v-if="pagination.state.error" class="alert alert-danger">
        {{ pagination.state.error }}
      </div>
      <div v-if="pagination.state.loading" class="alert alert-info">
        Cargando estadisticas...
      </div>

      <div class="table-responsive text-nowrap" v-if="pagination.state.data.length">
        <table class="table table-striped">
          <thead>
            <tr>
              <th>#</th>
              <th>Content</th>
              <th>Started At</th>
              <th>Ended At</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(row, index) in pagination.state.data" :key="row.id">
              <td>{{ index + 1 }}</td>
              <td>{{ row.content }}</td>
              <td>{{ formatUnixTimestamp(row.startedAt) }}</td>
              <td>{{ row.endedAt > 0 ? formatUnixTimestamp(row.endedAt) : "" }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <p class="text-body-secondary mb-0" v-else-if="!pagination.state.loading">
        Sin resultados.
      </p>

      <div
        v-if="pagination.state.data.length"
        class="d-flex flex-wrap gap-2 mt-3 align-items-center justify-content-between"
      >
        <PaginationControls
          :state="pagination.state"
          @prev="pagination.goPrev()"
          @next="pagination.goNext()"
        />
      </div>
    </div>
  </div>
</template>