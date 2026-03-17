<script setup lang="ts">
import { reactive } from "vue";
import { playbackApiService } from "../store/services";
import type {
  PlaybackLog,
  PlaybackUpdateMethod,
} from "../store/services/models";

const createState = reactive({
  deviceToken: "",
  eventId: 0,
  startedAt: "",
  loading: false,
  error: "",
  result: null as PlaybackLog | null,
});

const updateState = reactive({
  playbackId: "",
  endedAt: "",
  loading: false,
  error: "",
  success: "",
});

function parseUnix(value: string): number | undefined {
  const trimmed = value.trim();
  if (!trimmed) return undefined;

  const parsed = Number(trimmed);
  if (!Number.isFinite(parsed) || parsed < 0) return undefined;

  return Math.trunc(parsed);
}

async function createPlayback(): Promise<void> {
  createState.loading = true;
  createState.error = "";

  try {
    createState.result = await playbackApiService.createPlayback({
      deviceToken: createState.deviceToken,
      sofaScoreEventId: createState.eventId,
      startedAt: parseUnix(createState.startedAt) ?? 0,
    });

    if (!updateState.playbackId) {
      updateState.playbackId = String(createState.result.id);
    }
  } catch (error) {
    createState.error =
      error instanceof Error ? error.message : "No se pudo crear playback";
  } finally {
    createState.loading = false;
  }
}

async function closePlayback(method: PlaybackUpdateMethod): Promise<void> {
  updateState.loading = true;
  updateState.error = "";
  updateState.success = "";

  try {
    const id = Number(updateState.playbackId);
    await playbackApiService.updatePlayback(
      id,
      { endedAt: parseUnix(updateState.endedAt) ?? 0 },
      method,
    );
    updateState.success = `Playback actualizado con ${method}`;
  } catch (error) {
    updateState.error =
      error instanceof Error ? error.message : "No se pudo actualizar playback";
  } finally {
    updateState.loading = false;
  }
}
</script>

<template>
  <div class="row g-4">
    <div class="col-12 col-xl-6">
      <div class="card h-100">
        <div class="card-header">
          <h5 class="mb-0">Crear Playback</h5>
          <small class="text-body-secondary">POST /api/v1/playback</small>
        </div>

        <div class="card-body">
          <form class="row g-3" @submit.prevent="createPlayback">
            <div class="col-12">
              <label class="form-label">Device Token *</label>
              <input
                v-model="createState.deviceToken"
                class="form-control"
                type="text"
                required
              />
            </div>
            <div class="col-12">
              <label class="form-label">SofaScore Event ID *</label>
              <input
                v-model.number="createState.eventId"
                class="form-control"
                type="number"
                min="1"
                required
              />
            </div>
            <div class="col-12">
              <label class="form-label">Started At (unix, opcional)</label>
              <input
                v-model="createState.startedAt"
                class="form-control"
                type="text"
                placeholder="1772782000"
              />
            </div>
            <div class="col-12">
              <button class="btn btn-primary" :disabled="createState.loading">
                Crear
              </button>
            </div>
          </form>

          <div v-if="createState.error" class="alert alert-danger mt-3">
            {{ createState.error }}
          </div>

          <div v-if="createState.result" class="alert alert-success mt-3 mb-0">
            Playback creado con ID {{ createState.result.id }}
          </div>
        </div>
      </div>
    </div>

    <div class="col-12 col-xl-6">
      <div class="card h-100">
        <div class="card-header">
          <h5 class="mb-0">Cerrar Playback</h5>
          <small class="text-body-secondary"
            >PUT/PATCH /api/v1/playback/:id</small
          >
        </div>

        <div class="card-body">
          <form class="row g-3" @submit.prevent="closePlayback('PUT')">
            <div class="col-12">
              <label class="form-label">Playback ID *</label>
              <input
                v-model="updateState.playbackId"
                class="form-control"
                type="number"
                min="1"
                required
              />
            </div>
            <div class="col-12">
              <label class="form-label">Ended At (unix, opcional)</label>
              <input
                v-model="updateState.endedAt"
                class="form-control"
                type="text"
                placeholder="1772782600"
              />
            </div>
            <div class="col-12 d-flex gap-2">
              <button
                class="btn btn-warning"
                type="submit"
                :disabled="updateState.loading"
              >
                Cerrar con PUT
              </button>
              <button
                class="btn btn-outline-warning"
                type="button"
                :disabled="updateState.loading"
                @click="closePlayback('PATCH')"
              >
                Cerrar con PATCH
              </button>
            </div>
          </form>

          <div v-if="updateState.error" class="alert alert-danger mt-3">
            {{ updateState.error }}
          </div>
          <div v-if="updateState.success" class="alert alert-success mt-3 mb-0">
            {{ updateState.success }}
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
