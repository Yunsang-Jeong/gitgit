<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { defaultWorktreeName, resolveWorktreeDestination } from '../lib/worktree-actions'
  import type { CreateWorktreeRequest, RepositoryState } from '../lib/types'

  export let repository: RepositoryState
  export let busy = false
  export let onChooseParent: (current: string) => Promise<string>
  export let onSuggestParent: () => Promise<string>
  // Returns an empty string on success, or the message to show in place.
  // The status bar sits behind this dialog, so a failure reported only there
  // would be hidden by the very form that caused it.
  export let onCreate: (request: CreateWorktreeRequest) => Promise<string>
  export let onClose: () => void

  type Mode = 'new-branch' | 'existing-branch' | 'detached'

  let parentDirectory = ''
  let name = ''
  let mode: Mode = 'new-branch'
  let newBranch = ''
  let revision = ''
  let nameEdited = false
  let error = ''
  let nameInput: HTMLInputElement
  let dialog: HTMLDivElement

  // Branches already checked out elsewhere are refused by the backend, so they
  // are not offered as a start point in the first place.
  $: checkedOutBranches = new Set(
    repository.worktrees.filter((worktree) => !worktree.detached && worktree.branch).map((worktree) => worktree.branch as string),
  )
  $: availableBranches = [...new Set(repository.worktrees.map((worktree) => worktree.branch).filter((branch): branch is string => Boolean(branch)))]
    .concat(repository.default_branch)
    .filter((branch, index, all) => branch && all.indexOf(branch) === index && !checkedOutBranches.has(branch))
    .sort((left, right) => left.localeCompare(right))
  $: startPoint = mode === 'existing-branch' ? revision : mode === 'new-branch' ? revision : ''
  // The name follows whichever ref the user picked until they type their own.
  $: if (!nameEdited) {
    const source = mode === 'new-branch' ? newBranch : revision
    name = source ? defaultWorktreeName(source) : ''
  }
  $: destination = resolveWorktreeDestination(parentDirectory, name)
  $: missingRef =
    mode === 'new-branch'
      ? newBranch.trim() === ''
        ? 'Enter a name for the new branch.'
        : ''
      : mode === 'existing-branch' && revision.trim() === ''
        ? 'Choose a branch to check out.'
        : ''
  $: blocked = destination.error || missingRef
  $: canSubmit = !busy && blocked === ''

  onMount(() => {
    void initialize()
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && !busy) onClose()
    }
    window.addEventListener('keydown', closeOnEscape)
    return () => window.removeEventListener('keydown', closeOnEscape)
  })

  async function initialize(): Promise<void> {
    try {
      parentDirectory = await onSuggestParent()
    } catch {
      parentDirectory = ''
    }
    await tick()
    nameInput?.focus()
  }

  async function chooseParent(): Promise<void> {
    try {
      const chosen = await onChooseParent(parentDirectory)
      if (chosen) parentDirectory = chosen
    } catch (cause) {
      error = cause instanceof Error ? cause.message : String(cause)
    }
  }

  function editName(event: Event): void {
    nameEdited = true
    name = (event.currentTarget as HTMLInputElement).value
  }

  function selectMode(next: Mode): void {
    mode = next
    error = ''
    if (next === 'detached' && !revision) revision = repository.default_branch
  }

  async function submit(): Promise<void> {
    if (!canSubmit) return
    error = ''
    const failure = await onCreate({
      location: { parent_directory: parentDirectory.trim(), name: name.trim() },
      revision: (mode === 'detached' ? revision : startPoint).trim(),
      new_branch: mode === 'new-branch' ? newBranch.trim() : '',
      detach: mode === 'detached',
      // Network work belongs to the Commit screen; the backend refuses any
      // other value here.
      sync: 'none',
    })
    if (failure) error = failure
    else onClose()
  }
</script>

<div class="worktree-confirm-backdrop" role="presentation" on:mousedown={() => { if (!busy) onClose() }}>
  <div
    bind:this={dialog}
    class="worktree-confirm worktree-dialog"
    role="dialog"
    aria-modal="true"
    aria-labelledby="create-worktree-title"
    tabindex="-1"
    on:mousedown|stopPropagation
  >
    <h2 id="create-worktree-title">New worktree</h2>
    <p>A linked worktree checks out another branch beside this one, without touching the current checkout.</p>

    <div class="worktree-field">
      <label for="create-worktree-parent">Location</label>
      <div class="worktree-field-row">
        <input id="create-worktree-parent" type="text" bind:value={parentDirectory} disabled={busy} spellcheck="false" />
        <button type="button" disabled={busy} on:click={() => void chooseParent()}>Choose…</button>
      </div>
    </div>

    <div class="worktree-field">
      <label for="create-worktree-name">Directory name</label>
      <input id="create-worktree-name" bind:this={nameInput} type="text" value={name} disabled={busy} spellcheck="false" on:input={editName} />
    </div>

    <div class="worktree-field">
      <span class="worktree-field-label">Checkout</span>
      <div class="worktree-mode-row" role="radiogroup" aria-label="What to check out">
        <label><input type="radio" checked={mode === 'new-branch'} disabled={busy} on:change={() => selectMode('new-branch')} /><span>New branch</span></label>
        <label><input type="radio" checked={mode === 'existing-branch'} disabled={busy} on:change={() => selectMode('existing-branch')} /><span>Existing branch</span></label>
        <label><input type="radio" checked={mode === 'detached'} disabled={busy} on:change={() => selectMode('detached')} /><span>Detached</span></label>
      </div>
    </div>

    {#if mode === 'new-branch'}
      <div class="worktree-field">
        <label for="create-worktree-branch">New branch name</label>
        <input id="create-worktree-branch" type="text" bind:value={newBranch} disabled={busy} spellcheck="false" placeholder="feature/thing" />
      </div>
      <div class="worktree-field">
        <label for="create-worktree-start">Start point <small>optional</small></label>
        <input id="create-worktree-start" type="text" bind:value={revision} disabled={busy} spellcheck="false" placeholder={repository.default_branch} />
      </div>
    {:else}
      <div class="worktree-field">
        <label for="create-worktree-revision">{mode === 'detached' ? 'Revision' : 'Branch'}</label>
        {#if mode === 'existing-branch' && availableBranches.length > 0}
          <select id="create-worktree-revision" bind:value={revision} disabled={busy}>
            <option value="">Choose a branch…</option>
            {#each availableBranches as branch}<option value={branch}>{branch}</option>{/each}
          </select>
        {:else}
          <input id="create-worktree-revision" type="text" bind:value={revision} disabled={busy} spellcheck="false" placeholder={repository.default_branch} />
        {/if}
      </div>
    {/if}

    <dl class="worktree-dialog-preview">
      <dt>Creates</dt>
      <dd><code>{destination.path || '—'}</code></dd>
    </dl>

    {#if blocked}<p class="worktree-dialog-hint">{blocked}</p>{/if}
    {#if error}<p class="worktree-dialog-error" role="alert">{error}</p>{/if}

    <div class="worktree-confirm-actions">
      <button type="button" disabled={busy} on:click={onClose}>Cancel</button>
      <button class="primary" type="button" disabled={!canSubmit} on:click={() => void submit()}>{busy ? 'Creating…' : 'Create worktree'}</button>
    </div>
  </div>
</div>
