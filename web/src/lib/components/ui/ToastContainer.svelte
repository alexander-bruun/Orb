<script lang="ts">
  import { toasts, dismissToast } from '$lib/stores/ui/toast';
</script>

{#if $toasts.length > 0}
  <div class="toast-container">
    {#each $toasts as toast (toast.id)}
      <div class="toast toast--{toast.type}" role="alert">
        <span class="toast-message">{toast.message}</span>
        <button class="toast-close" onclick={() => dismissToast(toast.id)} aria-label="Dismiss">×</button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .toast-container {
    position: fixed;
    bottom: calc(var(--bottom-h, 80px) + 12px);
    left: 50%;
    transform: translateX(-50%);
    display: flex;
    flex-direction: column;
    gap: 8px;
    z-index: 9999;
    pointer-events: none;
    align-items: center;
  }

  .toast {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 16px;
    border-radius: 8px;
    font-size: 0.875rem;
    font-weight: 500;
    pointer-events: all;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
    animation: slide-in 0.18s ease;
    min-width: 220px;
    max-width: 380px;
  }

  .toast--info {
    background: var(--surface, #222);
    color: var(--text, #eee);
    border: 1px solid var(--border, #444);
  }

  .toast--warning {
    background: #3a2e00;
    color: #f5c842;
    border: 1px solid #5a4a00;
  }

  .toast--error {
    background: #3a0000;
    color: #f58282;
    border: 1px solid #6a0000;
  }

  .toast-message {
    flex: 1;
  }

  .toast-close {
    background: none;
    border: none;
    color: inherit;
    cursor: pointer;
    font-size: 1.1rem;
    line-height: 1;
    opacity: 0.6;
    padding: 0 2px;
  }

  .toast-close:hover {
    opacity: 1;
  }

  @keyframes slide-in {
    from { opacity: 0; transform: translateY(8px); }
    to   { opacity: 1; transform: translateY(0); }
  }
</style>
