<script setup lang="ts">
import { onMounted, reactive } from "vue";
import { domainsApiService, usersApiService } from "../store/services";
import type { Domain, User } from "../store/services/models";

const PAGE_LIMIT = 20;

const state = reactive({
  domains: [] as Domain[],
  users: [] as User[],
  loading: false,
  error: "",
  editingId: null as number | null,
  form: { domain: "", userId: 0 },
  currentCursor: "" as string,
  nextCursor: "" as string,
  prevCursors: [] as string[],
  hasNext: false,
  hasPrev: false,
});

function getDefaultUserId(): number {
  return state.users[0]?.id ?? 0;
}

function resetForm(): void {
  state.editingId = null;
  state.form.domain = "";
  state.form.userId = getDefaultUserId();
}

async function loadPage(cursor?: string): Promise<void> {
  state.loading = true;
  state.error = "";

  try {
    const [page, users] = await Promise.all([
      domainsApiService.getDomainPage(cursor, PAGE_LIMIT),
      usersApiService.getAllUsers(),
    ]);
    state.domains = page.data;
    state.users = users;
    state.nextCursor = page.page?.nextCursor ?? "";
    state.hasNext = page.page?.hasMore ?? false;
    state.currentCursor = cursor ?? "";
    state.hasPrev = state.prevCursors.length > 0;

    if (!state.editingId) {
      state.form.userId = state.form.userId || getDefaultUserId();
    }
  } catch (error) {
    state.error =
      error instanceof Error
        ? error.message
        : "No se pudieron cargar los dominios";
  } finally {
    state.loading = false;
  }
}

async function goNext(): Promise<void> {
  if (!state.hasNext || !state.nextCursor) return;
  state.prevCursors = [...state.prevCursors, state.currentCursor];
  await loadPage(state.nextCursor);
}

async function goPrev(): Promise<void> {
  if (state.prevCursors.length === 0) return;
  const prev = state.prevCursors[state.prevCursors.length - 1];
  state.prevCursors = state.prevCursors.slice(0, -1);
  await loadPage(prev || undefined);
}

function startEdit(domain: Domain): void {
  state.editingId = domain.id;
  state.form.domain = domain.domain;
  state.form.userId = domain.userId;
}

async function submitForm(): Promise<void> {
  if (!state.form.domain) {
    state.error = "El dominio es requerido";
    return;
  }

  if (!state.form.userId) {
    state.error = "Debe seleccionar un usuario";
    return;
  }

  state.loading = true;
  state.error = "";

  try {
    if (state.editingId) {
      await domainsApiService.updateDomain(state.editingId, {
        domain: state.form.domain,
        userId: state.form.userId,
      });
    } else {
      await domainsApiService.createDomain({
        domain: state.form.domain,
        userId: state.form.userId,
      });
    }

    resetForm();
    await loadPage(state.currentCursor || undefined);
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "No se pudo guardar el dominio";
  } finally {
    state.loading = false;
  }
}

async function deleteDomain(id: number): Promise<void> {
  if (!confirm("¿Está seguro de que desea eliminar este dominio?")) return;

  state.loading = true;
  state.error = "";

  try {
    await domainsApiService.deleteDomain(id);
    if (state.editingId === id) {
      resetForm();
    }
    await loadPage(state.currentCursor || undefined);
  } catch (error) {
    state.error =
      error instanceof Error ? error.message : "No se pudo eliminar el dominio";
  } finally {
    state.loading = false;
  }
}

function formatTimestamp(date?: string): string {
  if (!date) return "-";
  return new Date(date).toLocaleString();
}

onMounted(() => {
  void loadPage();
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Gestión de Dominios</h5>
    </div>

    <div class="card-body">
      <div v-if="state.error" class="alert alert-danger">
        {{ state.error }}
      </div>

      <div
        v-if="state.users.length === 0 && !state.loading"
        class="alert alert-info"
      >
        Debe crear al menos un usuario antes de registrar dominios.
      </div>

      <form class="row g-3 mb-4" @submit.prevent="submitForm">
        <div class="col-md-5">
          <label class="form-label">Dominio *</label>
          <input
            v-model="state.form.domain"
            type="text"
            class="form-control"
            placeholder="ejemplo.com"
            required
            :disabled="state.users.length === 0"
          />
        </div>

        <div class="col-md-4">
          <label class="form-label">Usuario *</label>
          <select
            v-model="state.form.userId"
            class="form-select"
            required
            :disabled="state.users.length === 0"
          >
            <option :value="0" disabled>Seleccione un usuario</option>
            <option v-for="user in state.users" :key="user.id" :value="user.id">
              {{ user.email }}
            </option>
          </select>
        </div>

        <div class="col-md-3 d-flex align-items-end gap-2">
          <button
            class="btn btn-primary"
            :disabled="state.loading || state.users.length === 0"
          >
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

      <div v-if="state.domains.length > 0" class="table-responsive">
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
            <tr v-for="domain in state.domains" :key="domain.id">
              <td>{{ domain.id }}</td>
              <td>{{ domain.domain }}</td>
              <td>{{ domain.user?.email || "-" }}</td>
              <td>{{ formatTimestamp(domain.createdAt) }}</td>
              <td>
                <button
                  class="btn btn-sm btn-warning me-2"
                  :disabled="state.loading"
                  @click="startEdit(domain)"
                >
                  Editar
                </button>
                <button
                  class="btn btn-sm btn-danger"
                  :disabled="state.loading"
                  @click="deleteDomain(domain.id)"
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
            :disabled="!state.hasPrev || state.loading"
            @click="goPrev"
          >
            Anterior
          </button>
          <button
            class="btn btn-outline-secondary"
            :disabled="!state.hasNext || state.loading"
            @click="goNext"
          >
            Siguiente
          </button>
        </div>
      </div>

      <div v-else-if="!state.loading" class="text-center text-muted">
        No hay dominios registrados
      </div>
    </div>
  </div>
</template>