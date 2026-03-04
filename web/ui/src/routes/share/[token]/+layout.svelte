<script lang="ts">
  let { children } = $props();
</script>

<!-- Isolated layout for share page: no auth, no shell chrome -->
{@render children()}
