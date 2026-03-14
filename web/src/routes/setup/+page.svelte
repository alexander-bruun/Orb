<script lang="ts">
  import { authStore } from '$lib/stores/auth';
  import { setupRequired } from '$lib/stores/auth/setup';
  import { goto } from '$app/navigation';
  import { validateEmail, validateUsername, validatePassword } from '$lib/utils/validation';

  let username = '';
  let email = '';
  let password = '';
  let error = '';
  let loading = false;

  let usernameError = '';
  let emailError = '';
  let passwordError = '';

  function blurUsername() { usernameError = validateUsername(username); }
  function blurEmail()    { emailError    = validateEmail(email); }
  function blurPassword() { passwordError = validatePassword(password); }

  async function handleSetup(e: Event) {
    e.preventDefault();
    usernameError = validateUsername(username);
    emailError    = validateEmail(email);
    passwordError = validatePassword(password);
    if (usernameError || emailError || passwordError) return;

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
        <input
          type="text"
          bind:value={username}
          on:blur={blurUsername}
          class:input-error={!!usernameError}
          autocomplete="username"
        />
        {#if usernameError}<span class="field-error">{usernameError}</span>{/if}
      </label>
      <label>
        Email
        <input
          type="email"
          bind:value={email}
          on:blur={blurEmail}
          class:input-error={!!emailError}
          autocomplete="email"
        />
        {#if emailError}<span class="field-error">{emailError}</span>{/if}
      </label>
      <label>
        Password
        <input
          type="password"
          bind:value={password}
          on:blur={blurPassword}
          class:input-error={!!passwordError}
          autocomplete="new-password"
        />
        {#if passwordError}<span class="field-error">{passwordError}</span>{/if}
        {#if !passwordError && password}
          <span class="field-hint">Strong password</span>
        {/if}
      </label>
      <button type="submit" disabled={loading} class="btn-primary">
        {loading ? 'Creating account…' : 'Create account'}
      </button>
    </form>
  </div>
</div>

<svelte:head><title>Setup – Orb</title></svelte:head>

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
  input.input-error { border-color: #f87171; }
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
  .field-error { color: #f87171; font-size: 0.8125rem; margin-top: -2px; }
  .field-hint { color: #4ade80; font-size: 0.8125rem; margin-top: -2px; }
</style>
