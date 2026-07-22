<script lang="ts">
  import type { Pattern } from '../lib/types'
  import { parseSearchExpression, searchExpressionText } from '../lib/search-expression'

  export let patterns: Pattern[] = []
  export let engine = 'glob'
  export let scope = 'HEAD'
  export let allRefs = false
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

  function changeScope(event: Event): void {
    scope = (event.currentTarget as HTMLInputElement).value
    allRefs = scope.trim().toLocaleLowerCase() === 'all refs'
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
        placeholder="MSG: *cache* AND FILE: **/*.go"
        aria-label="Search expression"
        aria-describedby="search-expression-helper"
        aria-invalid={Boolean(queryError)}
        spellcheck="false"
        autocomplete="off"
      />
    </label>
    <p id="search-expression-helper" class:error={Boolean(queryError)} class:stale={stale && !queryError} class="search-expression-helper" aria-live="polite">
      <span aria-hidden="true">{queryError ? '!' : stale ? '↻' : '?'}</span>
      {#if queryError}
        {queryError}
      {:else if !expressionDraft.trim()}
        Start with MSG:, DIFF:, or FILE:. Combine conditions with AND / OR and use ( ) for priority.
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
      <span>Scope</span>
      <input class="scope-input" value={scope} on:input={changeScope} list="scope-options" placeholder="HEAD or revision" />
      <datalist id="scope-options">
        <option value="HEAD"></option>
        <option value="All refs"></option>
      </datalist>
    </label>
    <label>
      <span>Author</span>
      <input bind:value={author} placeholder="Anyone" />
    </label>
    <label>
      <span>Since</span>
      <input bind:value={since} list="search-since-options" placeholder="Any time" aria-label="Search since date" />
      <datalist id="search-since-options">
        <option value="last:3d"></option>
        <option value="last:30d"></option>
        <option value="2026. 7. 19."></option>
        <option value="2026. 7. 19. 09:30:00"></option>
      </datalist>
    </label>
    <label>
      <span>Until</span>
      <input bind:value={until} list="search-until-options" placeholder="Now" aria-label="Search until date" />
      <datalist id="search-until-options">
        <option value="2026. 7. 19."></option>
        <option value="2026. 7. 19. 18:00:00"></option>
      </datalist>
    </label>
  </div>
</section>
