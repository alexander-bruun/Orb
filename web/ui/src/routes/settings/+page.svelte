<script lang="ts">
  import { authStore } from '$lib/stores/auth';
  import { themeStore, avatarStore, ACCENTS } from '$lib/stores/theme';
  import { apiFetch } from '$lib/api/client';

  // ── Avatar ────────────────────────────────────────────────
  let fileInput: HTMLInputElement;
  let uploadError = '';

  async function handleAvatarUpload(e: Event) {
    uploadError = '';
    const file = (e.target as HTMLInputElement).files?.[0];
    if (!file) return;

    if (!file.type.startsWith('image/')) {
      uploadError = 'Please select an image file.';
      return;
    }

    const img = new Image();
    const blobUrl = URL.createObjectURL(file);
    img.src = blobUrl;
    await new Promise<void>(resolve => { img.onload = () => resolve(); });

    const canvas = document.createElement('canvas');
    canvas.width = 128;
    canvas.height = 128;
    const ctx = canvas.getContext('2d')!;
    const size = Math.min(img.width, img.height);
    const sx = (img.width  - size) / 2;
    const sy = (img.height - size) / 2;
    ctx.drawImage(img, sx, sy, size, size, 0, 0, 128, 128);
    URL.revokeObjectURL(blobUrl);

    avatarStore.set(canvas.toDataURL('image/jpeg', 0.88));
  }

  function clearAvatar() {
    avatarStore.clear();
    if (fileInput) fileInput.value = '';
  }

  // ── Change password ───────────────────────────────────────
  let pwCurrent = '';
  let pwNew = '';
  let pwConfirm = '';
  let pwLoading = false;
  let pwError = '';
  let pwSuccess = false;

  async function submitPassword() {
    pwError = '';
    pwSuccess = false;
    if (!pwCurrent || !pwNew || !pwConfirm) {
      pwError = 'All fields are required.';
      return;
    }
    if (pwNew.length < 8) {
      pwError = 'New password must be at least 8 characters.';
      return;
    }
    if (pwNew !== pwConfirm) {
      pwError = 'New passwords do not match.';
      return;
    }
    pwLoading = true;
    try {
      await apiFetch('/auth/password', {
        method: 'PATCH',
        body: JSON.stringify({ current_password: pwCurrent, new_password: pwNew })
      });
      pwCurrent = pwNew = pwConfirm = '';
      pwSuccess = true;
    } catch (err: any) {
      pwError = err?.message ?? 'Failed to change password.';
    } finally {
      pwLoading = false;
    }
  }

  // ── Change email ──────────────────────────────────────────
  let emailNew = '';
  let emailPw = '';
  let emailLoading = false;
  let emailError = '';
  let emailSuccess = false;

  async function submitEmail() {
    emailError = '';
    emailSuccess = false;
    if (!emailNew || !emailPw) {
      emailError = 'All fields are required.';
      return;
    }
    if (!emailNew.includes('@')) {
      emailError = 'Enter a valid email address.';
      return;
    }
    emailLoading = true;
    try {
      await apiFetch('/auth/email', {
        method: 'PATCH',
        body: JSON.stringify({ new_email: emailNew, current_password: emailPw })
      });
      authStore.updateEmail(emailNew);
      emailNew = emailPw = '';
      emailSuccess = true;
    } catch (err: any) {
      emailError = err?.message ?? 'Failed to change email.';
    } finally {
      emailLoading = false;
    }
  }

  $: initials = ($authStore.user?.username ?? 'U').slice(0, 2).toUpperCase();
</script>

