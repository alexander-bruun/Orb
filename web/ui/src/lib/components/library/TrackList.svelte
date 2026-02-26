<script lang="ts">
  import type { Track } from '$lib/types';
  import TrackRow from './TrackRow.svelte';

  export let tracks: Track[];
  export let showCover: boolean = false;
</script>

<div class="track-list">
  {#each tracks as track, i (track.id)}
    <TrackRow {track} trackList={tracks} index={i} {showCover} />
  {/each}
</div>

<style>
  .track-list { display: flex; flex-direction: column; gap: 2px; }
</style>
