<script setup lang="ts">
import { onMounted } from "vue";
import { useCursorPagination } from "../composables/useCursorPagination";
import { devicesApiService } from "../store/services";
import type { Device, DevicePageResponse } from "../store/services/models";
import { formatUnixTimestamp } from "../utils/time";
import PaginationControls from "../components/PaginationControls.vue";

const pagination = useCursorPagination<Device>({
  routeName: "Devices",
  defaultSize: 20,
  fetchPage: async (cursor, size) => {
    const page: DevicePageResponse = await devicesApiService.getDevicePage(cursor, size);
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
    <div class="card-header">
      <h5 class="mb-0">Dispositivos</h5>
    </div>

    <div class="card-body">
      <div v-if="pagination.state.error" class="alert alert-danger">
        {{ pagination.state.error }}
      </div>
      <div v-if="pagination.state.loading" class="alert alert-info">
        Cargando dispositivos...
      </div>

      <div v-if="pagination.state.data.length > 0" class="table-responsive">
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
            <tr v-for="device in pagination.state.data" :key="device.id">
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

      <div v-else-if="!pagination.state.loading" class="text-center text-muted">
        No hay dispositivos registrados
      </div>

      <div
        v-if="pagination.state.data.length > 0"
        class="d-flex gap-2 mt-3 align-items-center justify-content-between"
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
