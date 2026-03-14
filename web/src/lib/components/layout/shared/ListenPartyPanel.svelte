<script lang="ts">
  import { onMount } from 'svelte';
  import QRCode from 'qrcode';
  import {
    lpRole,
    lpSessionId,
    lpParticipants,
    lpPanelOpen,
    lpConnected,
    lpCodeEnabled,
    lpAccessCode,
    hostKick,
    hostEndSession,
    hostEnableCode,
    hostDisableCode,
    createAndConnect,
  } from '$lib/stores/social/listenParty';

  const APP_BASE = typeof location !== 'undefined' ? location.origin : '';

  let copied = $state(false);
  let codeCopied = $state(false);
  let togglingCode = $state(false);
  let qrDataUrl = $state('');

  let inviteUrl = $derived(
    $lpSessionId
      ? `${APP_BASE}/listen/${$lpSessionId}`
      : ''
  );

  $effect(() => {
    if (inviteUrl) {
      QRCode.toDataURL(inviteUrl, { width: 200, margin: 1 }).then(url => {
        qrDataUrl = url;
      });
    } else {
      qrDataUrl = '';
    }
  });

  function fallbackCopy(text: string): boolean {
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.left = '-9999px';
    document.body.appendChild(ta);
    ta.select();
    let ok = false;
    try { ok = document.execCommand('copy'); } catch { ok = false; }
    document.body.removeChild(ta);
    return ok;
  }

  async function copyToClipboard(text: string): Promise<void> {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
    } else {
      if (!fallbackCopy(text)) throw new Error('copy failed');
    }
  }

  async function copyLink() {
    if (!inviteUrl) return;
    await copyToClipboard(inviteUrl).catch(() => {});
    copied = true;
    setTimeout(() => (copied = false), 2000);
  }

  async function copyCode() {
    if (!$lpAccessCode) return;
    await copyToClipboard($lpAccessCode).catch(() => {});
    codeCopied = true;
    setTimeout(() => (codeCopied = false), 2000);
  }

  async function toggleCode() {
    togglingCode = true;
    try {
      if ($lpCodeEnabled) {
        await hostDisableCode();
      } else {
        await hostEnableCode();
      }
    } finally {
      togglingCode = false;
    }
  }

  async function regenerateCode() {
    togglingCode = true;
    try {
      await hostEnableCode();
    } finally {
      togglingCode = false;
    }
  }

  async function startSession() {
    await createAndConnect();
  }

  async function endSession() {
    await hostEndSession();
  }
</script>

