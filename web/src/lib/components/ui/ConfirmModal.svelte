<script lang="ts">
  interface Props {
    open?: boolean;
    title?: string;
    message?: string;
    confirmText?: string;
    cancelText?: string;
    variant?: "default" | "danger";
    loading?: boolean;
    onConfirm?: () => void;
    onCancel?: () => void;
  }

  let {
    open = $bindable(false),
    title = "Confirm",
    message = "Are you sure?",
    confirmText = "Confirm",
    cancelText = "Cancel",
    variant = "default",
    loading = false,
    onConfirm,
    onCancel,
  }: Props = $props();

  function handleConfirm() {
    onConfirm?.();
  }

  function handleCancel() {
    open = false;
    onCancel?.();
  }

  function handleBackdropClick(e: MouseEvent) {
    if (e.target === e.currentTarget) {
      handleCancel();
    }
  }
</script>

{#if open}
  <div class="backdrop" onclick={handleBackdropClick} role="presentation">
    <div class="modal">
      <div class="header">
        <h2>{title}</h2>
      </div>
      <div class="body">
        {message}
      </div>
      <div class="footer">
        <button
          class="btn btn-secondary"
          onclick={handleCancel}
          disabled={loading}
        >
          {cancelText}
        </button>
        <button
          class="btn"
          class:btn-danger={variant === "danger"}
          class:btn-primary={variant === "default"}
          onclick={handleConfirm}
          disabled={loading}
        >
          {#if loading}
            <div class="spinner"></div>
          {/if}
          {confirmText}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
    animation: fadeIn 0.15s ease-out;
  }

  @keyframes fadeIn {
    from {
      opacity: 0;
    }
    to {
      opacity: 1;
    }
  }

  .modal {
    background: var(--bg);
    border-radius: 12px;
    box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.3);
    max-width: 400px;
    width: 90%;
    animation: slideUp 0.2s ease-out;
  }

  @keyframes slideUp {
    from {
      transform: translateY(20px);
      opacity: 0;
    }
    to {
      transform: translateY(0);
      opacity: 1;
    }
  }

  .header {
    padding: 20px 20px 12px;
    border-bottom: 1px solid var(--border);
  }

  .header h2 {
    margin: 0;
    font-size: 1.1rem;
    font-weight: 600;
  }

  .body {
    padding: 16px 20px;
    color: var(--text-muted);
    line-height: 1.5;
  }

  .footer {
    display: flex;
    gap: 8px;
    padding: 16px 20px;
    justify-content: flex-end;
    border-top: 1px solid var(--border);
  }

  .btn {
    padding: 8px 16px;
    border: none;
    border-radius: 6px;
    font-size: 0.9rem;
    font-weight: 500;
    cursor: pointer;
    transition: all 0.15s;
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .btn-primary {
    background: var(--accent);
    color: white;
  }

  .btn-primary:hover:not(:disabled) {
    opacity: 0.9;
  }

  .btn-danger {
    background: #ef4444;
    color: white;
  }

  .btn-danger:hover:not(:disabled) {
    background: #dc2626;
  }

  .btn-secondary {
    background: var(--bg-elevated);
    color: var(--text);
    border: 1px solid var(--border);
  }

  .btn-secondary:hover:not(:disabled) {
    background: color-mix(in srgb, var(--bg-elevated) 80%, var(--accent) 20%);
  }

  .spinner {
    width: 12px;
    height: 12px;
    border: 2px solid rgba(255, 255, 255, 0.4);
    border-top-color: white;
    border-radius: 50%;
    animation: spin 0.7s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
