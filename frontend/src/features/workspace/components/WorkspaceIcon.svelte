<script lang="ts">
  let { name, size = 18 }: { name: string; size?: number } = $props();
</script>

<svg
  class="workspace-control"
  class:workspace-directional-icon={name === 'arrow'}
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="1.7"
  stroke-linecap="round"
  stroke-linejoin="round"
  aria-hidden="true"
>
  {#if name === 'folder'}<path d="M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" />
  {:else if name === 'cards'}<rect x="6" y="7" width="14" height="14" rx="2" /><path
      d="M16 3H4a1 1 0 0 0-1 1v12m7-4h6m-6 4h4"
    />
  {:else if name === 'search'}<circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4.5 4.5" />
  {:else if name === 'sidebar'}<rect x="3" y="4" width="18" height="16" rx="2" /><path d="M9 4v16" />
  {:else if name === 'plus'}<path d="M12 5v14M5 12h14" />
  {:else if name === 'chevron'}<path d="m9 5 7 7-7 7" />
  {:else if name === 'arrow'}<path d="M4 12h16m-6-6 6 6-6 6" />
  {:else if name === 'import'}<path d="M12 3v12m-4-4 4 4 4-4M4 15v5h16v-5" />
  {:else if name === 'export'}<path d="M12 15V3m-4 4 4-4 4 4M4 15v5h16v-5" />
  {:else if name === 'check'}<path d="m5 12 4 4L19 6" />
  {:else if name === 'clock'}<circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" />
  {:else if name === 'close'}<path d="m6 6 12 12M18 6 6 18" />
  {:else if name === 'trash'}<path d="M3 6h18M9 6V4h6v2M5 6l1 14h12l1-14M10 10v6M14 10v6" />
  {:else if name === 'repeat'}<path d="M20 7h-9a7 7 0 1 0 7 10M16 3l4 4-4 4" />{/if}
</svg>
