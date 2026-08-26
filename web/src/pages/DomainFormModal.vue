<script setup lang="ts">
import { onBeforeUnmount, reactive, watch } from "vue";
import type { Domain, User } from "../store/services/models";

const props = withDefaults(
  defineProps<{ autoCloseModal?: boolean; users: User[] }>(),
  { autoCloseModal: true },
);

const emit = defineEmits<{
  submit: [payload: { id: number | null; domain: string; userId: number }];
}>();

const modal = reactive({
  open: false,
  form: { id: null as number | null, domain: "", userId: 0 },
  error: "",
  loading: false,
});

function defaultUserId(): number {
  return props.users[0]?.id ?? 0;
}

function reset(): void {
  modal.open = false;
  modal.error = "";
  modal.loading = false;
  modal.form = { id: null, domain: "", userId: defaultUserId() };
}

function open(entity?: Domain): void {
  modal.error = "";
  modal.form = entity
    ? { id: entity.id, domain: entity.domain, userId: entity.userId }
    : { id: null, domain: "", userId: defaultUserId() };
  modal.open = true;
}

async function submit(): Promise<void> {
  if (!modal.form.domain) {
    modal.error = "El dominio es requerido";
    return;
  }
  if (!modal.form.userId) {
    modal.error = "Debe seleccionar un usuario";
    return;
  }
  modal.loading = true;
  try {
    emit("submit", {
      id: modal.form.id,
      domain: modal.form.domain,
      userId: modal.form.userId,
    });
    if (props.autoCloseModal) reset();
  } finally {
    modal.loading = false;
  }
}

watch(
  () => modal.open,
  (isOpen) => {
    document.body.classList.toggle("modal-open", isOpen);
    document.body.style.overflow = isOpen ? "hidden" : "";
  },
);

onBeforeUnmount(() => {
  document.body.classList.remove("modal-open");
  document.body.style.overflow = "";
});

defineExpose({ open, reset });
</script>

<template>
  <div
    v-if="modal.open"
    class="modal fade show"
    tabindex="-1"
    style="display: block"
    aria-modal="true"
    role="dialog"
  >
    <div class="modal-dialog modal-dialog-centered">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title">
            {{ modal.form.id ? "Editar dominio" : "Crear dominio" }}
          </h5>
          <button type="button" class="btn-close" aria-label="Close" @click="reset"></button>
        </div>
        <div class="modal-body">
          <form class="row g-3" @submit.prevent="submit">
            <div class="col-12">
              <label class="form-label">Dominio *</label>
              <input
                v-model="modal.form.domain"
                type="text"
                class="form-control"
                placeholder="ejemplo.com"
                required
                :disabled="props.users.length === 0"
              />
            </div>
            <div class="col-12">
              <label class="form-label">Usuario *</label>
              <select
                v-model="modal.form.userId"
                class="form-select"
                required
                :disabled="props.users.length === 0"
              >
                <option :value="0" disabled>Seleccione un usuario</option>
                <option v-for="user in props.users" :key="user.id" :value="user.id">
                  {{ user.email }}
                </option>
              </select>
            </div>
            <div v-if="modal.error" class="col-12">
              <div class="alert alert-danger mb-0">{{ modal.error }}</div>
            </div>
          </form>
        </div>
        <div class="modal-footer">
          <button
            type="button"
            class="btn btn-label-secondary"
            :disabled="modal.loading"
            @click="reset"
          >
            Cancelar
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            :disabled="modal.loading || props.users.length === 0"
            @click="submit"
          >
            {{ modal.form.id ? "Actualizar" : "Crear" }}
          </button>
        </div>
      </div>
    </div>
  </div>
  <div v-if="modal.open" class="modal-backdrop fade show"></div>
</template>