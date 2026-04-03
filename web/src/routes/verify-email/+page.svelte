<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/stores";
  import { apiFetch } from "$lib/api/client";
  import { authStore } from "$lib/stores/auth";
  import { goto } from "$app/navigation";

  type Status = "loading" | "success" | "error";
  let status: Status = "loading";
  let errorMsg = "";

  onMount(async () => {
    const token = $page.url.searchParams.get("token");
    if (!token) {
      status = "error";
      errorMsg = "No verification token provided.";
      return;
    }
    try {
      await apiFetch(`/auth/verify-email?token=${encodeURIComponent(token)}`);
      status = "success";
      // If the user is already logged in, update their verified state in the store.
      authStore.updateEmailVerified(true);
    } catch (err: any) {
      status = "error";
      errorMsg =
        err?.message ??
        "Verification failed. The link may be invalid or expired.";
    }
  });
</script>

<div class="verify-wrap">
  <div class="verify-card">
    <div class="logo">Orb</div>

    {#if status === "loading"}
      <p class="hint">Verifying your email address…</p>
    {:else if status === "success"}
      <div class="icon success-icon">✓</div>
      <h1>Email verified</h1>
      <p class="hint">
        Your email address has been confirmed. You can now close this page or
        return to the app.
      </p>
      <button class="btn-primary" on:click={() => goto("/")}>Go to app</button>
    {:else}
      <div class="icon error-icon">✕</div>
      <h1>Verification failed</h1>
      <p class="hint">{errorMsg}</p>
      <button class="btn-primary" on:click={() => goto("/settings")}
        >Back to settings</button
      >
    {/if}
  </div>
</div>

<style>
  .verify-wrap {
    min-height: 100svh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg-base, #0f0f18);
    padding: 24px;
  }

  .verify-card {
    width: 100%;
    max-width: 420px;
    background: var(--surface, #1e1e2e);
    border-radius: 16px;
    padding: 40px 32px;
    text-align: center;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 16px;
  }

  .logo {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--accent, #a78bfa);
    letter-spacing: 0.05em;
  }

  h1 {
    font-size: 1.25rem;
    font-weight: 600;
    margin: 0;
    color: var(--text-primary, #e2e8f0);
  }

  .hint {
    color: var(--text-muted, #94a3b8);
    font-size: 0.9rem;
    line-height: 1.6;
    margin: 0;
  }

  .icon {
    width: 56px;
    height: 56px;
    border-radius: 50%;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 1.5rem;
    font-weight: 700;
  }

  .success-icon {
    background: rgba(34, 197, 94, 0.15);
    color: #22c55e;
  }

  .error-icon {
    background: rgba(239, 68, 68, 0.15);
    color: #ef4444;
  }

  .btn-primary {
    margin-top: 8px;
    padding: 10px 24px;
    background: var(--accent, #a78bfa);
    color: #fff;
    border: none;
    border-radius: 8px;
    font-size: 0.9rem;
    font-weight: 600;
    cursor: pointer;
    transition: opacity 0.15s;
  }

  .btn-primary:hover {
    opacity: 0.88;
  }
</style>
