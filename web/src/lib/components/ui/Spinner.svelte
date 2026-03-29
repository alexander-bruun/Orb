<script lang="ts">
  export let size: number = 20;
</script>

<!--
  Orb loading spinner — mirrors the app logo:
  outer ring + center dot + small dot orbiting the ring.
  Rotating the whole SVG is imperceptible for the symmetric ring/center,
  so only the orbiting dot appears to move.
-->
<svg
  viewBox="0 0 24 24"
  width={size}
  height={size}
  class="orb-spinner"
  aria-label="Loading"
  role="status"
>
  <!-- Outer ring -->
  <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="1.4" fill="none" opacity="0.25" />
  <!-- Center dot -->
  <circle cx="12" cy="12" r="3.5" fill="currentColor" opacity="0.4" />
  <!-- Orbiting dot — sits on the outer ring (r=10, top = y:2) -->
  <circle cx="12" cy="2" r="2.2" fill="currentColor" />
</svg>

<style>
  .orb-spinner {
    display: inline-block;
    flex-shrink: 0;
    animation: orb-spin 1.2s linear infinite;
  }

  @keyframes orb-spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
