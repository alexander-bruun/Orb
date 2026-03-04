import { writable } from 'svelte/store';

export type ToastType = 'info' | 'warning' | 'error';

export interface Toast {
  id: number;
  message: string;
  type: ToastType;
}

let nextId = 0;

export const toasts = writable<Toast[]>([]);

export function addToast(message: string, type: ToastType = 'info', duration = 4000) {
  const id = nextId++;
  toasts.update((all) => [...all, { id, message, type }]);
  setTimeout(() => {
    toasts.update((all) => all.filter((t) => t.id !== id));
  }, duration);
}

export function dismissToast(id: number) {
  toasts.update((all) => all.filter((t) => t.id !== id));
}
