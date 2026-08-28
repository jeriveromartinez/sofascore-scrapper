<script setup lang="ts">
import { onMounted, ref } from "vue";
import { toast } from "vue3-toastify";
import { useCursorPagination } from "../composables/useCursorPagination";
import UserFormModal from "./UserFormModal.vue";
import { usersApiService } from "../store/services";
import type { User, UserPageResponse } from "../store/services/models";
import PaginationControls from "../components/PaginationControls.vue";

const modalRef = ref<InstanceType<typeof UserFormModal> | null>(null);

const pagination = useCursorPagination<User>({
  routeName: "Users",
  defaultSize: 20,
  fetchPage: async (cursor, size) => {
    const page: UserPageResponse = await usersApiService.getUserPage(cursor, size);
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

function openEdit(user: User): void {
  modalRef.value?.open(user);
}

async function onSubmit(payload: { id: number | null; email: string; password: string }): Promise<void> {
  try {
    if (payload.id) {
      await usersApiService.updateUser(payload.id, {
        email: payload.email,
        password: payload.password,
      });
      modalRef.value?.reset();
      toast.success("Usuario actualizado");
    } else {
      await usersApiService.createUser({
        email: payload.email,
        password: payload.password,
      });
      modalRef.value?.reset();
      toast.success("Usuario creado");
    }
    await pagination.reload();
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "No se pudo guardar el usuario");
  }
}

async function deleteUser(id: number): Promise<void> {
  if (!confirm("¿Está seguro de que desea eliminar este usuario?")) return;
  try {
    await usersApiService.deleteUser(id);
    toast.success("Usuario eliminado");
    await pagination.reload();
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "No se pudo eliminar el usuario");
  }
}

function formatTimestamp(date?: string): string {
  if (!date) return "-";
  return new Date(date).toLocaleString();
}

onMounted(() => pagination.loadPage());
</script>

<template>
  <div class="card">
    <div class="card-header d-flex justify-content-between align-items-center">
      <h5 class="mb-0">Gestión de Usuarios</h5>
      <button class="btn btn-primary btn-sm" :disabled="pagination.state.loading" @click="openCreate">
        Crear
      </button>
    </div>

    <div class="card-body">
      <div v-if="pagination.state.error" class="alert alert-danger">
        {{ pagination.state.error }}
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
              <th>Email</th>
              <th>Creado</th>
              <th>Acciones</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="user in pagination.state.data" :key="user.id">
              <td>{{ user.id }}</td>
              <td>{{ user.email }}</td>
              <td>{{ formatTimestamp(user.createdAt) }}</td>
              <td>
                <button
                  class="btn btn-sm btn-warning me-2"
                  :disabled="pagination.state.loading"
                  @click="openEdit(user)"
                >
                  Editar
                </button>
                <button
                  class="btn btn-sm btn-danger"
                  :disabled="pagination.state.loading"
                  @click="deleteUser(user.id)"
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
        No hay usuarios registrados
      </div>
    </div>
  </div>

  <UserFormModal ref="modalRef" @submit="onSubmit" :auto-close-modal="false" />
</template>
