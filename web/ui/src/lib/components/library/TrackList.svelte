<script lang="ts">
  import type { Track } from '$lib/types';
  import TrackRow from './TrackRow.svelte';

  export let tracks: Track[];
</script>

<div class="track-list">
  {#each tracks as track, i (track.id)}
    <TrackRow {track} trackList={tracks} index={i} />
  {/each}
</div>

<style>
  .track-list { display: flex; flex-direction: column; gap: 2px; }
</style>
