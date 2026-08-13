<script lang="ts">
  import type { Pattern } from '../lib/types'
  import { formatLocalDateTimeInput } from '../lib/datetime'
  import { parseSearchExpression, searchExpressionText } from '../lib/search-expression'

  export let patterns: Pattern[] = []
  export let engine = 'glob'
  export let author = ''
  export let since = ''
  export let until = ''
  export let stale = false
  export let applied = false
  export let queryError = ''
  export let onSearch: () => void

  let expressionDraft = searchExpressionText(patterns)
  let observedPatterns = patterns
  let updatingFromExpression = false

  $: if (patterns !== observedPatterns) {
    observedPatterns = patterns
    if (!updatingFromExpression) {
      expressionDraft = searchExpressionText(patterns)
      queryError = ''
    }
    updatingFromExpression = false
  }
  function updateExpression(event: Event): void {
    expressionDraft = (event.currentTarget as HTMLInputElement).value
    const parsed = parseSearchExpression(expressionDraft)
    queryError = parsed.error
    if (parsed.error) return
    updatingFromExpression = true
    patterns = parsed.patterns
  }

  function handleExpressionKeydown(event: KeyboardEvent): void {
    if (event.key !== 'Enter') return
    event.preventDefault()
    if (!queryError && patterns.length > 0) onSearch()
  }

  function applyDateTime(event: Event, boundary: 'since' | 'until'): void {
    const input = event.currentTarget as HTMLInputElement
    const formatted = formatLocalDateTimeInput(input.value)
    if (!formatted) return
    if (boundary === 'since') since = formatted
    else until = formatted
    input.value = ''
  }
</script>

<section class="search-composer" aria-label="Search composer">
  <div class="search-expression-composer">
    <label class:error={Boolean(queryError)} class="search-expression-field">
      <span>Expression</span>
      <input
        data-pattern-input
        value={expressionDraft}
        on:input={updateExpression}
        on:keydown={handleExpressionKeydown}
        placeholder="MSG: *cache* OR (DIFF: *timeout* AND FILE: **/*.go)"
        aria-label="Search expression"
        aria-describedby="search-expression-helper"
        aria-invalid={Boolean(queryError)}
        spellcheck="false"
        autocomplete="off"
      />
    </label>
    <p
      id="search-expression-helper"
      class:error={Boolean(queryError)}
      class:stale={stale && !queryError}
      class="search-expression-helper"
      aria-live={expressionDraft.trim() || queryError ? 'polite' : 'off'}
    >
      <span aria-hidden="true">{queryError ? '!' : stale ? '↻' : '?'}</span>
      {#if queryError}
        {queryError}
      {:else if !expressionDraft.trim()}
        MSG: searches commit messages, DIFF: searches added or deleted line content, and FILE: searches changed paths. Combine conditions with AND / OR; use ( ) for priority and quotes around values with spaces.
      {:else if stale}
        Expression changed · Press Enter or Search to update the current results.
      {:else if applied}
        {patterns.length} {patterns.length === 1 ? 'condition' : 'conditions'} · Current results use this expression.
      {:else if engine === 'regex'}
        {patterns.length} {patterns.length === 1 ? 'condition' : 'conditions'} · Values use Go regular-expression syntax. Press Enter to search.
      {:else}
        {patterns.length} {patterns.length === 1 ? 'condition' : 'conditions'} · Use * for text and ** across directories. Press Enter to search.
      {/if}
    </p>
  </div>

  <div class="filter-row">
    <label>
      <span>Engine</span>
      <select bind:value={engine}>
        <option value="glob">Glob</option>
        <option value="regex">Regex</option>
      </select>
    </label>
    <label>
      <span>Author</span>
      <input bind:value={author} placeholder="Anyone" />
    </label>
    <div class="filter-field">
      <label for="search-since-input">Since</label>
      <div class="search-date-control">
        <input id="search-since-input" bind:value={since} list="search-since-options" placeholder="Any time" aria-label="Search since date" />
        <input
          class="search-date-picker"
          type="datetime-local"
          step="60"
          aria-label="Choose search since date and time"
          title="Choose date and time"
          on:change={(event) => applyDateTime(event, 'since')}
        />
      </div>
      <datalist id="search-since-options">
        <option value="last:3d"></option>
        <option value="last:30d"></option>
        <option value="2026. 7. 19."></option>
        <option value="2026. 7. 19. 09:30:00"></option>
      </datalist>
    </div>
    <div class="filter-field">
      <label for="search-until-input">Until</label>
      <div class="search-date-control">
        <input id="search-until-input" bind:value={until} list="search-until-options" placeholder="Now" aria-label="Search until date" />
        <input
          class="search-date-picker"
          type="datetime-local"
          step="60"
          aria-label="Choose search until date and time"
          title="Choose date and time"
          on:change={(event) => applyDateTime(event, 'until')}
        />
      </div>
      <datalist id="search-until-options">
        <option value="2026. 7. 19."></option>
        <option value="2026. 7. 19. 18:00:00"></option>
      </datalist>
    </div>
  </div>
</section>
