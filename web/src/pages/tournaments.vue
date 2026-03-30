<script setup lang="ts">
import { reactive, onMounted } from "vue";
import { tournamentsApiService } from "../store/services";
import type { Tournament } from "../store/services/models";

const state = reactive({
  tournaments: [] as Tournament[],
  loading: false,
  error: "",
  editingId: null as number | null,
  form: {
    name: "",
    slug: "",
  },
});

async function loadTournaments(): Promise<void> {
  state.loading = true;
  state.error = "";
  try {
    state.tournaments = await tournamentsApiService.getAllTournaments();
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudieron cargar los torneos";
  } finally {
    state.loading = false;
  }
}

async function createTournament(): Promise<void> {
  if (!state.form.name) {
    state.error = "El nombre es requerido";
    return;
  }

  state.loading = true;
  state.error = "";
  try {
    await tournamentsApiService.createTournament({
      name: state.form.name,
      slug: state.form.slug,
    });
    state.form.name = "";
    state.form.slug = "";
    await loadTournaments();
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "No se pudo crear el torneo";
  } finally {
    state.loading = false;
  }
}

function startEdit(tournament: Tournament): void {
  state.editingId = tournament.id;
  state.form.name = tournament.name;
  state.form.slug = tournament.slug;
}

function cancelEdit(): void {
  state.editingId = null;
  state.form.name = "";
  state.form.slug = "";
}

async function updateTournament(): Promise<void> {
  if (!state.editingId || !state.form.name) return;

  state.loading = true;
  state.error = "";
  try {
    await tournamentsApiService.updateTournament(state.editingId, {
      name: state.form.name,
      slug: state.form.slug,
    });
    cancelEdit();
    await loadTournaments();
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudo actualizar el torneo";
  } finally {
    state.loading = false;
  }
}

async function deleteTournament(id: number): Promise<void> {
  if (!confirm("¿Está seguro de que desea eliminar este torneo?")) return;

  state.loading = true;
  state.error = "";
  try {
    await tournamentsApiService.deleteTournament(id);
    await loadTournaments();
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "No se pudo eliminar el torneo";
  } finally {
    state.loading = false;
  }
}

onMounted(() => {
  loadTournaments();
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Gestión de Torneos</h5>
    </div>

    <div class="card-body">
      <div v-if="state.error" class="alert alert-danger">
        {{ state.error }}
      </div>

      <form
        v-if="state.editingId"
        class="row g-3 mb-4"
        @submit.prevent="state.editingId ? updateTournament() : createTournament()"
      >
        <div class="col-md-5">
          <label class="form-label">Nombre *</label>
          <input
            v-model="state.form.name"
            type="text"
            class="form-control"
            required
          />
        </div>
        <div class="col-md-5">
          <label class="form-label">Slug</label>
          <input v-model="state.form.slug" type="text" class="form-control" />
        </div>
        <div class="col-md-2 d-flex align-items-end">
          <button
            v-if="!state.editingId"
            class="btn btn-primary me-2"
            :disabled="state.loading"
          >
            Crear
          </button>
          <template v-else>
            <button class="btn btn-success me-2" :disabled="state.loading">
              Actualizar
            </button>
            <button type="button" class="btn btn-secondary" @click="cancelEdit">
              Cancelar
            </button>
          </template>
        </div>
      </form>

      <div v-if="state.loading" class="text-center">
        <div class="spinner-border" role="status">
          <span class="visually-hidden">Cargando...</span>
        </div>
      </div>

      <div v-else-if="state.tournaments.length > 0" class="table-responsive">
        <table class="table table-striped">
          <thead>
            <tr>
              <th>ID</th>
              <th>Nombre</th>
              <th>Slug</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="tournament in state.tournaments" :key="tournament.id">
              <td>{{ tournament.id }}</td>
              <td>{{ tournament.name }}</td>
              <td>{{ tournament.slug }}</td>
              <td>
                <button
                  class="btn btn-sm btn-warning me-2"
                  @click="startEdit(tournament)"
                >
                  Editar
                </button>
                <button
                  class="btn btn-sm btn-danger"
                  @click="deleteTournament(tournament.id)"
                >
                  Eliminar
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else class="text-center text-muted">
        No hay torneos registrados
      </div>
    </div>
  </div>
</template>
