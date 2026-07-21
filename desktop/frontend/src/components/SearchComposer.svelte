<script lang="ts">
  import type { Pattern, PatternSource, SearchPatternJoin } from '../lib/types'
  import {
    groupSearchPatternRange,
    removeSearchPatternAt,
    searchExpressionError,
    searchExpressionText,
    searchPatternDepths,
    ungroupSearchPatternRange,
  } from '../lib/search-expression'

  export let patterns: Pattern[] = []
  export let engine = 'glob'
  export let scope = 'HEAD'
  export let allRefs = false
  export let author = ''
  export let since = ''
  export let until = ''
  export let onSearch: () => void

  let draftSource: PatternSource = 'msg'
  let draftValue = ''
  let draftJoin: SearchPatternJoin = 'and'
  let selectedPatternIndexes: number[] = []
  let observedPatterns = patterns

  $: expressionError = searchExpressionError(patterns)
  $: expressionText = searchExpressionText(patterns)
  $: patternDepths = searchPatternDepths(patterns)
  $: if (patterns !== observedPatterns) {
    observedPatterns = patterns
    selectedPatternIndexes = []
  }
  $: selectedRange = [...selectedPatternIndexes].sort((a, b) => a - b)
  $: selectionIsContiguous = selectedRange.every((index, offset) => offset === 0 || index === selectedRange[offset - 1] + 1)
  $: canGroupSelection = selectedRange.length >= 2
    && selectedRange[selectedRange.length - 1] < patterns.length
    && selectionIsContiguous
    && (patterns[selectedRange[0]]?.open_groups ?? 0) < 8
    && (patterns[selectedRange[selectedRange.length - 1]]?.close_groups ?? 0) < 8
  $: canUngroupSelection = canGroupSelection
    && (patterns[selectedRange[0]]?.open_groups ?? 0) > 0
    && (patterns[selectedRange[selectedRange.length - 1]]?.close_groups ?? 0) > 0

  function patternPlaceholder(source: PatternSource): string {
    return source === 'file' ? '**/*.go' : '*pattern*'
  }

  function updatePattern(index: number, patch: Partial<Pattern>): void {
    patterns = patterns.map((pattern, patternIndex) => patternIndex === index
      ? { ...pattern, ...patch, join: index === 0 ? undefined : (patch.join ?? pattern.join ?? 'or') }
      : pattern)
  }

  function removePattern(index: number): void {
    patterns = removeSearchPatternAt(patterns, index)
    selectedPatternIndexes = selectedPatternIndexes
      .filter((selectedIndex) => selectedIndex !== index)
      .map((selectedIndex) => selectedIndex > index ? selectedIndex - 1 : selectedIndex)
  }

  function addPattern(): void {
    const value = draftValue.trim()
    if (!value) return
    patterns = [...patterns, {
      source: draftSource,
      value,
      join: patterns.length > 0 ? draftJoin : undefined,
    }]
    draftValue = ''
  }

  function togglePatternSelection(index: number): void {
    selectedPatternIndexes = selectedPatternIndexes.includes(index)
      ? selectedPatternIndexes.filter((selectedIndex) => selectedIndex !== index)
      : [...selectedPatternIndexes, index]
  }

  function groupSelection(): void {
    if (!canGroupSelection) return
    patterns = groupSearchPatternRange(patterns, selectedRange[0], selectedRange[selectedRange.length - 1])
    selectedPatternIndexes = []
  }

  function ungroupSelection(): void {
    if (!canUngroupSelection) return
    patterns = ungroupSearchPatternRange(patterns, selectedRange[0], selectedRange[selectedRange.length - 1])
    selectedPatternIndexes = []
  }

  function handlePatternKeydown(event: KeyboardEvent): void {
    if (event.key === 'Enter') {
      event.preventDefault()
      if (draftValue.trim()) addPattern()
      else if (patterns.length > 0) onSearch()
    }
  }

  function handleExistingPatternKeydown(index: number, event: KeyboardEvent): void {
    if (event.key !== 'Enter') return
    event.preventDefault()
    if (!patterns[index]?.value.trim()) return
    onSearch()
  }

  function changeScope(event: Event): void {
    scope = (event.currentTarget as HTMLInputElement).value
    allRefs = scope.trim().toLocaleLowerCase() === 'all refs'
  }
