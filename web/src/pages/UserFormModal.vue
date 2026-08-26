<script setup lang="ts">
import { onBeforeUnmount, reactive, watch } from "vue";
import type { User } from "../store/services/models";

const props = withDefaults(defineProps<{ autoCloseModal?: boolean }>(), {
  autoCloseModal: true,
});

const emit = defineEmits<{
  submit: [payload: { id: number | null; email: string; password: string }];
}>();

const modal = reactive({
  open: false,
  form: { id: null as number | null, email: "", password: "" },
  error: "",
  loading: false,
});

function reset(): void {
  modal.open = false;
  modal.error = "";
  modal.loading = false;
  modal.form = { id: null, email: "", password: "" };
}

function open(entity?: User): void {
  modal.error = "";
  modal.form = entity
    ? { id: entity.id, email: entity.email, password: "" }
    : { id: null, email: "", password: "" };
  modal.open = true;
}

async function submit(): Promise<void> {
  if (!modal.form.email) {
    modal.error = "El email es requerido";
    return;
  }
  if (!modal.form.id && !modal.form.password) {
    modal.error = "La contraseña es requerida para crear un usuario";
    return;
  }
  modal.loading = true;
  try {
    emit("submit", {
      id: modal.form.id,
      email: modal.form.email,
      password: modal.form.password,
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
            {{ modal.form.id ? "Editar usuario" : "Crear usuario" }}
          </h5>
          <button type="button" class="btn-close" aria-label="Close" @click="reset"></button>
        </div>
        <div class="modal-body">
          <form class="row g-3" @submit.prevent="submit">
            <div class="col-12">
              <label class="form-label">Email *</label>
              <input v-model="modal.form.email" type="email" class="form-control" required />
            </div>
            <div class="col-12">
              <label class="form-label">
                {{ modal.form.id ? "Nueva contraseña" : "Contraseña *" }}
              </label>
              <input
                v-model="modal.form.password"
                type="password"
                class="form-control"
                :required="!modal.form.id"
              />
              <small v-if="modal.form.id" class="text-muted">
                Déjela vacía para mantener la contraseña actual.
              </small>
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
            :disabled="modal.loading"
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
