import { onBeforeUnmount, onMounted, watch, type Ref } from "vue";

export interface UseModalOptions {
  isOpen: () => boolean;
  container: Readonly<Ref<HTMLElement | null>>;
  onClose: () => void;
}

interface ActiveModal {
  container: Readonly<Ref<HTMLElement | null>>;
  onClose: () => void;
  restoreFocus: HTMLElement | null;
}

const FOCUSABLE_SELECTOR = [
  "a[href]",
  "button:not([disabled])",
  "input:not([disabled]):not([type='hidden'])",
  "select:not([disabled])",
  "textarea:not([disabled])",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

// Shared by every modal so stacked modals close top-first and the page stays
// locked until the last one closes.
const openModals: ActiveModal[] = [];

function focusableIn(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
}

function trapTab(event: KeyboardEvent, container: HTMLElement): void {
  const focusable = focusableIn(container);
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (!first || !last) {
    event.preventDefault();
    container.focus();
    return;
  }
  const active = document.activeElement;
  const outside = active === container || !container.contains(active);
  if (event.shiftKey && (outside || active === first)) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && (outside || active === last)) {
    event.preventDefault();
    first.focus();
  }
}

function onKeydown(event: KeyboardEvent): void {
  const top = openModals[openModals.length - 1];
  const container = top?.container.value;
  if (!top || !container) return;
  if (event.key === "Escape") {
    event.preventDefault();
    top.onClose();
  } else if (event.key === "Tab") {
    trapTab(event, container);
  }
}

function lockPage(): void {
  document.body.classList.add("modal-open");
  document.body.style.overflow = "hidden";
  document.addEventListener("keydown", onKeydown);
}

function unlockPage(): void {
  document.body.classList.remove("modal-open");
  document.body.style.overflow = "";
  document.removeEventListener("keydown", onKeydown);
}

export function useModal({ isOpen, container, onClose }: UseModalOptions): void {
  let entry: ActiveModal | null = null;

  function activate(): void {
    if (entry) return;
    const active = document.activeElement;
    entry = { container, onClose, restoreFocus: active instanceof HTMLElement ? active : null };
    if (openModals.length === 0) lockPage();
    openModals.push(entry);
    container.value?.focus();
  }

  function deactivate(): void {
    if (!entry) return;
    openModals.splice(openModals.indexOf(entry), 1);
    if (openModals.length === 0) unlockPage();
    if (entry.restoreFocus?.isConnected) entry.restoreFocus.focus();
    entry = null;
  }

  onMounted(() => {
    if (isOpen()) activate();
  });
  watch(isOpen, (open) => (open ? activate() : deactivate()), { flush: "post" });
  onBeforeUnmount(deactivate);
}
