<script lang="ts">
  import { authStore } from '$lib/stores/auth';
  import { setupRequired } from '$lib/stores/setup';
  import { goto } from '$app/navigation';

  let username = '';
  let email = '';
  let password = '';
  let error = '';
  let loading = false;

  async function handleSetup(e: Event) {
    e.preventDefault();
    error = '';
    loading = true;
    try {
      await authStore.register(username, email, password);
      await authStore.login(email, password);
      setupRequired.set(false);
      goto('/');
    } catch (err: any) {
      error = err.message ?? 'Setup failed';
    } finally {
      loading = false;
    }
  }
</script>

<div class="setup-page">
  <div class="setup-card">
    <h1 class="logo">orb</h1>
    <p class="subtitle">Create your admin account to get started</p>

    {#if error}
      <p class="error">{error}</p>
    {/if}

    <form on:submit={handleSetup}>
      <label>
        Username
        <input type="text" bind:value={username} required autocomplete="username" />
      </label>
      <label>
        Email
        <input type="email" bind:value={email} required autocomplete="email" />
      </label>
      <label>
        Password
        <input type="password" bind:value={password} required autocomplete="new-password" />
      </label>
      <button type="submit" disabled={loading} class="btn-primary">
        {loading ? 'Creating account…' : 'Create account'}
      </button>
    </form>
  </div>
</div>

<style>
  .setup-page {
    min-height: 100dvh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
  }
  .setup-card {
    width: 100%;
    max-width: 360px;
    padding: 40px;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 12px;
  }
  .logo {
    font-size: 2rem;
    font-weight: 700;
    color: var(--accent);
    letter-spacing: -0.05em;
    margin: 0 0 8px;
  }
  .subtitle { color: var(--text-muted); font-size: 0.875rem; margin: 0 0 24px; }
  form { display: flex; flex-direction: column; gap: 16px; }
  label { display: flex; flex-direction: column; gap: 6px; font-size: 0.875rem; color: var(--text-muted); }
  input {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    color: var(--text);
    font-size: 0.9375rem;
    outline: none;
  }
  input:focus { border-color: var(--accent); }
  .btn-primary {
    background: var(--accent);
    border: none;
    border-radius: 6px;
    padding: 10px;
    color: #fff;
    font-size: 0.9375rem;
    font-weight: 600;
    cursor: pointer;
    transition: background 0.15s;
  }
  .btn-primary:hover { background: var(--accent-hover); }
  .btn-primary:disabled { opacity: 0.6; cursor: not-allowed; }
  .error { color: #f87171; font-size: 0.875rem; }
</style>
