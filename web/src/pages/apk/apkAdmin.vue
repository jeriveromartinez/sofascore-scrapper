<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useCursorPagination } from "../../composables/useCursorPagination";
import apkUploadModal from "./apkUploadModal.vue";
import apkEditUrlModal from "./apkEditUrl.vue";
import { apkApiService } from "../../store/services";
import type {
  ApkPageResponse,
  ApkVersionInfo,
} from "../../store/services/models";
import PaginationControls from "../../components/PaginationControls.vue";

const editModal = ref<typeof apkEditUrlModal>();

const pagination = useCursorPagination<ApkVersionInfo>({
  routeName: "ApkAdmin",
  defaultSize: 10,
  fetchPage: async (cursor, size) => {
    const page: ApkPageResponse = await apkApiService.listVersionsPage(
      cursor,
      size,
    );
    return {
      data: page.data,
      nextCursor: page.page?.nextCursor ?? "",
      hasMore: page.page?.hasMore ?? false,
    };
  },
});

function getDownloadUrl(appKey: string): string {
  return apkApiService.getDownloadUrl(appKey);
}

function openEditModal(version: ApkVersionInfo): void {
  editModal.value?.openModal(version);
}

function formatFileSize(bytes: number): string {
  if (bytes === 0) return "0 B";

  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const k = 1024;
  const i = Math.floor(Math.log(bytes) / Math.log(k));

  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + " " + sizes[i];
}

onMounted(() => pagination.loadPage());
</script>

<template>
  <div class="col-12">
    <div class="card h-100">
      <div
        class="card-header d-flex justify-content-between align-items-center"
      >
        <div>
          <h5 class="mb-0">Versiones APK</h5>
        </div>
        <div>
          <apk-upload-modal
            @uploaded="() => pagination.reload()"
            :auto-close-modal="false"
          />
          <button
            class="btn btn-outline-primary btn-sm ms-2"
            :disabled="pagination.state.loading"
            @click="pagination.reload()"
          >
            Recargar
          </button>
        </div>
      </div>

      <div class="card-body">
        <div v-if="pagination.state.error" class="alert alert-danger">
          {{ pagination.state.error }}
        </div>
        <div v-if="pagination.state.loading" class="alert alert-info">
          Cargando versiones...
        </div>

        <div
          class="table-responsive text-nowrap"
          v-if="pagination.state.data.length"
        >
          <table class="table table-sm table-striped align-middle">
            <thead>
              <tr>
                <th>Version</th>
                <th>Package</th>
                <th>Download Token</th>
                <th>Size</th>
                <th>Downloads</th>
                <th>Panel URL</th>
                <th>Accion</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="version in pagination.state.data" :key="version.id">
                <td>{{ version.version }}</td>
                <td>{{ version.packageName }}</td>
                <td>{{ version.downloadToken }}</td>
                <td>{{ formatFileSize(version.fileSize) }}</td>
                <td>{{ version.downloads }}</td>
                <td>{{ version.panelUrl }}</td>
                <td>
                  <a
                    class="btn btn-sm btn-outline-success"
                    :href="getDownloadUrl(version.downloadUrl)"
                  >
                    Descargar
                  </a>
                  <button
                    class="ms-2 btn btn-sm btn-outline-warning"
                    @click.prevent="() => openEditModal(version)"
                  >
                    Editar
                  </button>
                </td>
              </tr>
            </tbody>
          </table>

          <div class="d-flex justify-content-between align-items-center mt-3">
            <PaginationControls
              :state="pagination.state"
              @prev="pagination.goPrev()"
              @next="pagination.goNext()"
            />
          </div>
        </div>
        <p
          class="text-body-secondary mb-0"
          v-else-if="!pagination.state.loading"
        >
          No hay versiones cargadas.
        </p>
      </div>
    </div>
  </div>

  <apk-edit-url-modal
    ref="editModal"
    @updated="() => pagination.reload()"
    :auto-close-modal="false"
  />
</template>
