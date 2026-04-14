<script lang="ts">
  import type { Track } from "$lib/types";
  import TrackRow from "./TrackRow.svelte";
  import {
    selectAll,
    invertSelection,
    clearSelection,
  } from "$lib/stores/ui/trackSelection";
  import { onDestroy } from "svelte";

  export let tracks: Track[];
  export let showCover: boolean = false;
  export let showDiscNumbers: boolean = true;

  // Group tracks by disc number; sort groups and tracks within each group.
  $: discs = (() => {
    const groups = new Map<number, Track[]>();
    for (const track of tracks) {
      const disc = track.disc_number ?? 1;
      if (!groups.has(disc)) groups.set(disc, []);
      groups.get(disc)!.push(track);
    }
    return [...groups.entries()].sort(([a], [b]) => a - b);
  })();

  $: isMultiDisc = discs.length > 1;

  // Map track.id → flat index in the tracks array (for shift-select ranges)
  $: flatIndexMap = new Map(tracks.map((t, i) => [t.id, i]));

  // Clear selection whenever the track list changes (e.g. navigating to a different album)
  let _prevTracks: Track[] | null = null;
  $: if (tracks !== _prevTracks) {
    _prevTracks = tracks;
    clearSelection();
  }

  onDestroy(() => clearSelection());

  function onKeydown(e: KeyboardEvent) {
    if (!e.ctrlKey && !e.metaKey) return;
    const target = e.target as HTMLElement;
    if (
      target.tagName === "INPUT" ||
      target.tagName === "TEXTAREA" ||
      target.isContentEditable
    )
      return;
    if (e.key === "a" || e.key === "A") {
      e.preventDefault();
      selectAll(tracks);
    } else if (e.key === "i" || e.key === "I") {
      e.preventDefault();
      invertSelection(tracks);
    } else if (e.key === "Escape") {
      clearSelection();
    }
  }
</script>

<svelte:window on:keydown={onKeydown} />

{#if showDiscNumbers && isMultiDisc}
  {#each discs as [discNum, discTracks]}
    <div class="disc-group">
      <div class="disc-header">
        <svg
          width="13"
          height="13"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <circle cx="12" cy="12" r="10" /><circle cx="12" cy="12" r="3" />
        </svg>
        Disc {discNum}
      </div>
      <div class="track-list">
        {#each discTracks as track, i (track.id)}
          <TrackRow
            {track}
            trackList={tracks}
            index={i}
            flatIndex={flatIndexMap.get(track.id) ?? i}
            {showCover}
          />
        {/each}
      </div>
    </div>
  {/each}
{:else}
  <div class="track-list">
    {#each tracks as track, i (track.id)}
      <TrackRow
        {track}
        trackList={tracks}
        index={i}
        flatIndex={i}
        {showCover}
        useRankIndex={!showDiscNumbers}
      />
    {/each}
  </div>
{/if}

<style>
  .track-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .disc-group {
    margin-bottom: 24px;
  }
  .disc-group:last-child {
    margin-bottom: 0;
  }

  .disc-header {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 0.75rem;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--text-muted);
    padding: 0 12px 8px;
    border-bottom: 1px solid var(--border, rgba(255, 255, 255, 0.08));
    margin-bottom: 4px;
  }
</style>
