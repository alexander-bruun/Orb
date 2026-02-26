<script lang="ts">
  import {
    lpRole,
    lpSessionId,
    lpParticipants,
    lpPanelOpen,
    lpConnected,
    hostKick,
    hostEndSession,
    createAndConnect,
  } from '$lib/stores/listenParty';

  const BASE = import.meta.env.VITE_API_BASE ?? '/api';
  const APP_BASE = typeof location !== 'undefined' ? location.origin : '';

  let copied = $state(false);

  let inviteUrl = $derived(
    $lpSessionId
      ? `${APP_BASE}/listen/${$lpSessionId}`
      : ''
  );

  async function copyLink() {
    if (!inviteUrl) return;
    await navigator.clipboard.writeText(inviteUrl).catch(() => {});
    copied = true;
    setTimeout(() => (copied = false), 2000);
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
    position: fixed;
    right: 0;
    top: var(--top-h, 48px);
    bottom: var(--bottom-h, 80px);
    width: 280px;
    background: var(--bg-elevated);
    border-left: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    z-index: 100;
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

  .invite-section, .participants-section {
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
</style>
