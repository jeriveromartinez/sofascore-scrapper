<script setup lang="ts">
import { onMounted, ref } from "vue";
import { toast } from "vue3-toastify";
import { useCursorPagination } from "../composables/useCursorPagination";
import TournamentFormModal from "./TournamentFormModal.vue";
import { tournamentsApiService } from "../store/services";
import type { Tournament, TournamentPageResponse } from "../store/services/models";

const modalRef = ref<InstanceType<typeof TournamentFormModal> | null>(null);

const pagination = useCursorPagination<Tournament>({
  routeName: "Tournaments",
  defaultSize: 20,
  fetchPage: async (cursor, size) => {
    const page: TournamentPageResponse = await tournamentsApiService.getTournamentPage(cursor, size);
    return {
      data: page.data,
      nextCursor: page.page?.nextCursor ?? "",
      hasMore: page.page?.hasMore ?? false,
    };
  },
});

function openCreate(): void {
  modalRef.value?.open();
}

function openEdit(t: Tournament): void {
  modalRef.value?.open(t);
}

async function onSubmit(payload: { id: number | null; name: string; slug: string }): Promise<void> {
  try {
    if (payload.id) {
      await tournamentsApiService.updateTournament(payload.id, {
        name: payload.name,
        slug: payload.slug,
      });
      modalRef.value?.reset();
      toast.success("Torneo actualizado");
    } else {
      await tournamentsApiService.createTournament({
        name: payload.name,
        slug: payload.slug,
      });
      modalRef.value?.reset();
      toast.success("Torneo creado");
    }
    await pagination.reload();
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "No se pudo guardar el torneo");
  }
}

async function deleteTournament(id: number): Promise<void> {
  if (!confirm("¿Está seguro de que desea eliminar este torneo?")) return;
  try {
    await tournamentsApiService.deleteTournament(id);
    toast.success("Torneo eliminado");
    await pagination.reload();
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "No se pudo eliminar el torneo");
  }
}

onMounted(() => pagination.loadPage());
</script>

<template>
  <div class="card">
    <div class="card-header d-flex justify-content-between align-items-center">
      <h5 class="mb-0">Gestión de Torneos</h5>
      <button class="btn btn-primary btn-sm" :disabled="pagination.state.loading" @click="openCreate">
        Crear
      </button>
    </div>

    <div class="card-body">
      <div v-if="pagination.state.error" class="alert alert-danger">
        {{ pagination.state.error }}
      </div>

      <div v-if="pagination.state.loading" class="text-center">
        <div class="spinner-border" role="status">
          <span class="visually-hidden">Cargando...</span>
        </div>
      </div>

      <div v-else-if="pagination.state.data.length > 0" class="table-responsive">
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
            <tr v-for="tournament in pagination.state.data" :key="tournament.id">
              <td>{{ tournament.id }}</td>
              <td>{{ tournament.name }}</td>
              <td>{{ tournament.slug }}</td>
              <td>
                <button
                  class="btn btn-sm btn-warning me-2"
                  :disabled="pagination.state.loading"
                  @click="openEdit(tournament)"
                >
                  Editar
                </button>
                <button
                  class="btn btn-sm btn-danger"
                  :disabled="pagination.state.loading"
                  @click="deleteTournament(tournament.id)"
                >
                  Eliminar
                </button>
              </td>
            </tr>
          </tbody>
        </table>

        <div class="d-flex justify-content-between align-items-center mt-3">
          <button
            class="btn btn-outline-secondary"
            :disabled="!pagination.state.hasPrev || pagination.state.loading"
            @click="pagination.goPrev()"
          >
            Anterior
          </button>
          <button
            class="btn btn-outline-secondary"
            :disabled="!pagination.state.hasNext || pagination.state.loading"
            @click="pagination.goNext()"
          >
            Siguiente
          </button>
        </div>
      </div>

      <div v-else class="text-center text-muted">
        No hay torneos registrados
      </div>
    </div>
  </div>

  <TournamentFormModal ref="modalRef" @submit="onSubmit" :auto-close-modal="false" />
</template>
