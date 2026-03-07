<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { apkApiService } from "../../store/services";
import type { ApkVersionInfo } from "../../store/services/models";

const upload = reactive({
  file: null as File | null,
  version: "",
  description: "",
  loading: false,
  error: "",
  success: "",
});

const listState = reactive({
  loading: false,
  error: "",
  versions: [] as ApkVersionInfo[],
});

function onFileChange(event: Event): void {
  const target = event.target as HTMLInputElement;
  upload.file = target.files?.[0] ?? null;
}

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

async function submitUpload(): Promise<void> {
  if (!upload.file) {
    upload.error = "Selecciona un archivo APK";
    return;
  }

  upload.loading = true;
  upload.error = "";
  upload.success = "";

  try {
    const response = await apkApiService.uploadApk(
      upload.file,
      upload.version || undefined,
      upload.description || undefined,
    );
    upload.success = `APK ${response.version} cargado correctamente`;
    upload.file = null;
    upload.version = "";
    upload.description = "";
    await loadVersions();
  } catch (error) {
    upload.error =
      error instanceof Error ? error.message : "No se pudo subir el APK";
  } finally {
    upload.loading = false;
  }
}

function getDownloadUrl(path: string): string {
  return apkApiService.getDownloadUrl(path);
}

onMounted(() => loadVersions());
</script>

<template>
  <div class="row g-4">
    <div class="col-12 col-xl-5">
      <div class="card h-100">
        <div class="card-header">
          <h5 class="mb-0">Subir APK</h5>
          <small class="text-body-secondary">POST /api/v1/apk/upload</small>
        </div>

        <div class="card-body">
          <form class="row g-3" @submit.prevent="submitUpload">
            <div class="col-12">
              <label class="form-label">Archivo APK *</label>
              <input
                class="form-control"
                type="file"
                accept=".apk"
                @change="onFileChange"
                required
              />
            </div>
            <div class="col-12">
              <label class="form-label">Version (x.y.z)</label>
              <input
                v-model="upload.version"
                class="form-control"
                type="text"
                placeholder="1.2.3"
              />
            </div>
            <div class="col-12">
              <label class="form-label">Descripcion</label>
              <textarea
                v-model="upload.description"
                class="form-control"
                rows="3"
              ></textarea>
            </div>
            <div class="col-12">
              <button class="btn btn-primary" :disabled="upload.loading">
                Subir
              </button>
            </div>
          </form>

          <div v-if="upload.error" class="alert alert-danger mt-3">
            {{ upload.error }}
          </div>
          <div v-if="upload.success" class="alert alert-success mt-3 mb-0">
            {{ upload.success }}
          </div>
        </div>
      </div>
    </div>

    <div class="col-12 col-xl-7">
      <div class="card h-100">
        <div
          class="card-header d-flex justify-content-between align-items-center"
        >
          <div>
            <h5 class="mb-0">Versiones APK</h5>
          </div>
          <button
            class="btn btn-outline-primary btn-sm"
            :disabled="listState.loading"
            @click="loadVersions"
          >
            Recargar
          </button>
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
  </div>
</template>
