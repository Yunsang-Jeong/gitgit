<script lang="ts">
  import type { ContextMenuItem } from '../lib/types'

  export let x = 0
  export let y = 0
  export let items: ContextMenuItem[] = []
  export let ariaLabel = 'Context actions'
  export let onClose: () => void

  $: width = 228
  $: estimatedHeight = items.length * 31 + items.filter((item) => item.separatorBefore).length * 7 + 10
  $: left = Math.max(8, Math.min(x, window.innerWidth - width - 8))
  $: top = Math.max(8, Math.min(y, window.innerHeight - estimatedHeight - 8))

  async function choose(item: ContextMenuItem): Promise<void> {
    if (item.disabled) return
    onClose()
    await item.run()
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
    }
  }
</script>

<svelte:window on:keydown={handleKeydown} />
<div class="context-menu-scrim" role="presentation" on:pointerdown={onClose}>
  <div
    class="context-menu"
    role="menu"
    tabindex="-1"
    aria-label={ariaLabel}
    style={`left: ${left}px; top: ${top}px; width: ${width}px`}
    on:pointerdown|stopPropagation
  >
    {#each items as item}
      {#if item.separatorBefore}<span class="context-menu-separator" aria-hidden="true"></span>{/if}
      <button
        class:danger={item.danger}
        type="button"
        role="menuitem"
        disabled={item.disabled}
        on:click={() => void choose(item)}
      >
        <span>{item.label}</span>
        {#if item.shortcut}<kbd>{item.shortcut}</kbd>{/if}
      </button>
    {/each}
  </div>
</div>
