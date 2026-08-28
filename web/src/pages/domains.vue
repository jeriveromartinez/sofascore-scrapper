<script setup lang="ts">
import { onMounted, ref } from "vue";
import { toast } from "vue3-toastify";
import { useCursorPagination } from "../composables/useCursorPagination";
import DomainFormModal from "./DomainFormModal.vue";
import { domainsApiService, usersApiService } from "../store/services";
import type { Domain, DomainPageResponse, User } from "../store/services/models";
import PaginationControls from "../components/PaginationControls.vue";

const modalRef = ref<InstanceType<typeof DomainFormModal> | null>(null);
const users = ref<User[]>([]);

const pagination = useCursorPagination<Domain>({
  routeName: "Domains",
  defaultSize: 20,
  fetchPage: async (cursor, size) => {
    const page: DomainPageResponse = await domainsApiService.getDomainPage(cursor, size);
    return {
      data: page.data,
      nextCursor: page.page?.nextCursor ?? "",
      hasMore: page.page?.hasMore ?? false,
    };
  },
});

function openCreate(): void {
  if (users.value.length === 0) {
    toast.info("Debe crear al menos un usuario antes de registrar dominios");
    return;
  }
  modalRef.value?.open();
}

function openEdit(d: Domain): void {
  modalRef.value?.open(d);
}

async function onSubmit(payload: { id: number | null; domain: string; userId: number }): Promise<void> {
  try {
    if (payload.id) {
      await domainsApiService.updateDomain(payload.id, {
        domain: payload.domain,
        userId: payload.userId,
      });
      modalRef.value?.reset();
      toast.success("Dominio actualizado");
    } else {
      await domainsApiService.createDomain({
        domain: payload.domain,
        userId: payload.userId,
      });
      modalRef.value?.reset();
      toast.success("Dominio creado");
    }
    await pagination.reload();
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "No se pudo guardar el dominio");
  }
}

async function deleteDomain(id: number): Promise<void> {
  if (!confirm("¿Está seguro de que desea eliminar este dominio?")) return;
  try {
    await domainsApiService.deleteDomain(id);
    toast.success("Dominio eliminado");
    await pagination.reload();
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "No se pudo eliminar el dominio");
  }
}

function formatTimestamp(date?: string): string {
  if (!date) return "-";
  return new Date(date).toLocaleString();
}

onMounted(async () => {
  try {
    users.value = await usersApiService.getAllUsers();
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "No se pudieron cargar los usuarios");
  }
  await pagination.loadPage();
});
</script>

<template>
  <div class="card">
    <div class="card-header d-flex justify-content-between align-items-center">
      <h5 class="mb-0">Gestión de Dominios</h5>
      <button
        class="btn btn-primary btn-sm"
        :disabled="pagination.state.loading || users.length === 0"
        @click="openCreate"
      >
        Crear
      </button>
    </div>

    <div class="card-body">
      <div v-if="pagination.state.error" class="alert alert-danger">
        {{ pagination.state.error }}
      </div>

      <div
        v-if="users.length === 0 && !pagination.state.loading"
        class="alert alert-info"
      >
        Debe crear al menos un usuario antes de registrar dominios.
      </div>

      <div v-if="pagination.state.loading" class="text-center mb-3">
        <div class="spinner-border" role="status">
          <span class="visually-hidden">Cargando...</span>
        </div>
      </div>

      <div v-if="pagination.state.data.length > 0" class="table-responsive">
        <table class="table table-striped align-middle">
          <thead>
            <tr>
              <th>ID</th>
              <th>Dominio</th>
              <th>Usuario</th>
              <th>Creado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="domain in pagination.state.data" :key="domain.id">
              <td>{{ domain.id }}</td>
              <td>{{ domain.domain }}</td>
              <td>{{ domain.user?.email || "-" }}</td>
              <td>{{ formatTimestamp(domain.createdAt) }}</td>
              <td>
                <button
                  class="btn btn-sm btn-warning me-2"
                  :disabled="pagination.state.loading"
                  @click="openEdit(domain)"
                >
                  Editar
                </button>
                <button
                  class="btn btn-sm btn-danger"
                  :disabled="pagination.state.loading"
                  @click="deleteDomain(domain.id)"
                >
                  Eliminar
                </button>
              </td>
            </tr>
          </tbody>
        </table>

        <PaginationControls
          :state="pagination.state"
          @prev="pagination.goPrev()"
          @next="pagination.goNext()"
        />
      </div>

      <div v-else-if="!pagination.state.loading" class="text-center text-muted">
        No hay dominios registrados
      </div>
    </div>
  </div>

  <DomainFormModal ref="modalRef" :users="users" @submit="onSubmit" :auto-close-modal="false" />
</template>
