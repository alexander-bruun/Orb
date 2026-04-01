<script lang="ts">
  import type { Album } from '$lib/types';
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';
  import { getArtistName } from '$lib/stores/library/artists';
  import { currentTrack, playbackState } from '$lib/stores/player';

  export let album: Album;

  let artistName: string = '';

  $: isActiveAlbum = $currentTrack?.album_id === album.id;
  $: isPlaying = isActiveAlbum && $playbackState === 'playing';

  import { getApiBase } from '$lib/api/base';

  // Each straight edge uses a sine wave multiplied by a smoothstep envelope
  // that drives both amplitude and its derivative to zero at each endpoint.
  // This ensures C1 continuity where the wave meets the corner arc — no kink.
  const wavePath = (() => {
    const size = 200, amp = 3, r = 8, waves = 9, steps = 120;
    const e = size - r;

    function edge(x0: number, y0: number, x1: number, y1: number, nx: number, ny: number): string {
      const pts: string[] = [];
      for (let i = 0; i <= steps; i++) {
        const u = i / steps;
        // Smoothstep envelope fades the wave to zero over one wave period at
        // each end, so the path arrives at the corner arc tangentially.
        const fade = 1 / waves;
        const t = Math.min(u / fade, (1 - u) / fade, 1);
        const env = t * t * (3 - 2 * t);
        const w = amp * env * Math.sin(u * waves * Math.PI * 2);
        pts.push(`${(x0 + u*(x1-x0) + nx*w).toFixed(2)},${(y0 + u*(y1-y0) + ny*w).toFixed(2)}`);
      }
      return pts.join(' L ');
    }

    return [
      `M ${r},0 L`,      edge(r, 0,    e, 0,    0, -1),  // top →
      `A ${r},${r} 0 0 1 ${size},${r} L`,
                          edge(size, r, size, e, 1, 0),   // right ↓
      `A ${r},${r} 0 0 1 ${e},${size} L`,
                          edge(e, size, r, size, 0, 1),   // bottom ←
      `A ${r},${r} 0 0 1 0,${e} L`,
                          edge(0, e,    0, r,   -1, 0),   // left ↑
      `A ${r},${r} 0 0 1 ${r},0 Z`,
    ].join(' ');
  })();

  onMount(async () => {
    if (album.artist_name) {
      artistName = album.artist_name;
      return;
    }
    if (album.artist_id) {
      artistName = await getArtistName(album.artist_id);
    }
  });
</script>

<button class="album-card" class:playing={isActiveAlbum} on:click={() => goto(`/library/albums/${album.id}`)} aria-label="View album {album.title}">
  <div class="cover-wrap" class:wave-active={isActiveAlbum}>
    {#if album.cover_art_key}
      <img src="{getApiBase()}/covers/{album.id}" alt={album.title} class="cover" loading="lazy" />
    {:else}
      <div class="cover placeholder album-fallback">♪</div>
    {/if}
    {#if album.track_count === 1}
      <span class="badge-single">Single</span>
    {/if}
    {#if (album.max_channels ?? 2) >= 6}
      <span class="badge-channels">{album.max_channels === 8 ? '7.1' : '5.1'}</span>
    {/if}
    <svg class="wave-ring" class:wave-visible={isPlaying} viewBox="-5 -5 210 210" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
      <path d={wavePath} pathLength="1000" fill="none"
            stroke="var(--accent)" stroke-width="2" stroke-linecap="round"
            stroke-dasharray="350 650" class="wave-dash" />
      <path d={wavePath} pathLength="1000" fill="none"
            stroke="var(--accent)" stroke-width="6" stroke-linecap="round"
            stroke-dasharray="350 650" class="wave-dash wave-glow" />
    </svg>
  </div>
  <div class="info">
    <span class="title">{album.title}</span>
    <div class="meta">
      {#if artistName}<span class="artist">{artistName}</span>{/if}
      {#if album.release_year}<span class="year">{album.release_year}</span>{/if}
    </div>
  </div>
</button>

<style>
  .album-card {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
    max-width: 240px;
    box-sizing: border-box;
    background: var(--bg-elevated);
    border: 1px solid var(--border);
    border-radius: 8px;
    padding: 12px;
    cursor: pointer;
    text-align: left;
    transition: background 0.15s, border-color 0.15s;
    position: relative;
  }
  .album-card:hover { background: var(--bg-hover); border-color: var(--accent); }
  .album-card.playing { border-color: var(--accent); background: var(--accent-dim); }
  .album-card.playing:hover { background: var(--bg-hover); }

  .cover-wrap {
    position: relative;
    width: 100%;
    height: 0;
    padding-bottom: 100%;
    overflow: hidden;
    border-radius: 4px;
  }
  /* Allow wave SVG to bleed outside cover bounds while active or fading */
  .cover-wrap.wave-active {
    overflow: visible;
  }

  /* Wavy border ring — positioned to match cover, with extra space for amplitude */
  .wave-ring {
    position: absolute;
    inset: -5px;
    width: calc(100% + 10px);
    height: calc(100% + 10px);
    pointer-events: none;
    z-index: 1;
    opacity: 0;
    transition: opacity 0.6s ease;
  }
  .wave-ring.wave-visible {
    opacity: 1;
  }
  .wave-dash {
    animation: dash-travel 5s linear infinite;
  }
  .wave-glow {
    opacity: 0.15;
  }
  @keyframes dash-travel {
    to { stroke-dashoffset: -1000; }
  }
  .cover {
    position: absolute;
    inset: 0;
    width: 100%;
    height: 100%;
    object-fit: cover;
  }
  .placeholder {
    position: absolute;
    inset: 0;
    background: var(--bg-hover);
  }
  .album-fallback {
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 2.5rem;
    color: var(--text-muted);
    user-select: none;
  }
  .badge-single {
    position: absolute;
    top: 6px;
    right: 6px;
    background: var(--accent);
    color: var(--bg);
    font-size: 0.625rem;
    font-weight: 700;
    letter-spacing: 0.06em;
    text-transform: uppercase;
    padding: 2px 6px;
    border-radius: 3px;
    pointer-events: none;
  }
  .badge-channels {
    position: absolute;
    top: 6px;
    left: 6px;
    background: rgba(0, 0, 0, 0.55);
    color: #fff;
    font-family: 'DM Mono', monospace;
    font-size: 0.6rem;
    font-weight: 600;
    letter-spacing: 0.05em;
    padding: 2px 5px;
    border-radius: 3px;
    pointer-events: none;
    backdrop-filter: blur(4px);
  }

  .info { display: flex; flex-direction: column; gap: 2px; }
  .title { font-size: 0.875rem; font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; color: var(--text); }
  .meta { display: flex; align-items: baseline; justify-content: space-between; gap: 4px; min-width: 0; }
  .artist { font-size: 0.75rem; color: var(--text-muted); flex: 1; min-width: 0; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
  .year { font-size: 0.75rem; color: var(--text-muted); flex-shrink: 0; }
</style>
