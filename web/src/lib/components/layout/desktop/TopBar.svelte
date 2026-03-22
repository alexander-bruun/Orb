<script lang="ts">
  import { authStore } from '$lib/stores/auth';
  import { formattedFormat } from '$lib/stores/player';
  import { abFormattedFormat } from '$lib/stores/player/audiobookPlayer';
  import { activePlayer } from '$lib/stores/player/engine';
  import { sidebarOpen } from '$lib/stores/ui/sidebar';
  import { isDesktop } from '$lib/utils/platform';
  import IngestIndicator from './IngestIndicator.svelte';
  import QuickSearch from './QuickSearch.svelte';
  import UserMenu from './UserMenu.svelte';

  let quickSearchRef: QuickSearch;
  let userMenuRef: UserMenu;

  function closeAll() {
    userMenuRef?.close();
    quickSearchRef?.blur();
  }
</script>

<svelte:window on:click={closeAll} />

<header class="topbar">
  <!-- Hamburger: only visible on mobile -->
  <button class="hamburger" on:click|stopPropagation={() => sidebarOpen.update(v => !v)} aria-label="Toggle navigation">
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
      <line x1="3" y1="6" x2="21" y2="6"/>
      <line x1="3" y1="12" x2="21" y2="12"/>
      <line x1="3" y1="18" x2="21" y2="18"/>
    </svg>
  </button>

  {#if !isDesktop()}
  <a href="/" class="wordmark" aria-label="Orb">
    <svg viewBox="0 0 52 28" height="30" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <circle cx="12" cy="14" r="10" stroke="currentColor" stroke-width="1.4" opacity="0.3"/>
      <circle cx="12" cy="14" r="4.5" fill="currentColor"/>
      <text x="27" y="21" font-family="'Instrument Serif', Georgia, serif" font-style="italic" font-size="22" fill="currentColor" letter-spacing="-0.02em">orb</text>
    </svg>
  </a>
  {/if}

  <QuickSearch bind:this={quickSearchRef} />

  <div class="spacer"></div>

  {#if $activePlayer === 'audiobook' ? $abFormattedFormat : $formattedFormat}
    <div class="format-badge">{$activePlayer === 'audiobook' ? $abFormattedFormat : $formattedFormat}</div>
  {/if}

  {#if $authStore.user?.is_admin}
    <IngestIndicator />
  {/if}

  <UserMenu bind:this={userMenuRef} />

</header>

<style>
  .topbar {
    display: flex;
    align-items: center;
    padding: 0 20px;
    gap: 16px;
    border-bottom: 1px solid var(--border);
    background: rgba(8,8,9,0.95);
    backdrop-filter: blur(12px);
    z-index: 20;
  }

  :global([data-theme="light"]) .topbar {
    background: rgba(240,240,245,0.95);
  }

  .wordmark {
    color: var(--accent);
    flex-shrink: 0;
    margin-right: 4px;
    display: flex;
    align-items: center;
  }

  .spacer { flex: 1; }

  .format-badge {
    font-family: 'DM Mono', monospace;
    font-size: 10px;
    letter-spacing: 0.08em;
    color: var(--accent);
    background: var(--accent-dim);
    border: 1px solid var(--accent-glow);
    border-radius: 4px;
    padding: 3px 8px;
  }

  /* ── Hamburger (mobile only) ────────────────────────────── */
  .hamburger {
    display: none;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    color: var(--text-2);
    cursor: pointer;
    padding: 4px;
    border-radius: 6px;
    flex-shrink: 0;
    transition: color 0.15s, background 0.15s;
  }
  .hamburger:hover { color: var(--text); background: var(--surface-2); }

  @media (max-width: 640px) {
    .hamburger { display: flex; }
    .format-badge { display: none; }
    .topbar { padding: 0 12px; gap: 10px; }
  }
</style>
