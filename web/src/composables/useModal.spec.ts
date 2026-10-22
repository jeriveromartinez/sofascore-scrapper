import { afterEach, describe, expect, it, vi } from "vitest";
import { mount, type VueWrapper } from "@vue/test-utils";
import { defineComponent, h, nextTick, ref } from "vue";
import { useModal } from "./useModal";

const TestModal = defineComponent({
  props: { name: { type: String, required: true } },
  emits: ["close"],
  setup(props, { emit, expose }) {
    const isOpen = ref(false);
    const container = ref<HTMLElement | null>(null);
    const onClose = vi.fn(() => {
      isOpen.value = false;
      emit("close");
    });
    useModal({ isOpen: () => isOpen.value, container, onClose });
    expose({ isOpen, onClose });
    return () =>
      isOpen.value
        ? h("div", { ref: container, role: "dialog", tabindex: -1, "data-name": props.name }, [
            h("button", { class: "first" }, "first"),
            h("input", { class: "middle" }),
            h("button", { class: "last" }, "last"),
          ])
        : null;
  },
});

type ModalVm = { isOpen: boolean; onClose: ReturnType<typeof vi.fn> };

const mounted: VueWrapper[] = [];

function mountModal(name: string): { wrapper: VueWrapper; vm: ModalVm } {
  const wrapper = mount(TestModal, { props: { name }, attachTo: document.body });
  mounted.push(wrapper);
  return { wrapper, vm: wrapper.vm as unknown as ModalVm };
}

async function setOpen(vm: ModalVm, open: boolean): Promise<void> {
  vm.isOpen = open;
  await nextTick();
}

function pressKey(key: string, options: KeyboardEventInit = {}): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true, ...options });
  (document.activeElement ?? document.body).dispatchEvent(event);
  return event;
}

function bodyLocked(): boolean {
  return document.body.classList.contains("modal-open") && document.body.style.overflow === "hidden";
}

describe("useModal", () => {
  afterEach(() => {
    while (mounted.length) mounted.pop()!.unmount();
    document.body.innerHTML = "";
  });

  it("locks page scroll while open and releases it on close", async () => {
    const { vm } = mountModal("a");
    expect(bodyLocked()).toBe(false);
    await setOpen(vm, true);
    expect(bodyLocked()).toBe(true);
    await setOpen(vm, false);
    expect(bodyLocked()).toBe(false);
  });

  it("keeps the page locked until the last stacked modal closes", async () => {
    const a = mountModal("a");
    const b = mountModal("b");
    await setOpen(a.vm, true);
    await setOpen(b.vm, true);

    await setOpen(b.vm, false);
    expect(bodyLocked()).toBe(true);

    await setOpen(a.vm, false);
    expect(bodyLocked()).toBe(false);
  });

  it("releases the lock when an open modal is unmounted", async () => {
    const { wrapper, vm } = mountModal("a");
    await setOpen(vm, true);
    wrapper.unmount();
    mounted.splice(mounted.indexOf(wrapper), 1);
    expect(bodyLocked()).toBe(false);
  });

  it("closes only the topmost modal on Escape", async () => {
    const a = mountModal("a");
    const b = mountModal("b");
    await setOpen(a.vm, true);
    await setOpen(b.vm, true);

    pressKey("Escape");
    await nextTick();

    expect(b.vm.onClose).toHaveBeenCalledTimes(1);
    expect(a.vm.onClose).not.toHaveBeenCalled();
    expect(bodyLocked()).toBe(true);
  });

  it("moves focus into the dialog and restores it to the trigger on close", async () => {
    const trigger = document.createElement("button");
    document.body.appendChild(trigger);
    trigger.focus();
    const { wrapper, vm } = mountModal("a");

    await setOpen(vm, true);
    expect(wrapper.find("[role='dialog']").element.contains(document.activeElement)).toBe(true);

    await setOpen(vm, false);
    expect(document.activeElement).toBe(trigger);
  });

  it("wraps Tab and Shift+Tab inside the dialog", async () => {
    const { wrapper, vm } = mountModal("a");
    await setOpen(vm, true);
    const first = wrapper.find(".first").element as HTMLElement;
    const last = wrapper.find(".last").element as HTMLElement;

    last.focus();
    const forward = pressKey("Tab");
    expect(forward.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(first);

    const backward = pressKey("Tab", { shiftKey: true });
    expect(backward.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(last);
  });

  it("ignores keys once every modal is closed", async () => {
    const { vm } = mountModal("a");
    await setOpen(vm, true);
    await setOpen(vm, false);
    pressKey("Escape");
    expect(vm.onClose).not.toHaveBeenCalled();
  });
});
