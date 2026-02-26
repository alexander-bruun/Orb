<script lang="ts">
  let { children } = $props();
</script>

<!-- Isolated layout for guest listen-along page: no auth, no shell chrome -->
{@render children()}