<div class="page">
  <h1 class="page-title">Settings</h1>

  <!-- ── Profile ──────────────────────────────────────────── -->
  <section class="card">
    <h2 class="section-title">Profile</h2>

    <div class="avatar-area">
      <div class="big-avatar">
        {#if $avatarStore}
          <img src={$avatarStore} alt="" class="avatar-img" />
        {:else}
          <span class="avatar-initials">{initials}</span>
        {/if}
      </div>
      <div class="avatar-actions">
        <button class="btn-primary" on:click={() => fileInput.click()}>
          Upload photo
        </button>
        {#if $avatarStore}
          <button class="btn-ghost" on:click={clearAvatar}>Remove</button>
        {/if}
        {#if uploadError}
          <span class="msg msg--error">{uploadError}</span>
        {/if}
        <input
          bind:this={fileInput}
          type="file"
          accept="image/*"
          style="display:none"
          on:change={handleAvatarUpload}
        />
      </div>
    </div>

    <div class="fields">
      <div class="field">
        <span class="field-label">Username</span>
        <div class="field-value">{$authStore.user?.username ?? '—'}</div>
      </div>
      <div class="field">
        <span class="field-label">Email</span>
        <div class="field-value">{$authStore.user?.email ?? '—'}</div>
      </div>
    </div>
  </section>

  <!-- ── Change password ────────────────────────────────────── -->
  <section class="card">
    <h2 class="section-title">Change password</h2>

    <div class="form-grid">
      <label class="form-label" for="pw-current">Current password</label>
      <input
        id="pw-current"
        class="form-input"
        type="password"
        autocomplete="current-password"
        bind:value={pwCurrent}
        disabled={pwLoading}
      />

      <label class="form-label" for="pw-new">New password</label>
      <input
        id="pw-new"
        class="form-input"
        type="password"
        autocomplete="new-password"
        placeholder="min. 8 characters"
        bind:value={pwNew}
        disabled={pwLoading}
      />

      <label class="form-label" for="pw-confirm">Confirm new</label>
      <input
        id="pw-confirm"
        class="form-input"
        type="password"
        autocomplete="new-password"
        bind:value={pwConfirm}
        disabled={pwLoading}
      />
    </div>

    {#if pwError}
      <p class="msg msg--error">{pwError}</p>
    {/if}
    {#if pwSuccess}
      <p class="msg msg--ok">Password updated successfully.</p>
    {/if}

    <div>
      <button class="btn-primary" on:click={submitPassword} disabled={pwLoading}>
        {pwLoading ? 'Saving…' : 'Update password'}
      </button>
    </div>
  </section>

  <!-- ── Change email ──────────────────────────────────────── -->
  <section class="card">
    <h2 class="section-title">Change email</h2>

    <div class="form-grid">
      <label class="form-label" for="email-new">New email</label>
      <input
        id="email-new"
        class="form-input"
        type="email"
        autocomplete="email"
        bind:value={emailNew}
        disabled={emailLoading}
      />

      <label class="form-label" for="email-pw">Current password</label>
      <input
        id="email-pw"
        class="form-input"
        type="password"
        autocomplete="current-password"
        bind:value={emailPw}
        disabled={emailLoading}
      />
    </div>

    {#if emailError}
      <p class="msg msg--error">{emailError}</p>
    {/if}
    {#if emailSuccess}
      <p class="msg msg--ok">Email updated successfully.</p>
    {/if}

    <div>
      <button class="btn-primary" on:click={submitEmail} disabled={emailLoading}>
        {emailLoading ? 'Saving…' : 'Update email'}
      </button>
    </div>
  </section>

  <!-- ── Appearance ─────────────────────────────────────── -->
  <section class="card">
    <h2 class="section-title">Appearance</h2>

    <div class="setting-row">
      <div class="setting-info">
        <span class="setting-name">Color mode</span>
        <span class="setting-desc">Choose between dark and light interface</span>
      </div>
      <div class="mode-toggle">
        <button
          class="mode-btn"
          class:active={$themeStore.mode === 'dark'}
          on:click={() => themeStore.setMode('dark')}
        >
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/>
          </svg>
          Dark
        </button>
        <button
          class="mode-btn"
          class:active={$themeStore.mode === 'light'}
          on:click={() => themeStore.setMode('light')}
        >
          <svg width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
            <circle cx="12" cy="12" r="5"/>
            <line x1="12" y1="1"  x2="12" y2="3"/>
            <line x1="12" y1="21" x2="12" y2="23"/>
            <line x1="4.22" y1="4.22"  x2="5.64" y2="5.64"/>
            <line x1="18.36" y1="18.36" x2="19.78" y2="19.78"/>
            <line x1="1" y1="12" x2="3" y2="12"/>
            <line x1="21" y1="12" x2="23" y2="12"/>
            <line x1="4.22" y1="19.78" x2="5.64" y2="18.36"/>
            <line x1="18.36" y1="5.64" x2="19.78" y2="4.22"/>
          </svg>
          Light
        </button>
      </div>
    </div>

    <div class="setting-row setting-row--col">
      <div class="setting-info">
        <span class="setting-name">Accent color</span>
        <span class="setting-desc">Used for highlights, active states, and interactive elements</span>
      </div>
      <div class="palette">
        {#each ACCENTS as accent}
          <button
            class="swatch"
            class:selected={$themeStore.accent === accent.value}
            style="--swatch: {accent.value}; --swatch-glow: rgba({accent.rgb},0.4);"
            title={accent.name}
            on:click={() => themeStore.setAccent(accent.value)}
            aria-label={accent.name}
            aria-pressed={$themeStore.accent === accent.value}
          >
            {#if $themeStore.accent === accent.value}
              <svg width="10" height="10" fill="none" stroke="white" stroke-width="2.5" viewBox="0 0 24 24">
                <polyline points="20 6 9 17 4 12"/>
              </svg>
            {/if}
          </button>
        {/each}
      </div>
    </div>
  </section>
</div>

<style>
  .page {
    max-width: 560px;
    padding: 8px 0 32px;
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .page-title {
    font-size: 1.5rem;
    font-weight: 700;
    color: var(--text);
    letter-spacing: -0.02em;
    margin-bottom: 4px;
  }

  /* ── Card ── */
  .card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 12px;
    padding: 20px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .section-title {
    font-size: 0.8rem;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-2);
  }

  /* ── Profile ── */
  .avatar-area {
    display: flex;
    align-items: center;
    gap: 16px;
  }

  .big-avatar {
    width: 72px;
    height: 72px;
    border-radius: 50%;
    background: linear-gradient(135deg, var(--accent), #818cf8);
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    overflow: hidden;
    box-shadow: 0 0 0 3px var(--accent-dim);
  }

  .avatar-img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }

  .avatar-initials {
    font-size: 22px;
    font-weight: 700;
    color: white;
    font-family: 'Syne', sans-serif;
  }

  .avatar-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .fields {
    display: flex;
    flex-direction: column;
    gap: 10px;
    border-top: 1px solid var(--border);
    padding-top: 14px;
  }

  .field {
    display: flex;
    align-items: baseline;
    gap: 12px;
  }

  .field-label {
    font-size: 12px;
    color: var(--text-2);
    width: 72px;
    flex-shrink: 0;
  }

  .field-value {
    font-size: 13px;
    color: var(--text);
    font-family: 'DM Mono', monospace;
  }

  /* ── Form ── */
  .form-grid {
    display: grid;
    grid-template-columns: 140px 1fr;
    align-items: center;
    gap: 10px 12px;
  }

  .form-label {
    font-size: 12px;
    color: var(--text-2);
    text-align: right;
  }

  .form-input {
    width: 100%;
    height: 34px;
    background: var(--surface-2);
    border: 1px solid var(--border-2);
    border-radius: 7px;
    padding: 0 10px;
    color: var(--text);
    font-size: 13px;
    font-family: 'DM Mono', monospace;
    outline: none;
    transition: border-color 0.15s;
  }
  .form-input:focus { border-color: var(--accent); }
  .form-input:disabled { opacity: 0.5; }

  /* ── Messages ── */
  .msg {
    font-size: 12px;
    margin: 0;
  }
  .msg--error { color: #f87171; }
  .msg--ok    { color: #4ade80; }

  /* ── Buttons ── */
  .btn-primary {
    padding: 7px 16px;
    border-radius: 7px;
    border: none;
    background: var(--accent);
    color: #fff;
    font-size: 12px;
    font-weight: 600;
    font-family: 'Syne', sans-serif;
    cursor: pointer;
    transition: opacity 0.15s;
  }
  .btn-primary:hover:not(:disabled) { opacity: 0.85; }
  .btn-primary:disabled { opacity: 0.5; cursor: default; }

  .btn-ghost {
    padding: 7px 14px;
    border-radius: 7px;
    border: 1px solid var(--border-2);
    background: transparent;
    color: var(--text-2);
    font-size: 12px;
    font-family: 'Syne', sans-serif;
    cursor: pointer;
    transition: color 0.15s, border-color 0.15s;
  }
  .btn-ghost:hover { color: var(--text); border-color: var(--accent); }

  /* ── Appearance ── */
  .setting-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 14px 0;
    border-top: 1px solid var(--border);
  }
  .setting-row:first-of-type { border-top: none; padding-top: 0; }
  .setting-row--col {
    flex-direction: column;
    align-items: flex-start;
  }

  .setting-info {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .setting-name {
    font-size: 13px;
    font-weight: 600;
    color: var(--text);
  }

  .setting-desc {
    font-size: 11px;
    color: var(--text-2);
  }

  .mode-toggle {
    display: flex;
    border: 1px solid var(--border-2);
    border-radius: 8px;
    overflow: hidden;
    flex-shrink: 0;
  }

  .mode-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 14px;
    background: none;
    border: none;
    font-size: 12px;
    font-weight: 500;
    font-family: 'Syne', sans-serif;
    color: var(--text-2);
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }
  .mode-btn + .mode-btn { border-left: 1px solid var(--border-2); }
  .mode-btn.active { background: var(--accent-dim); color: var(--accent); }
  .mode-btn:hover:not(.active) { background: var(--surface-2); color: var(--text); }

  .palette {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
    margin-top: 8px;
  }

  .swatch {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    background: var(--swatch);
    border: 2px solid transparent;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: transform 0.15s, box-shadow 0.15s, border-color 0.15s;
    flex-shrink: 0;
  }
  .swatch:hover { transform: scale(1.1); box-shadow: 0 0 0 4px var(--swatch-glow); }
  .swatch.selected { border-color: white; box-shadow: 0 0 0 3px var(--swatch-glow); transform: scale(1.05); }
</style>
