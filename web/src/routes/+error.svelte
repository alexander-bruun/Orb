<script lang="ts">
  import { page } from "$app/stores";
  import { goto } from "$app/navigation";
  import { onMount } from "svelte";
  import { get } from "svelte/store";
  import {
    isOffline,
    checkConnectivity,
  } from "$lib/stores/offline/connectivity";
  import { downloads, restoreDownloads } from "$lib/stores/offline/downloads";

  let checking = true;
  let offline = false;
  let hasDownloads = false;

  onMount(async () => {
    // Ensure download metadata is available
    restoreDownloads();

    // If the connectivity monitor already knows we're offline, skip the
    // network round-trip (avoids a 3-5 s hang waiting for the fetch to
    // time out when airplane mode just kicked in).
    const alreadyOffline = get(isOffline);
    let isDown: boolean;
    if (alreadyOffline) {
      isDown = true;
    } else {
      // Check actual connectivity — page errors might be from network failure
      isDown = await checkConnectivity();
    }
    offline = isDown;

    if (isDown) {
      const dlMap = get(downloads);
      hasDownloads = [...dlMap.values()].some((e) => e.status === "done");
      if (hasDownloads) {
        // Redirect to offline mode; await so we don't leave the component in
        // a "checking" state if navigation is delayed.
        try {
          await goto("/offline");
        } catch {
          // Navigation failed — fall through so the user sees a useful UI
          // rather than an endless "Checking connection…" spinner.
          checking = false;
        }
        return;
      }
    }

    checking = false;
  });
</script>

{#if checking}
  <div class="error-page">
    <p class="checking">Checking connection…</p>
  </div>
{:else if offline && !hasDownloads}
  <div class="error-page">
    <div class="icon">
      <svg
        width="48"
        height="48"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <line x1="1" y1="1" x2="23" y2="23" />
        <path d="M16.72 11.06A10.94 10.94 0 0 1 19 12.55" />
        <path d="M5 12.55a10.94 10.94 0 0 1 5.17-2.39" />
        <path d="M10.71 5.05A16 16 0 0 1 22.56 9" />
        <path d="M1.42 9a15.91 15.91 0 0 1 4.7-2.88" />
        <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
        <line x1="12" y1="20" x2="12.01" y2="20" />
      </svg>
    </div>
    <h1>You're Offline</h1>
    <p class="sub">
      The server can't be reached and you have no downloaded tracks.
    </p>
    <p class="hint">
      Download music while connected to enable offline playback.
    </p>
    <button
      class="btn-retry"
      on:click={() => {
        checking = true;
        checkConnectivity().then((down) => {
          if (!down) goto("/");
          else checking = false;
        });
      }}
    >
      Retry Connection
    </button>
  </div>
{:else}
  <div class="error-page">
    <h1>Something went wrong</h1>
    <p class="sub">
      {$page.status} — {$page.error?.message ?? "Unknown error"}
    </p>
    <div class="actions">
      <button class="btn-retry" on:click={() => goto("/")}>Go Home</button>
      <button class="btn-retry" on:click={() => location.reload()}
        >Reload</button
      >
    </div>
  </div>
{/if}

<svelte:head><title>Error – Orb</title></svelte:head>

<style>
  .error-page {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    min-height: 60vh;
    text-align: center;
    padding: 40px 20px;
    gap: 8px;
  }

  .checking {
    color: var(--text-muted, #888);
    font-size: 0.95rem;
  }

  .icon {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 80px;
    height: 80px;
    border-radius: 50%;
    background: color-mix(in srgb, var(--accent, #6366f1) 12%, transparent);
    color: var(--accent, #6366f1);
    margin-bottom: 12px;
  }

  h1 {
    font-size: 1.4rem;
    font-weight: 700;
    margin: 0;
    color: var(--text, #fff);
  }

  .sub {
    color: var(--text-muted, #888);
    font-size: 0.9rem;
    margin: 0;
  }

  .hint {
    color: var(--text-muted, #888);
    font-size: 0.85rem;
    opacity: 0.7;
    margin: 0;
  }

  .actions {
    display: flex;
    gap: 8px;
    margin-top: 12px;
  }

  .btn-retry {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 8px 18px;
    border-radius: 20px;
    border: 1px solid var(--border, #333);
    background: transparent;
    color: var(--text-muted, #888);
    font-size: 0.85rem;
    font-weight: 600;
    cursor: pointer;
    transition:
      color 0.15s,
      border-color 0.15s;
    margin-top: 8px;
  }

  .btn-retry:hover {
    color: var(--text, #fff);
    border-color: var(--text, #fff);
  }
</style>
