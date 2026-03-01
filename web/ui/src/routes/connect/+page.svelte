<script lang="ts">
  import { goto } from '$app/navigation';
  import { setServerUrl } from '$lib/api/base';
  import { isTauri } from '$lib/utils/platform';
  import { setupRequired } from '$lib/stores/setup';
  import { apiFetch } from '$lib/api/client';

  let url = '';
  let phase: 'idle' | 'connecting' | 'success' | 'error' = 'idle';
  let errorMsg = '';

  // Discovery state
  let discovering = false;
  let discovered: Array<{name: string; host: string; port: number; url: string; version: string}> = [];
  let discoveryError = '';

  const showDiscover = isTauri();

  async function discoverServers() {
    discovering = true;
    discoveryError = '';
    discovered = [];

    try {
      const w = window as any;
      const servers: typeof discovered = await w.__TAURI_INTERNALS__.invoke('discover_servers');
      discovered = servers;
      if (servers.length === 0) {
        discoveryError = 'No Orb servers found on your network.';
      }
    } catch (err: any) {
      discoveryError = err.message ?? 'Discovery failed.';
    } finally {
      discovering = false;
    }
  }

  function selectServer(serverUrl: string) {
    url = serverUrl;
    discovered = [];
  }

  async function connect(e: Event) {
    e.preventDefault();
    errorMsg = '';

    const cleaned = url.replace(/\/+$/, '');
    if (!cleaned) {
      errorMsg = 'Please enter a server URL.';
      return;
    }

    phase = 'connecting';

    try {
      const res = await fetch(`${cleaned}/healthz`, {
        signal: AbortSignal.timeout(8000),
      });
      if (!res.ok) throw new Error(`Server returned ${res.status}`);
      const text = await res.text();
      if (!text.includes('ok')) throw new Error('Not an Orb server');
    } catch (err: any) {
      phase = 'error';
      if (err.name === 'TimeoutError') {
        errorMsg = 'Connection timed out. Check the URL and try again.';
      } else if (err.name === 'TypeError') {
        errorMsg = 'Could not reach server. Check the URL and your network.';
      } else {
        errorMsg = err.message ?? 'Connection failed.';
      }
      return;
    }

    phase = 'success';
    setServerUrl(cleaned);

    // Fetch setup status now that we have a valid server URL,
    // so the layout's $effect can route correctly after navigation.
    try {
      const data = await apiFetch<{ setup_required: boolean }>('/auth/setup');
      setupRequired.set(data.setup_required);
    } catch {
      setupRequired.set(false);
    }

    await new Promise((r) => setTimeout(r, 600));
    goto('/login');
  }
</script>