</script>

<section class="search-composer" aria-label="Search composer">
  <div class="search-condition-list" aria-label="Search conditions">
    {#each patterns as pattern, index}
      <div class:grouped={patternDepths[index] > 0} class="search-condition-row">
        {#if index === 0}
          <span class="search-condition-where">Where</span>
        {:else}
          <select
            class="search-condition-join"
            value={pattern.join ?? 'or'}
            on:change={(event) => updatePattern(index, { join: (event.currentTarget as HTMLSelectElement).value as SearchPatternJoin })}
            aria-label="Condition {index + 1} operator"
            title="Condition operator"
          >
            <option value="and">AND</option>
            <option value="or">OR</option>
          </select>
        {/if}
        <label class="search-condition-select" title="Select adjacent conditions to group">
          <input type="checkbox" checked={selectedPatternIndexes.includes(index)} on:change={() => togglePatternSelection(index)} aria-label="Select condition {index + 1} for grouping" />
        </label>
        <span class="search-group-rails" aria-label={patternDepths[index] > 0 ? `Group depth ${patternDepths[index]}` : 'Not grouped'}>
          {#each Array(patternDepths[index]) as _, depth}<i style={`left: ${depth * 4}px`}></i>{/each}
        </span>
        <select
          class="search-condition-source"
          value={pattern.source}
          on:change={(event) => updatePattern(index, { source: (event.currentTarget as HTMLSelectElement).value as PatternSource })}
          aria-label="Condition {index + 1} source"
          title="Pattern source"
        >
          <option value="msg">Message</option>
          <option value="diff">DIFF</option>
          <option value="file">FILE</option>
        </select>
        <input
          class="search-condition-value"
          value={pattern.value}
          on:input={(event) => updatePattern(index, { value: (event.currentTarget as HTMLInputElement).value })}
          on:keydown={(event) => handleExistingPatternKeydown(index, event)}
          placeholder={patternPlaceholder(pattern.source)}
          aria-label="Condition {index + 1} pattern"
        />
        <button
          class="remove-search-condition"
          type="button"
          on:click={() => removePattern(index)}
          aria-label="Remove condition {index + 1}"
          title="Remove condition"
        >×</button>
      </div>
    {/each}

    <div class="search-condition-row search-condition-add-row">
      {#if patterns.length === 0}
        <span class="search-condition-where">Where</span>
      {:else}
        <select bind:value={draftJoin} class="search-condition-join" aria-label="New condition operator" title="Condition operator">
          <option value="and">AND</option>
          <option value="or">OR</option>
        </select>
      {/if}
      <span class="search-condition-select-spacer"></span>
      <span class="search-group-rails"></span>
      <select bind:value={draftSource} class="search-condition-source" aria-label="New condition source" title="Pattern source">
        <option value="msg">Message</option>
        <option value="diff">DIFF</option>
        <option value="file">FILE</option>
      </select>
      <input
        data-pattern-input
        bind:value={draftValue}
        class="search-condition-value"
        on:keydown={handlePatternKeydown}
        placeholder={patternPlaceholder(draftSource)}
        aria-label="New condition pattern"
      />
      <button class="add-pattern" type="button" on:click={addPattern} disabled={!draftValue.trim()}>＋ Condition</button>
    </div>
    <div class:error={Boolean(expressionError)} class="search-expression-preview">
      <span>Expression</span>
      <code>{expressionText || 'Add a condition to build the query'}</code>
      <div class="search-group-actions">
        <button type="button" on:click={groupSelection} disabled={!canGroupSelection} title="Select two or more adjacent conditions">Group selected</button>
        <button type="button" on:click={ungroupSelection} disabled={!canUngroupSelection} title="Select an existing group from its first through last condition">Ungroup</button>
      </div>
    </div>
    <p class:error={Boolean(expressionError)} class="search-condition-hint">{expressionError || 'Select adjacent conditions to group them. Without a group, AND is evaluated before OR. Scope, Author, and dates apply to the whole query.'}</p>
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
