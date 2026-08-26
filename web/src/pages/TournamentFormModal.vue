<script setup lang="ts">
import { onBeforeUnmount, reactive, watch } from "vue";
import type { Tournament } from "../store/services/models";

const props = withDefaults(defineProps<{ autoCloseModal?: boolean }>(), {
  autoCloseModal: true,
});

const emit = defineEmits<{
  submit: [payload: { id: number | null; name: string; slug: string }];
}>();

const modal = reactive({
  open: false,
  form: { id: null as number | null, name: "", slug: "" },
  error: "",
  loading: false,
});

function reset(): void {
  modal.open = false;
  modal.error = "";
  modal.loading = false;
  modal.form = { id: null, name: "", slug: "" };
}

function open(entity?: Tournament): void {
  modal.error = "";
  modal.form = entity
    ? { id: entity.id, name: entity.name, slug: entity.slug }
    : { id: null, name: "", slug: "" };
  modal.open = true;
}

async function submit(): Promise<void> {
  if (!modal.form.name) {
    modal.error = "El nombre es requerido";
    return;
  }
  modal.loading = true;
  try {
    emit("submit", { id: modal.form.id, name: modal.form.name, slug: modal.form.slug });
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

defineExpose({ open });
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
            {{ modal.form.id ? "Editar torneo" : "Crear torneo" }}
          </h5>
          <button
            type="button"
            class="btn-close"
            aria-label="Close"
            @click="reset"
          ></button>
        </div>
        <div class="modal-body">
          <form class="row g-3" @submit.prevent="submit">
            <div class="col-12">
              <label class="form-label">Nombre *</label>
              <input
                v-model="modal.form.name"
                type="text"
                class="form-control"
                required
              />
            </div>
            <div class="col-12">
              <label class="form-label">Slug</label>
              <input v-model="modal.form.slug" type="text" class="form-control" />
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
