<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { resolveWorktreeDestination, worktreeName, worktreeParentDirectory, worktreePathTail } from '../lib/worktree-actions'
  import type { MoveWorktreeRequest, WorktreeInfo } from '../lib/types'

  export let worktree: WorktreeInfo
  export let busy = false
  export let onChooseParent: (current: string) => Promise<string>
  // Returns an empty string on success, or the message to show in place.
  export let onMove: (request: MoveWorktreeRequest) => Promise<string>
  export let onClose: () => void

  let parentDirectory = worktreeParentDirectory(worktree.path)
  let name = worktreePathTail(worktree.path)
  let error = ''
  let nameInput: HTMLInputElement

  $: destination = resolveWorktreeDestination(parentDirectory, name)
  $: unchanged = destination.path === worktree.path
  $: blocked = destination.error || (unchanged ? 'Choose a different location or name.' : '')
  $: canSubmit = !busy && blocked === ''

  onMount(() => {
    void focusName()
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onClose()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  })

  async function focusName(): Promise<void> {
    await tick()
    nameInput?.focus()
    nameInput?.select()
  }

  async function chooseParent(): Promise<void> {
    try {
      const chosen = await onChooseParent(parentDirectory)
      if (chosen) parentDirectory = chosen
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause)
    }
  }

  async function submit(): Promise<void> {
    if (!canSubmit) return
    error = ''
    const failure = await onMove({
      path: worktree.path,
      location: { parent_directory: parentDirectory.trim(), name: name.trim() },
    })
    if (failure) error = failure
    else onClose()
  }
</script>

<div class="worktree-confirm-backdrop" role="presentation" on:mousedown={() => { if (!busy) onClose() }}>
  <div
    class="worktree-confirm worktree-dialog"
    role="dialog"
    aria-modal="true"
    aria-labelledby="move-worktree-title"
    tabindex="-1"
    on:mousedown|stopPropagation
  >
    <h2 id="move-worktree-title">Move {worktreeName(worktree)}</h2>
    <p>The checkout is relocated and Git is told where it went. Its branch, commits and local changes are untouched.</p>

    <div class="worktree-field">
      <label for="move-worktree-parent">New location</label>
      <div class="worktree-field-row">
        <input id="move-worktree-parent" type="text" bind:value={parentDirectory} disabled={busy} spellcheck="false" />
        <button type="button" disabled={busy} on:click={() => void chooseParent()}>Choose…</button>
      </div>
    </div>

    <div class="worktree-field">
      <label for="move-worktree-name">Directory name</label>
      <input id="move-worktree-name" bind:this={nameInput} type="text" bind:value={name} disabled={busy} spellcheck="false" />
    </div>

    <dl class="worktree-dialog-preview">
      <dt>From</dt>
      <dd><code>{worktree.path}</code></dd>
      <dt>To</dt>
      <dd><code>{destination.path || '—'}</code></dd>
    </dl>

    {#if blocked}<p class="worktree-dialog-hint">{blocked}</p>{/if}
    {#if error}<p class="worktree-dialog-error" role="alert">{error}</p>{/if}

    <div class="worktree-confirm-actions">
      <button type="button" disabled={busy} on:click={onClose}>Cancel</button>
      <button class="primary" type="button" disabled={!canSubmit} on:click={() => void submit()}>{busy ? 'Moving…' : 'Move worktree'}</button>
    </div>
  </div>
</div>
