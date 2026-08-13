<script lang="ts">
  import { tick } from 'svelte'
  import { formatDate } from '../lib/datetime'
  import { defaultFirstRefs } from '../lib/remotes'
  import type { SearchResult } from '../lib/types'

  export let results: SearchResult[] = []
  export let selectedIndex = -1
  export let searching = false
  export let hasSearched = false
  export let hasPreviousResults = false
  export let filteredOut = false
  export let error = ''
  export let defaultBranch = ''
  export let onRetry: () => void
  export let onSelect: (index: number) => void

  function displayMessage(message: string): string {
    return message.split('\n')[0]
  }

  function sourceLabel(source: string): string {
    return source === 'msg' ? 'Message' : source.toUpperCase()
  }

  let rowsRoot: HTMLElement | undefined

  function selectResult(index: number): void {
    selectedIndex = index
    onSelect(index)
  }

  function focusResultRow(index: number): void {
    const row = rowsRoot?.querySelectorAll<HTMLElement>('.result-row')[index]
    if (!row) return
    row.focus({ preventScroll: true })
    row.scrollIntoView({ block: 'nearest' })
  }

  function handleRowKeydown(event: KeyboardEvent, index: number): void {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      selectResult(index)
      return
    }
    const step = event.key === 'PageDown' || event.key === 'PageUp' ? 10 : 1
    const last = results.length - 1
    let next = index
    if (event.key === 'ArrowDown' || event.key === 'PageDown') next = Math.min(last, index + step)
    else if (event.key === 'ArrowUp' || event.key === 'PageUp') next = Math.max(0, index - step)
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = last
    else return

    event.preventDefault()
    if (next === index) return
    selectResult(next)
    void tick().then(() => focusResultRow(next))
  }
</script>

<div class="results-table">
  <div class="results-body">
    {#if error && results.length > 0}
      <div class="criteria-summary-bar filter-rules" role="alert">
        <span class="criteria-summary-label">Search</span>
        <div class="criteria-summary-items">
          <span class="filter-rule action-hide"><strong>Failed</strong><span>{error} · Showing previous successful results.</span></span>
          <button class="history-action" type="button" on:click={onRetry}>Retry</button>
        </div>
      </div>
    {:else if searching && results.length > 0}
      <div class="criteria-summary-bar" role="status" aria-live="polite">
        <span class="criteria-summary-label">Search</span>
        <div class="criteria-summary-items">
          <span class="filter-rule"><strong>Running</strong><span>Showing previous successful results until this search succeeds.</span></span>
        </div>
      </div>
    {/if}
    {#if results.length === 0}
      <div class="results-empty">
        {#if searching}
          <span class="empty-spinner"></span>
          <strong>Searching selected history…</strong>
          <p>Results will appear after GitGit scans the revision scope.</p>
        {:else if error}
          <span class="empty-symbol">!</span>
          <strong>Search failed</strong>
          <p>{error}</p>
          <p>{hasPreviousResults ? 'Previous successful results are preserved but hidden by active filters.' : hasSearched ? 'The previous successful search had no matches; no results were replaced.' : 'No results were replaced.'}</p>
          <button class="history-load-more" type="button" on:click={onRetry}>Retry current search</button>
        {:else if filteredOut}
          <span class="empty-symbol">∅</span>
          <strong>No results match active filters</strong>
          <p>Disable a Filter or Preset to show more search results.</p>
        {:else if hasSearched}
          <span class="empty-symbol">∅</span>
          <strong>No matches</strong>
          <p>Adjust a pattern, author, date, or revision scope.</p>
        {:else}
          <span class="empty-symbol">⌕</span>
          <strong>Compose a history search</strong>
          <p>Add at least one Message, DIFF, or FILE pattern above.</p>
        {/if}
      </div>
    {:else}
      <div bind:this={rowsRoot} class="results-rows" role="table" aria-label="Search results">
      <div role="rowgroup">
      {#each results as result, index}
        {@const refs = defaultFirstRefs(result.refs, defaultBranch)}
        <div
          class:selected={selectedIndex === index}
          class="result-row result-grid"
          role="row"
          tabindex={index === Math.max(selectedIndex, 0) ? 0 : -1}
          aria-selected={selectedIndex === index}
          on:click={() => selectResult(index)}
          on:keydown={(event) => handleRowKeydown(event, index)}
        >
          <span class="commit-cell" role="cell"><code>{result.short_commit}</code></span>
          <span class="message-cell" role="cell" title={result.message}>
            <strong>{displayMessage(result.message)}</strong>
            {#if refs.length}
              <span class="result-ref-list">
                {#each refs.slice(0, 2) as ref}<small title={ref}>{ref}</small>{/each}
                {#if refs.length > 2}<small title={refs.slice(2).join('\n')}>+{refs.length - 2}</small>{/if}
              </span>
            {/if}
          </span>
          <span role="cell">{result.author.name}</span>
          <span role="cell">{formatDate(result.date)}</span>
          <span class="match-cell" role="cell">
            {#each result.match_sources as source}
              <b class="source-{source}">{sourceLabel(source)}</b>
            {/each}
          </span>
        </div>
      {/each}
      </div>
      </div>
    {/if}
  </div>
</div>
