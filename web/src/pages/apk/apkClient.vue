<script setup lang="ts">
import { reactive } from "vue";
import { apkApiService } from "../../store/services";
import type { ApkCheckResponse } from "../../store/services/models";

const state = reactive({
  version: "1.0.0",
  loading: false,
  error: "",
  data: null as ApkCheckResponse | null,
  downloadError: "",
});

function extractToken(downloadUrl: string): string | null {
  const parts = downloadUrl.split("/");
  const lastIndex = parts.length - 1;
  if (lastIndex < 0) return null;

  const lastPart = parts[lastIndex];
  return lastPart ?? null;
}

function downloadWithAnchor(path: string): void {
  const href = apkApiService.getDownloadUrl(path);
  const anchor = document.createElement("a");
  anchor.href = href;
  anchor.target = "_blank";
  anchor.rel = "noopener";
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
}

async function downloadWithApiBlob(path: string): Promise<void> {
  state.downloadError = "";

  try {
    const token = extractToken(path);
    if (!token) {
      throw new Error("Download token invalido");
    }

    const blob = await apkApiService.downloadByToken(token);
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `sofascore-${state.data?.latest_version ?? "latest"}.apk`;
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
  } catch (error) {
    state.downloadError =
      error instanceof Error ? error.message : "No se pudo descargar el APK";
  }
}

async function check(): Promise<void> {
  state.loading = true;
  state.error = "";
  state.data = null;

  try {
    state.data = await apkApiService.checkUpdate(state.version);
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "No se pudo validar version";
  } finally {
    state.loading = false;
  }
}
</script>

<template>
  <div class="card">
    <div class="card-header d-flex justify-content-between align-items-center">
      <div>
        <h5 class="mb-0">APK Cliente</h5>
        <small class="text-body-secondary"
          >GET /api/v1/apk/check y GET /api/v1/apk/download/:token</small
        >
      </div>
    </div>

    <div class="card-body">
      <form class="row g-3 mb-4" @submit.prevent="check">
        <div class="col-md-4">
          <label class="form-label">Version actual</label>
          <input
            v-model="state.version"
            class="form-control"
            type="text"
            placeholder="1.0.0"
            required
          />
        </div>
        <div class="col-md-2 d-flex align-items-end">
          <button class="btn btn-primary w-100" :disabled="state.loading">
            Verificar
          </button>
        </div>
      </form>

      <div v-if="state.error" class="alert alert-danger">{{ state.error }}</div>
      <div v-if="state.loading" class="alert alert-info">Consultando...</div>

      <div v-if="state.data" class="border rounded p-3">
        <p class="mb-1">
          <strong>Ultima version:</strong> {{ state.data.latest_version }}
        </p>
        <p class="mb-1">
          <strong>Package:</strong> {{ state.data.package_name }}
        </p>
        <p class="mb-1">
          <strong>VersionCode:</strong> {{ state.data.version_code }}
        </p>
        <p class="mb-3">
          <strong>Update:</strong>
          <span
            :class="state.data.update_available ? 'text-success' : 'text-muted'"
          >
            {{ state.data.update_available ? "Disponible" : "No disponible" }}
          </span>
        </p>

        <div
          v-if="state.data.update_available && state.data.download_url"
          class="d-flex flex-wrap gap-2"
        >
          <button
            class="btn btn-success"
            type="button"
            @click="downloadWithAnchor(state.data.download_url)"
          >
            Descargar (enlace directo)
          </button>
          <button
            class="btn btn-outline-success"
            type="button"
            @click="downloadWithApiBlob(state.data.download_url)"
          >
            Descargar (via API Blob)
          </button>
        </div>

        <div v-if="state.downloadError" class="alert alert-danger mt-3 mb-0">
          {{ state.downloadError }}
        </div>
      </div>
    </div>
  </div>
</template>
