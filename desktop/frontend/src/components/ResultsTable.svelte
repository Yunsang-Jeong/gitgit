<script lang="ts">
  import { isCommitHighlighted } from '../lib/history'
  import { formatDate } from '../lib/datetime'
  import { defaultFirstRefs } from '../lib/remotes'
  import type { CommitFilterRule, SearchResult } from '../lib/types'

  export let results: SearchResult[] = []
  export let selectedIndex = -1
  export let searching = false
  export let hasSearched = false
  export let hasPreviousResults = false
  export let rules: CommitFilterRule[] = []
  export let filteredOut = false
  export let error = ''
  export let defaultBranch = ''
  export let onRetry: () => void

  function displayMessage(message: string): string {
    return message.split('\n')[0]
  }

  function displayFile(result: SearchResult): string {
    const matchedFiles = result.matched_files ?? [result.file]
    if (matchedFiles.length > 1) return `${matchedFiles.length} changed files`
    const file = matchedFiles[0]
    return file.old_path ? `${file.old_path} → ${file.path}` : file.path
  }

  function displayFileTitle(result: SearchResult): string {
    return (result.matched_files ?? [result.file])
      .map((file) => file.old_path ? `${file.old_path} → ${file.path}` : file.path)
      .join('\n')
  }

  function sourceLabel(source: string): string {
    return source === 'msg' ? 'Message' : source.toUpperCase()
  }
</script>

<div class="results-table" role="table" aria-label="Search results">
  <div class="results-header result-grid" role="row">
    <span aria-label="Topology"></span>
    <span>Commit</span>
    <span>Message</span>
    <span>Author</span>
    <span>Date</span>
    <span>File</span>
    <span>Match</span>
  </div>
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
      {#each results as result, index}
        {@const refs = defaultFirstRefs(result.refs, defaultBranch)}
        <button
          class:selected={selectedIndex === index}
          class:highlighted={isCommitHighlighted(result, rules)}
          class="result-row result-grid"
          type="button"
          role="row"
          on:click={() => (selectedIndex = index)}
        >
          <span class="topology-cell" role="cell"><i class="lane-dot"></i></span>
          <span class="commit-cell" role="cell">
            <code>{result.short_commit}</code>
            {#if refs.length}<small>{refs[0]}</small>{/if}
          </span>
          <span class="message-cell" role="cell" title={result.message}>{displayMessage(result.message)}</span>
          <span role="cell">{result.author.name}</span>
          <span role="cell">{formatDate(result.date)}</span>
          <span class="file-cell" role="cell" title={displayFileTitle(result)}>{displayFile(result)}</span>
          <span class="match-cell" role="cell">
            {#each result.match_sources as source}
              <b class="source-{source}">{sourceLabel(source)}</b>
            {/each}
          </span>
        </button>
      {/each}
    {/if}
  </div>
</div>
