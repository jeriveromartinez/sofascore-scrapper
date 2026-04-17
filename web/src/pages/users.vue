<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { usersApiService } from "../store/services";
import type { User } from "../store/services/models";

const state = reactive({
  users: [] as User[],
  loading: false,
  error: "",
  editingId: null as number | null,
  form: {
    email: "",
    password: "",
  },
});

function resetForm(): void {
  state.editingId = null;
  state.form.email = "";
  state.form.password = "";
}

async function loadUsers(): Promise<void> {
  state.loading = true;
  state.error = "";

  try {
    state.users = await usersApiService.getAllUsers();
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudieron cargar los usuarios";
  } finally {
    state.loading = false;
  }
}

function startEdit(user: User): void {
  state.editingId = user.id;
  state.form.email = user.email;
  state.form.password = "";
}

async function submitForm(): Promise<void> {
  if (!state.form.email) {
    state.error = "El email es requerido";
    return;
  }

  if (!state.editingId && !state.form.password) {
    state.error = "La contraseña es requerida para crear un usuario";
    return;
  }

  state.loading = true;
  state.error = "";

  try {
    if (state.editingId) {
      await usersApiService.updateUser(state.editingId, {
        email: state.form.email,
        password: state.form.password,
      });
    } else {
      await usersApiService.createUser({
        email: state.form.email,
        password: state.form.password,
      });
    }

    resetForm();
    await loadUsers();
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "No se pudo guardar el usuario";
  } finally {
    state.loading = false;
  }
}

async function deleteUser(id: number): Promise<void> {
  if (!confirm("¿Está seguro de que desea eliminar este usuario?")) return;

  state.loading = true;
  state.error = "";

  try {
    await usersApiService.deleteUser(id);
    if (state.editingId === id) {
      resetForm();
    }
    await loadUsers();
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "No se pudo eliminar el usuario";
  } finally {
    state.loading = false;
  }
}

onMounted(() => {
  void loadUsers();
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Gestión de Usuarios</h5>
    </div>

    <div class="card-body">
      <div v-if="state.error" class="alert alert-danger">
        {{ state.error }}
      </div>

      <form class="row g-3 mb-4" @submit.prevent="submitForm">
        <div class="col-md-5">
          <label class="form-label">Email *</label>
          <input
            v-model="state.form.email"
            type="email"
            class="form-control"
            required
          />
        </div>

        <div class="col-md-4">
          <label class="form-label">
            {{ state.editingId ? "Nueva contraseña" : "Contraseña *" }}
          </label>
          <input
            v-model="state.form.password"
            type="password"
            class="form-control"
            :required="!state.editingId"
          />
          <small v-if="state.editingId" class="text-muted">
            Déjela vacía para mantener la contraseña actual.
          </small>
        </div>

        <div class="col-md-3 d-flex align-items-end gap-2">
          <button class="btn btn-primary" :disabled="state.loading">
            {{ state.editingId ? "Actualizar" : "Crear" }}
          </button>
          <button
            v-if="state.editingId"
            type="button"
            class="btn btn-secondary"
            :disabled="state.loading"
            @click="resetForm"
          >
            Cancelar
          </button>
        </div>
      </form>

      <div v-if="state.loading" class="text-center mb-3">
        <div class="spinner-border" role="status">
          <span class="visually-hidden">Cargando...</span>
        </div>
      </div>

      <div v-if="state.users.length > 0" class="table-responsive">
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
            <tr v-for="user in state.users" :key="user.id">
              <td>{{ user.id }}</td>
              <td>{{ user.email }}</td>
              <td>{{ user.createdAt || "-" }}</td>
              <td>
                <button
                  class="btn btn-sm btn-warning me-2"
                  :disabled="state.loading"
                  @click="startEdit(user)"
                >
                  Editar
                </button>
                <button
                  class="btn btn-sm btn-danger"
                  :disabled="state.loading"
                  @click="deleteUser(user.id)"
                >
                  Eliminar
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else-if="!state.loading" class="text-center text-muted">
        No hay usuarios registrados
      </div>
    </div>
  </div>
</template>