<div class="connect-page">
  <div class="connect-card">
    <h1 class="logo">orb</h1>
    <p class="subtitle">Connect to your Orb server</p>

    {#if phase === 'connecting'}
      <div class="loader-wrap">
        <div class="orb-loader">
          <div class="ring ring-1"></div>
          <div class="ring ring-2"></div>
          <div class="ring ring-3"></div>
          <div class="dot"></div>
        </div>
        <p class="loader-text">Connecting...</p>
      </div>
    {:else if phase === 'success'}
      <div class="loader-wrap">
        <div class="success-check">
          <svg width="40" height="40" viewBox="0 0 24 24" fill="none" stroke="var(--accent)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <polyline points="20 6 9 17 4 12"/>
          </svg>
        </div>
        <p class="loader-text">Connected</p>
      </div>
    {:else}
      {#if errorMsg}
        <p class="error">{errorMsg}</p>
      {/if}

      <form onsubmit={connect}>
        <label>
          Server URL
          <input
            type="url"
            bind:value={url}
            placeholder="https://orb.example.com/api"
            required
            autocomplete="url"
            autofocus
          />
        </label>
        <button type="submit" class="btn-primary">Connect</button>
      </form>

      {#if showDiscover}
        <div class="discovery">
          <button
            type="button"
            class="btn-discover"
            onclick={discoverServers}
            disabled={discovering}
          >
            {discovering ? 'Scanning...' : 'Discover servers'}
          </button>

          {#if discoveryError}
            <p class="discovery-msg">{discoveryError}</p>
          {/if}

          {#if discovered.length > 0}
            <ul class="server-list">
              {#each discovered as server}
                <li>
                  <button
                    type="button"
                    class="server-item"
                    onclick={() => selectServer(server.url)}
                  >
                    <span class="server-name">{server.name}</span>
                    <span class="server-url">{server.url}</span>
                  </button>
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      {/if}

      <p class="hint">
        Enter the base URL of your Orb server, including <code>/api</code> if behind a reverse proxy.
      </p>
    {/if}
  </div>
</div>

<style>
  .connect-page {
    min-height: 100dvh;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg);
  }

  .connect-card {
    width: 100%;
    max-width: 380px;
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
    font-family: 'DM Mono', monospace;
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

  .error {
    color: #f87171;
    font-size: 0.875rem;
    margin: 0 0 12px;
  }

  .hint {
    margin: 16px 0 0;
    font-size: 0.75rem;
    color: var(--text-muted);
    line-height: 1.4;
  }
  .hint code {
    font-family: 'DM Mono', monospace;
    font-size: 0.7rem;
    background: var(--bg);
    padding: 1px 5px;
    border-radius: 4px;
    color: var(--accent);
  }

  /* ── Discovery ── */
  .discovery {
    margin-top: 16px;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .btn-discover {
    background: transparent;
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px;
    color: var(--text-muted);
    font-size: 0.875rem;
    cursor: pointer;
    transition: border-color 0.15s, color 0.15s;
  }
  .btn-discover:hover { border-color: var(--accent); color: var(--text); }
  .btn-discover:disabled { opacity: 0.6; cursor: not-allowed; }

  .discovery-msg {
    color: var(--text-muted);
    font-size: 0.8rem;
    margin: 0;
  }

  .server-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .server-item {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    text-align: left;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    padding: 8px 12px;
    cursor: pointer;
    transition: border-color 0.15s;
  }
  .server-item:hover { border-color: var(--accent); }

  .server-name {
    font-size: 0.875rem;
    font-weight: 600;
    color: var(--text);
  }

  .server-url {
    font-size: 0.75rem;
    font-family: 'DM Mono', monospace;
    color: var(--text-muted);
  }

  /* ── Loader ── */
  .loader-wrap {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 20px;
    padding: 24px 0;
  }

  .loader-text {
    font-size: 0.875rem;
    color: var(--text-muted);
    margin: 0;
    letter-spacing: 0.04em;
  }

  .orb-loader {
    position: relative;
    width: 80px;
    height: 80px;
  }

  .ring {
    position: absolute;
    inset: 0;
    border-radius: 50%;
    border: 2px solid transparent;
  }

  .ring-1 {
    border-top-color: var(--accent);
    animation: spin 1.2s linear infinite;
  }

  .ring-2 {
    inset: 8px;
    border-right-color: var(--accent);
    opacity: 0.6;
    animation: spin 1.8s linear infinite reverse;
  }

  .ring-3 {
    inset: 16px;
    border-bottom-color: var(--accent);
    opacity: 0.3;
    animation: spin 2.4s linear infinite;
  }

  .dot {
    position: absolute;
    width: 8px;
    height: 8px;
    background: var(--accent);
    border-radius: 50%;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    box-shadow: 0 0 12px var(--accent-glow, rgba(168, 130, 255, 0.5));
    animation: pulse 1.2s ease-in-out infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  @keyframes pulse {
    0%, 100% { opacity: 0.4; transform: translate(-50%, -50%) scale(0.8); }
    50% { opacity: 1; transform: translate(-50%, -50%) scale(1.2); }
  }

  /* ── Success ── */
  .success-check {
    width: 80px;
    height: 80px;
    display: flex;
    align-items: center;
    justify-content: center;
    border-radius: 50%;
    background: var(--accent-dim);
    animation: pop 0.3s ease-out;
  }

  @keyframes pop {
    0% { transform: scale(0.5); opacity: 0; }
    100% { transform: scale(1); opacity: 1; }
  }
</style>
