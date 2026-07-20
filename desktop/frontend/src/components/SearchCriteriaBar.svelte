<script lang="ts">
  import type { Pattern } from '../lib/types'

  export let patterns: Pattern[] = []
  export let stale = false
  export let applied = false
  export let onRemove: (index: number) => void

  function sourceLabel(source: Pattern['source']): string {
    return source === 'msg' ? 'Message' : source.toUpperCase()
  }
</script>

{#if patterns.length > 0 || stale}
  <section class="criteria-summary-bar search-criteria-bar" aria-label="Search patterns">
    <span class="criteria-summary-label">Search</span>
    <div class="criteria-summary-items search-criteria-patterns">
      {#each patterns as pattern, index}
        {#if index > 0}
          <span class="search-criteria-join">{(pattern.join ?? 'or').toUpperCase()}</span>
        {/if}
        <span class="search-criteria-chip source-{pattern.source}" title={`${sourceLabel(pattern.source)} ${pattern.value}`}>
          <strong>{sourceLabel(pattern.source)}</strong>
          <code>{pattern.value}</code>
          <button
            type="button"
            on:click={() => onRemove(index)}
            aria-label="Remove {sourceLabel(pattern.source)} pattern from the next search"
          >×</button>
        </span>
      {/each}
      {#if stale}
        <span class="search-criteria-chip source-diff" role="status" title="Search draft changed after these results were produced">
          <strong>Draft</strong>
          <code>Changes not applied</code>
        </span>
      {:else if applied}
        <span class="search-criteria-chip" role="status" title="These patterns produced the current results">
          <strong>Applied</strong>
          <code>Current results</code>
        </span>
      {/if}
    </div>
  </section>
{/if}
