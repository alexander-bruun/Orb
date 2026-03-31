<script lang="ts">
  export let size: number = 48;
  export let centered: boolean = false;
</script>

<!--
  Orb loading spinner — mirrors the app logo:
  outer ring + center dot + small dot orbiting the ring.
  Rotating the whole SVG is imperceptible for the symmetric ring/center,
  so only the orbiting dot appears to move.
-->
{#if centered}
  <div class="orb-spinner-centered">
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
  </div>
{:else}
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
{/if}

<style>
  .orb-spinner {
    display: inline-block;
    flex-shrink: 0;
    animation: orb-spin 1.2s linear infinite;
  }

  .orb-spinner-centered {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 100%;
    height: 100%;
    min-height: 200px;
  }

  @keyframes orb-spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
