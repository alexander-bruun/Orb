<script lang="ts">
  import { onMount } from 'svelte';
  import { library as libApi } from '$lib/api/library';
  import type { Track } from '$lib/types';
  import TrackList from '$lib/components/library/TrackList.svelte';

  let recentTracks: Track[] = [];
  let loading = true;

  onMount(async () => {
    try {
      recentTracks = (await libApi.recentlyPlayed()) ?? [];
    } catch {
      // ignore — user may not be logged in
    } finally {
      loading = false;
    }
  });
</script>

<section>
  <h2 class="section-title">Recently Played</h2>
  {#if loading}
    <p class="muted">Loading…</p>
  {:else if recentTracks.length === 0}
    <p class="muted">Nothing played yet. Go find some music!</p>
  {:else}
    <TrackList tracks={recentTracks} />
  {/if}
</section>

<style>
  .section-title { font-size: 1.25rem; font-weight: 600; margin-bottom: 16px; }
  .muted { color: var(--text-muted); font-size: 0.875rem; }
</style>