{#if $lpPanelOpen && $lpRole === 'host'}
  <aside class="party-panel">
    <div class="panel-header">
      <span class="panel-title">Listen Along</span>
      <button class="close-btn" onclick={() => lpPanelOpen.set(false)} aria-label="Close panel">
        <svg width="14" height="14" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          <line x1="2" y1="2" x2="12" y2="12"/><line x1="12" y1="2" x2="2" y2="12"/>
        </svg>
      </button>
    </div>

    <div class="status-row">
      <span class="status-dot" class:connected={$lpConnected}></span>
      <span class="status-label">{$lpConnected ? 'Live' : 'Connecting…'}</span>
    </div>

    <div class="invite-section">
      <p class="section-label">Invite Link</p>
      {#if qrDataUrl}
        <div class="qr-wrap">
          <img src={qrDataUrl} alt="Party invite QR code" class="qr-img" />
        </div>
      {/if}
      <div class="invite-row">
        <input
          class="invite-input"
          type="text"
          readonly
          value={inviteUrl}
          onclick={(e) => (e.target as HTMLInputElement).select()}
        />
        <button class="copy-btn" onclick={copyLink}>
          {#if copied}
            <svg width="14" height="14" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="4 10 8 14 16 6"/>
            </svg>
          {:else}
            <svg width="14" height="14" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <rect x="7" y="7" width="10" height="10" rx="1"/><path d="M13 7V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2"/>
            </svg>
          {/if}
        </button>
      </div>
    </div>

    <div class="code-section">
      <div class="code-header-row">
        <p class="section-label" style="margin:0">Access Code</p>
        <button
          class="code-toggle-btn"
          class:active={$lpCodeEnabled}
          onclick={toggleCode}
          disabled={togglingCode}
          aria-label={$lpCodeEnabled ? 'Disable access code' : 'Enable access code'}
        >
          <span class="toggle-track" class:on={$lpCodeEnabled}>
            <span class="toggle-thumb"></span>
          </span>
        </button>
      </div>
      {#if $lpCodeEnabled && $lpAccessCode}
        <div class="code-display-row">
          <span class="code-digits">{$lpAccessCode}</span>
          <button class="icon-btn" onclick={copyCode} title="Copy code" aria-label="Copy access code">
            {#if codeCopied}
              <svg width="14" height="14" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
                <polyline points="4 10 8 14 16 6"/>
              </svg>
            {:else}
              <svg width="14" height="14" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <rect x="7" y="7" width="10" height="10" rx="1"/><path d="M13 7V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v6a2 2 0 0 0 2 2h2"/>
              </svg>
            {/if}
          </button>
          <button class="icon-btn" onclick={regenerateCode} disabled={togglingCode} title="Generate new code" aria-label="Generate new code">
            <svg width="14" height="14" viewBox="0 0 20 20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <polyline points="1 4 1 10 7 10"/>
              <path d="M3.51 15a9 9 0 1 0 .49-4.11"/>
            </svg>
          </button>
        </div>
        <p class="code-hint">Share this code with guests to let them join.</p>
      {:else if !$lpCodeEnabled}
        <p class="code-hint">Enable to require guests to enter a 4-digit code before joining.</p>
      {/if}
    </div>

    <div class="participants-section">
      <p class="section-label">
        Listeners
        {#if $lpParticipants.length > 0}
          <span class="count-badge">{$lpParticipants.length}</span>
        {/if}
      </p>

      {#if $lpParticipants.length === 0}
        <p class="empty-msg">No one has joined yet.</p>
      {:else}
        <ul class="participant-list">
          {#each $lpParticipants as p (p.id)}
            <li class="participant-row">
              <span class="participant-avatar">{p.nickname[0].toUpperCase()}</span>
              <span class="participant-name">{p.nickname}</span>
              <button
                class="kick-btn"
                onclick={() => hostKick(p.id)}
                title="Remove {p.nickname}"
                aria-label="Remove {p.nickname}"
              >
                <svg width="12" height="12" viewBox="0 0 14 14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
                  <line x1="2" y1="2" x2="12" y2="12"/><line x1="12" y1="2" x2="2" y2="12"/>
                </svg>
              </button>
            </li>
          {/each}
        </ul>
      {/if}
    </div>

    <div class="panel-footer">
      <button class="end-btn" onclick={endSession}>End Session</button>
    </div>
  </aside>
{/if}

<style>
  .party-panel {
    background: var(--bg-elevated);
    border-left: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    overflow-y: auto;
    box-shadow: -4px 0 16px rgba(0,0,0,0.15);
  }

  .panel-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 16px 16px 12px;
    border-bottom: 1px solid var(--border);
  }

  .panel-title {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--text);
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px;
    display: flex;
    align-items: center;
    border-radius: 4px;
    transition: color 0.15s, background 0.15s;
  }
  .close-btn:hover { color: var(--text); background: var(--bg-hover); }

  .status-row {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 10px 16px;
  }
  .status-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: var(--text-muted);
    transition: background 0.3s;
  }
  .status-dot.connected { background: #22c55e; box-shadow: 0 0 6px #22c55e88; }
  .status-label { font-size: 0.78rem; color: var(--text-muted); }

  .invite-section, .participants-section, .code-section {
    padding: 12px 16px;
    border-bottom: 1px solid var(--border);
  }

  .section-label {
    font-size: 0.75rem;
    font-weight: 600;
    color: var(--text-muted);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    margin: 0 0 8px;
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .count-badge {
    background: var(--accent-dim);
    color: var(--accent);
    border-radius: 10px;
    padding: 1px 6px;
    font-size: 0.7rem;
    font-weight: 700;
  }

  .qr-wrap {
    display: none;
    justify-content: center;
    padding: 8px 0 12px;
  }

  .qr-img {
    border-radius: 10px;
    border: 1px solid var(--border);
    width: 180px;
    height: 180px;
  }

  .invite-row {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .invite-input {
    flex: 1;
    min-width: 0;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-muted);
    font-size: 0.72rem;
    padding: 6px 8px;
    cursor: text;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .copy-btn {
    flex-shrink: 0;
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
    color: var(--accent);
    border-radius: 6px;
    padding: 6px 8px;
    cursor: pointer;
    display: flex;
    align-items: center;
    transition: background 0.15s;
  }
  .copy-btn:hover { background: var(--accent); color: #fff; }

  .participants-section {
    flex: 1;
    overflow-y: auto;
    border-bottom: none;
  }

  .empty-msg {
    font-size: 0.8rem;
    color: var(--text-muted);
    margin: 0;
    padding: 4px 0;
  }

  .participant-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .participant-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 8px;
    border-radius: 6px;
    transition: background 0.15s;
  }
  .participant-row:hover { background: var(--bg-hover); }
  .participant-row:hover .kick-btn { opacity: 1; }

  .participant-avatar {
    width: 28px;
    height: 28px;
    border-radius: 50%;
    background: var(--accent-dim);
    color: var(--accent);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 0.75rem;
    font-weight: 700;
    flex-shrink: 0;
  }

  .participant-name {
    flex: 1;
    min-width: 0;
    font-size: 0.85rem;
    color: var(--text);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .kick-btn {
    background: none;
    border: none;
    color: var(--text-muted);
    cursor: pointer;
    padding: 4px;
    opacity: 0;
    transition: opacity 0.15s, color 0.15s;
    display: flex;
    align-items: center;
    border-radius: 4px;
  }
  .kick-btn:hover { color: #ef4444; }

  .panel-footer {
    padding: 12px 16px;
    border-top: 1px solid var(--border);
  }

  .end-btn {
    width: 100%;
    padding: 8px;
    background: none;
    border: 1px solid #ef444480;
    border-radius: 6px;
    color: #ef4444;
    font-size: 0.85rem;
    cursor: pointer;
    transition: background 0.15s;
  }
  .end-btn:hover { background: #ef444415; }

  /* Access code section */
  .code-header-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 10px;
  }

  .code-toggle-btn {
    background: none;
    border: none;
    padding: 2px;
    cursor: pointer;
    display: flex;
    align-items: center;
  }
  .code-toggle-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .toggle-track {
    display: flex;
    align-items: center;
    width: 32px;
    height: 18px;
    border-radius: 9px;
    background: var(--bg-hover);
    border: 1px solid var(--border);
    padding: 2px;
    transition: background 0.2s, border-color 0.2s;
    cursor: pointer;
  }
  .toggle-track.on {
    background: var(--accent);
    border-color: var(--accent);
  }
  .toggle-thumb {
    width: 12px;
    height: 12px;
    border-radius: 50%;
    background: var(--text-muted);
    transition: transform 0.2s, background 0.2s;
  }
  .toggle-track.on .toggle-thumb {
    transform: translateX(14px);
    background: #fff;
  }

  .code-display-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }

  .code-digits {
    font-family: 'DM Mono', monospace, monospace;
    font-size: 1.6rem;
    font-weight: 700;
    letter-spacing: 0.25em;
    color: var(--accent);
    background: var(--accent-dim);
    border-radius: 8px;
    padding: 6px 14px;
    flex: 1;
    text-align: center;
  }

  .icon-btn {
    background: none;
    border: 1px solid var(--border);
    border-radius: 6px;
    color: var(--text-muted);
    cursor: pointer;
    padding: 6px;
    display: flex;
    align-items: center;
    transition: color 0.15s, background 0.15s;
  }
  .icon-btn:hover:not(:disabled) { color: var(--text); background: var(--bg-hover); }
  .icon-btn:disabled { opacity: 0.4; cursor: not-allowed; }

  .code-hint {
    font-size: 0.72rem;
    color: var(--text-muted);
    margin: 0;
    line-height: 1.4;
  }

  /* ── Mobile: full-screen bottom sheet ──────────────────────────────────── */
  @media (max-width: 640px) {
    .qr-wrap {
      display: flex;
    }

    .party-panel {
      position: fixed;
      top: 0;
      bottom: 0;
      left: 0;
      width: 100%;
      border-left: none;
      border-radius: 0;
      z-index: 110;
      box-shadow: 0 -4px 24px rgba(0,0,0,0.25);
    }

    .panel-header {
      padding: 16px 20px 14px;
      padding-top: calc(16px + env(safe-area-inset-top, 0px));
    }

    .panel-title {
      font-size: 1rem;
    }

    .close-btn {
      padding: 8px;
    }
    .close-btn svg {
      width: 18px;
      height: 18px;
    }

    .status-row {
      padding: 10px 20px;
    }

    .invite-section, .participants-section, .code-section {
      padding: 14px 20px;
    }

    .invite-input {
      font-size: 0.82rem;
      padding: 10px 12px;
    }

    .copy-btn {
      padding: 10px 12px;
    }

    .section-label {
      font-size: 0.8rem;
    }

    .participant-row {
      padding: 10px 8px;
    }

    .participant-avatar {
      width: 34px;
      height: 34px;
      font-size: 0.85rem;
    }

    .participant-name {
      font-size: 0.92rem;
    }

    .kick-btn {
      opacity: 1;
      padding: 8px;
    }
    .kick-btn svg {
      width: 16px;
      height: 16px;
    }

    .panel-footer {
      padding: 14px 20px;
      padding-bottom: calc(14px + env(safe-area-inset-bottom, 0px));
    }

    .end-btn {
      padding: 12px;
      font-size: 0.95rem;
    }

    .code-digits {
      font-size: 1.8rem;
      padding: 10px 16px;
    }

    .icon-btn {
      padding: 10px;
    }
    .icon-btn svg {
      width: 18px;
      height: 18px;
    }

    .code-hint {
      font-size: 0.8rem;
    }
  }
</style>
