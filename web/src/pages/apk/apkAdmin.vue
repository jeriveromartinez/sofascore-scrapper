<script setup lang="ts">
import { onMounted, reactive } from "vue";
import apkUploadModal from "./apkUploadModal.vue";
import { apkApiService } from "../../store/services";
import type { ApkVersionInfo } from "../../store/services/models";

const listState = reactive({
  loading: false,
  error: "",
  versions: [] as ApkVersionInfo[],
});

async function loadVersions(): Promise<void> {
  listState.loading = true;
  listState.error = "";

  try {
    listState.versions = await apkApiService.listVersions();
  } catch (error) {
    listState.error =
      error instanceof Error ? error.message : "No se pudo cargar el listado";
  } finally {
    listState.loading = false;
  }
}

function getDownloadUrl(appKey: string): string {
  return apkApiService.getDownloadUrl(appKey);
}

onMounted(() => loadVersions());
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
            @uploaded="loadVersions"
            :auto-close-modal="false"
          />
          <button
            class="btn btn-outline-primary btn-sm ms-2"
            :disabled="listState.loading"
            @click="loadVersions"
          >
            Recargar
          </button>
        </div>
      </div>

      <div class="card-body">
        <div v-if="listState.error" class="alert alert-danger">
          {{ listState.error }}
        </div>
        <div v-if="listState.loading" class="alert alert-info">
          Cargando versiones...
        </div>

        <div
          class="table-responsive text-nowrap"
          v-if="listState.versions.length"
        >
          <table class="table table-sm table-striped align-middle">
            <thead>
              <tr>
                <th>Version</th>
                <th>Package</th>
                <th>Download Token</th>
                <th>Size</th>
                <th>Accion</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="version in listState.versions" :key="version.id">
                <td>{{ version.version }}</td>
                <td>{{ version.package_name }}</td>
                <td>{{ version.download_token }}</td>
                <td>{{ version.file_size }}</td>
                <td>
                  <a
                    class="btn btn-sm btn-outline-success"
                    :href="getDownloadUrl(version.download_url)"
                  >
                    Descargar
                  </a>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <p class="text-body-secondary mb-0" v-else-if="!listState.loading">
          No hay versiones cargadas.
        </p>
      </div>
    </div>
  </div>
</template>
