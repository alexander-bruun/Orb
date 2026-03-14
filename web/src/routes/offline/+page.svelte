<script lang="ts">
  import { goto } from '$app/navigation';
  import { onMount } from 'svelte';

  // The /offline route is deprecated — Favorites is now the offline-first
  // landing pivot.  Redirect immediately so existing bookmarks still work.
  onMount(() => goto('/favorites', { replaceState: true }));
</script>
