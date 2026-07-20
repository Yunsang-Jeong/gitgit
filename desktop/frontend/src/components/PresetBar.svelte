<script lang="ts">
  import { presetUnavailable } from '../lib/presets'
  import type { Author, CommitFilterPreset } from '../lib/types'

  export let presets: CommitFilterPreset[] = []
  export let activeIDs: string[] = []
  export let author: Author = { name: '', email: '' }
  export let onToggle: (id: string) => void

  function description(preset: CommitFilterPreset): string {
    return preset.rules.map((rule) => `${rule.action.toUpperCase()} ${rule.field}: ${rule.pattern}`).join(' · ')
  }
</script>

<div class="preset-controls" aria-label="Commit filter presets">
  {#each presets as preset (preset.id)}
    <button
      class:active={activeIDs.includes(preset.id)}
      type="button"
      aria-pressed={activeIDs.includes(preset.id)}
      title={description(preset)}
      disabled={presetUnavailable(preset, author)}
      on:click={() => onToggle(preset.id)}
    >
      <span class="preset-button-state"></span>
      {preset.label}
    </button>
  {:else}
    <small>Add presets in Settings.</small>
  {/each}
</div>
