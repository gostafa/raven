<script lang="ts">
  let {
    width = '100%',
    height = '1em',
    radius = '4px',
  }: { width?: string; height?: string; radius?: string } = $props();
</script>

<span
  class="skeleton"
  aria-hidden="true"
  style:inline-size={width}
  style:block-size={height}
  style:border-radius={radius}
></span>

<style>
  .skeleton {
    display: block;
    flex-shrink: 0;
    max-inline-size: 100%;
    background: var(--line);
    animation: pulse 1.8s ease-in-out infinite;
    pointer-events: none;
  }
  @keyframes pulse {
    50% {
      opacity: 0.45;
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .skeleton {
      animation: none;
    }
  }
</style>
