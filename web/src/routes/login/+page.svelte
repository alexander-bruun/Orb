<script lang="ts">
  import { authStore } from "$lib/stores/auth";
  import { goto } from "$app/navigation";
  import { isNative } from "$lib/utils/platform";
  import { validateEmail } from "$lib/utils/validation";

  let email = "";
  let password = "";
  let error = "";
  let loading = false;

  let emailError = "";
  let passwordError = "";

  function blurEmail() {
    emailError = validateEmail(email);
  }
  function blurPassword() {
    passwordError = password ? "" : "Password is required.";
  }

  // TOTP step state
  let totpRequired = false;
  let tempToken = "";
  let totpCode = "";

  const showChangeServer = isNative();

  async function handleLogin(e: Event) {
    e.preventDefault();
    emailError = validateEmail(email);
    passwordError = password ? "" : "Password is required.";
    if (emailError || passwordError) return;

    error = "";
    loading = true;
    try {
      const result = await authStore.login(email, password);
      if (result.totpRequired && result.tempToken) {
        tempToken = result.tempToken;
        totpRequired = true;
      } else {
        goto("/");
      }
    } catch (err: any) {
      error = err.message ?? "Login failed";
    } finally {
      loading = false;
    }
  }

  async function handleTOTP(e: Event) {
    e.preventDefault();
    error = "";
    loading = true;
    try {
      await authStore.verifyTOTP(tempToken, totpCode, email);
      goto("/");
    } catch (err: any) {
      error = err.message ?? "Invalid code";
      totpCode = "";
    } finally {
      loading = false;
    }
  }

  function backToLogin() {
    totpRequired = false;
    tempToken = "";
    totpCode = "";
    error = "";
    password = "";
  }
</script>

<div class="login-page">
  <div class="login-card">
    <h1 class="logo">orb</h1>

    {#if !totpRequired}
      <p class="subtitle">Sign in to your music library</p>

      {#if error}
        <p class="error">{error}</p>
      {/if}

      <form onsubmit={handleLogin}>
        <label>
          Email
          <input
            type="email"
            bind:value={email}
            onblur={blurEmail}
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
            onblur={blurPassword}
            class:input-error={!!passwordError}
            autocomplete="current-password"
          />
          {#if passwordError}<span class="field-error">{passwordError}</span
            >{/if}
        </label>
        <button type="submit" disabled={loading} class="btn-primary">
          {loading ? "Signing in…" : "Sign in"}
        </button>
      </form>

      {#if showChangeServer}
        <button class="link-btn" onclick={() => goto("/connect")}
          >Change server</button
        >
      {/if}
    {:else}
      <p class="subtitle">Two-factor authentication</p>
      <p class="totp-hint">
        Enter the 6-digit code from your authenticator app, or a backup code.
      </p>

      {#if error}
        <p class="error">{error}</p>
      {/if}

      <form onsubmit={handleTOTP}>
        <label>
          Authentication code
          <input
            type="text"
            inputmode="numeric"
            pattern="[0-9a-fA-F]*"
            maxlength="10"
            placeholder="••••••"
            bind:value={totpCode}
            required
            autocomplete="one-time-code"
            class="totp-input"
          />
        </label>
        <button type="submit" disabled={loading} class="btn-primary">
          {loading ? "Verifying…" : "Verify"}
        </button>
      </form>

      <button class="link-btn" onclick={backToLogin}>← Back to login</button>
    {/if}
  </div>
</div>

<svelte:head><title>Sign In – Orb</title></svelte:head>

<style>
  .login-page {
    min-height: 100dvh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
  }
  .login-card {
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
  .subtitle {
    color: var(--text-muted);
    font-size: 0.875rem;
    margin: 0 0 24px;
  }
  form {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  label {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 0.875rem;
    color: var(--text-muted);
  }
  input {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    color: var(--text);
    font-size: 0.9375rem;
    outline: none;
  }
  input:focus {
    border-color: var(--accent);
  }
  input.input-error {
    border-color: #f87171;
  }
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
  .btn-primary:hover {
    background: var(--accent-hover);
  }
  .btn-primary:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }
  .error {
    color: #f87171;
    font-size: 0.875rem;
  }
  .field-error {
    color: #f87171;
    font-size: 0.8125rem;
    margin-top: -2px;
  }
  .link-btn {
    margin-top: 16px;
    background: none;
    border: none;
    color: var(--text-muted);
    font-size: 0.8125rem;
    cursor: pointer;
    text-decoration: underline;
    padding: 0;
  }
  .link-btn:hover {
    color: var(--accent);
  }
  .totp-hint {
    color: var(--text-muted);
    font-size: 0.8125rem;
    margin: -12px 0 16px;
    line-height: 1.5;
  }
  .totp-input {
    letter-spacing: 0.15em;
    font-size: 1.25rem;
    text-align: center;
    font-family: monospace;
  }
</style>
