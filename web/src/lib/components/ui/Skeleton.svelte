<script lang="ts">
  let {
    width = "100%",
    height = "1rem",
    radius = "4px",
    class: klass = "",
  }: {
    width?: string;
    height?: string;
    radius?: string;
    class?: string;
  } = $props();
</script>

<div
  class="skeleton {klass}"
  style="width: {width}; height: {height}; border-radius: {radius}"
  aria-hidden="true"
></div>

<style>
  .skeleton {
    background: linear-gradient(
      90deg,
      var(--bg-3, #2a2a2a) 25%,
      var(--bg-2, #333) 50%,
      var(--bg-3, #2a2a2a) 75%
    );
    background-size: 200% 100%;
    animation: shimmer 1.4s ease-in-out infinite;
    flex-shrink: 0;
  }

  @keyframes shimmer {
    0% {
      background-position: 200% 0;
    }
    100% {
      background-position: -200% 0;
    }
  }
</style>
