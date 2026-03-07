<script setup lang="ts">
import { reactive } from "vue";
import { devicesApiService } from "../store/services";
import type { Device } from "../store/services/models";

const form = reactive({
  token: "",
  platform: "android",
  name: "",
  loading: false,
  error: "",
  success: "",
  result: null as Device | null,
});

async function submit(): Promise<void> {
  form.loading = true;
  form.error = "";
  form.success = "";

  try {
    form.result = await devicesApiService.registerDevice({
      token: form.token,
      platform: form.platform,
      name: form.name,
    });
    form.success = "Dispositivo registrado/actualizado";
  } catch (error) {
    form.error =
      error instanceof Error
        ? error.message
        : "No se pudo registrar el dispositivo";
  } finally {
    form.loading = false;
  }
}
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Dispositivos</h5>
      <small class="text-body-secondary">POST /api/v1/devices</small>
    </div>

    <div class="card-body">
      <form class="row g-3" @submit.prevent="submit">
        <div class="col-md-5">
          <label class="form-label">Token *</label>
          <input
            v-model="form.token"
            type="text"
            class="form-control"
            required
          />
        </div>
        <div class="col-md-3">
          <label class="form-label">Plataforma</label>
          <input v-model="form.platform" type="text" class="form-control" />
        </div>
        <div class="col-md-4">
          <label class="form-label">Nombre</label>
          <input v-model="form.name" type="text" class="form-control" />
        </div>

        <div class="col-12">
          <button class="btn btn-primary" :disabled="form.loading">
            Guardar dispositivo
          </button>
        </div>
      </form>

      <div v-if="form.error" class="alert alert-danger mt-3">
        {{ form.error }}
      </div>
      <div v-if="form.success" class="alert alert-success mt-3">
        {{ form.success }}
      </div>

      <div v-if="form.result" class="mt-3 p-3 border rounded bg-lighter">
        <h6 class="mb-2">Respuesta</h6>
        <p class="mb-1"><strong>ID:</strong> {{ form.result.ID }}</p>
        <p class="mb-1"><strong>UserID:</strong> {{ form.result.UserID }}</p>
        <p class="mb-1"><strong>Token:</strong> {{ form.result.Token }}</p>
        <p class="mb-1">
          <strong>LastSeen:</strong> {{ form.result.LastSeen }}
        </p>
      </div>
    </div>
  </div>
</template>
