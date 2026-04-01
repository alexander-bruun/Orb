<script lang="ts">
  import { onMount } from "svelte";
  import { podcasts as podcastApi } from "$lib/api/podcasts";
  import type { Podcast } from "$lib/types";
  import { getApiBase } from "$lib/api/base";
  import Skeleton from "$lib/components/ui/Skeleton.svelte";
  import { goto } from "$app/navigation";

  let subscriptions: Podcast[] = [];
  let loading = true;
  let rssUrl = "";
  let subscribing = false;

  async function loadSubscriptions() {
    loading = true;
    try {
      const res = await podcastApi.getSubscriptions();
      subscriptions = res.podcasts ?? [];
    } catch (err) {
      console.error("Failed to load subscriptions", err);
    } finally {
      loading = false;
    }
  }

  async function handleSubscribe() {
    if (!rssUrl) return;
    subscribing = true;
    try {
      await podcastApi.subscribe(rssUrl);
      rssUrl = "";
      await loadSubscriptions();
    } catch (err) {
      alert("Failed to subscribe: " + err);
    } finally {
      subscribing = false;
    }
  }

  onMount(loadSubscriptions);
</script>

<svelte:head><title>Podcasts – Orb</title></svelte:head>

<div class="page">
  <div class="page-header">
    <h1 class="page-title">Podcasts</h1>
    <div class="subscribe-box">
      <input 
        type="text" 
        placeholder="RSS Feed URL" 
        bind:value={rssUrl} 
        on:keydown={(e) => e.key === 'Enter' && handleSubscribe()}
        disabled={subscribing}
      />
      <button on:click={handleSubscribe} disabled={subscribing || !rssUrl}>
        {subscribing ? "Subscribing..." : "Subscribe"}
      </button>
    </div>
  </div>

  {#if loading}
    <div class="grid">
      {#each { length: 6 } as _}
        <div class="card-skeleton">
          <div class="skeleton-cover"></div>
          <Skeleton width="70%" height="0.85rem" radius="4px" />
          <Skeleton width="50%" height="0.75rem" radius="4px" />
        </div>
      {/each}
    </div>
  {:else if subscriptions.length === 0}
    <div class="empty">
      <p>No podcast subscriptions yet.</p>
    </div>
  {:else}
    <div class="grid">
      {#each subscriptions as podcast (podcast.id)}
        <div class="podcast-card" on:click={() => goto(`/podcasts/${podcast.id}`)}>
          <div class="cover-wrap">
            {#if podcast.cover_art_key}
              <img src="{getApiBase()}/covers/podcast/{podcast.id}" alt={podcast.title} class="cover" />
            {:else}
              <div class="cover placeholder">
                <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
                  <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
                  <line x1="12" y1="19" x2="12" y2="23"/>
                  <line x1="8" y1="23" x2="16" y2="23"/>
                </svg>
              </div>
            {/if}
          </div>
          <div class="info">
            <span class="title">{podcast.title}</span>
            {#if podcast.author}
              <span class="author">{podcast.author}</span>
            {/if}
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .page { padding: 24px; }
  .page-header { display: flex; align-items: center; justify-content: space-between; margin-bottom: 24px; }
  .page-title { font-size: 1.5rem; font-weight: 700; margin: 0; }
  
  .subscribe-box { display: flex; gap: 8px; }
  .subscribe-box input { 
    background: var(--bg-elevated); 
    border: 1px solid var(--border); 
    color: var(--text); 
    padding: 8px 12px; 
    border-radius: 4px; 
    width: 300px;
  }
  .subscribe-box button {
    background: var(--accent);
    color: white;
    border: none;
    padding: 8px 16px;
    border-radius: 4px;
    cursor: pointer;
  }
  .subscribe-box button:disabled { opacity: 0.5; cursor: not-allowed; }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
    gap: 24px;
  }

  .podcast-card { cursor: pointer; display: flex; flex-direction: column; gap: 8px; }
  .cover-wrap { width: 100%; aspect-ratio: 1; border-radius: 8px; overflow: hidden; background: var(--bg-elevated); }
  .cover { width: 100%; height: 100%; object-fit: cover; }
  .placeholder { display: flex; align-items: center; justify-content: center; color: var(--text-muted); }
  
  .info { display: flex; flex-direction: column; }
  .title { font-weight: 600; font-size: 0.9rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .author { font-size: 0.8rem; color: var(--text-muted); }

  .card-skeleton { display: flex; flex-direction: column; gap: 8px; }
  .skeleton-cover { width: 100%; aspect-ratio: 1; border-radius: 8px; background: var(--bg-elevated); }

  .empty { text-align: center; padding: 48px; color: var(--text-muted); }
</style>
