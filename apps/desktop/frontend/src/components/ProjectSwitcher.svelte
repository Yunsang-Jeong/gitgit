<script lang="ts">
  import { onMount, tick } from 'svelte'
  import type { RegisteredProject } from '../lib/types'

  export let projects: RegisteredProject[] = []
  export let activeProjectRoot = ''
  export let disabled = false
  export let showFavorites = true
  export let onRegister: () => void
  export let onSelect: (project: RegisteredProject) => void
  export let onToggleFavorite: (project: RegisteredProject) => void
  export let onUnregister: (project: RegisteredProject) => void

  let open = false
  let query = ''
  let root: HTMLDivElement
  let searchInput: HTMLInputElement

  $: activeProject = projects.find((project) => project.root === activeProjectRoot)
  $: activeProjectDisplayName = activeProject?.name ?? projectNameFromRoot(activeProjectRoot)
  $: favorites = projects.filter((project) => project.favorite)
  $: filteredProjects = projects
    .map((project, index) => ({ project, index }))
    .filter(({ project }) => matchesQuery(project))
    .sort((left, right) => Number(right.project.favorite) - Number(left.project.favorite) || left.index - right.index)
    .map(({ project }) => project)

  onMount(() => {
    const closeOutside = (event: PointerEvent) => {
      if (open && event.target instanceof Node && !root?.contains(event.target)) close()
    }
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && open) close()
    }
    window.addEventListener('pointerdown', closeOutside)
    window.addEventListener('keydown', closeOnEscape)
    return () => {
      window.removeEventListener('pointerdown', closeOutside)
      window.removeEventListener('keydown', closeOnEscape)
    }
  })

  function matchesQuery(project: RegisteredProject): boolean {
    const pattern = query.trim().toLocaleLowerCase()
    return !pattern || project.name.toLocaleLowerCase().includes(pattern) || project.root.toLocaleLowerCase().includes(pattern)
  }

  async function toggle(): Promise<void> {
    open = !open
    if (!open) {
      query = ''
      return
    }
    await tick()
    searchInput?.focus()
  }

  function close(): void {
    open = false
    query = ''
  }

  function select(project: RegisteredProject): void {
    onSelect(project)
    close()
  }

  function register(): void {
    onRegister()
    close()
  }

  function projectNameFromRoot(projectRoot: string): string {
    const root = projectRoot.replace(/[\\/]+$/, '')
    return root.slice(Math.max(root.lastIndexOf('/'), root.lastIndexOf('\\')) + 1) || 'Select project'
  }
</script>

<div class="project-switcher" bind:this={root}>
  <button
    class:open
    class="project-switcher-trigger"
    type="button"
    {disabled}
    title={activeProject?.root ?? 'Select or register a Git project'}
    aria-haspopup="dialog"
    aria-expanded={open}
    on:click={() => void toggle()}
  >
    <span class="project-trigger-label">Project</span>
    <strong>{activeProjectDisplayName}</strong>
    <svg class="project-chevron" viewBox="0 0 16 16" aria-hidden="true"><path d="m4 6 4 4 4-4" /></svg>
  </button>

  {#if open}
    <div class="project-menu" role="dialog" aria-label="Select project">
      <div class="project-search">
        <svg class="project-search-icon" viewBox="0 0 16 16" aria-hidden="true">
          <circle cx="6.5" cy="6.5" r="3.5" />
          <path d="m9.2 9.2 3 3" />
        </svg>
        <input bind:this={searchInput} bind:value={query} type="search" placeholder="Search projects…" aria-label="Search registered projects" />
      </div>
      <div class="project-menu-list" role="listbox" aria-label="Registered projects">
        {#if filteredProjects.length === 0}
          <p class="project-menu-empty">No matching projects</p>
        {:else}
          {#each filteredProjects as project}
            <div class:active={project.root === activeProjectRoot} class="project-option">
              <button
                class="project-option-select"
                type="button"
                {disabled}
                role="option"
                aria-selected={project.root === activeProjectRoot}
                title={project.root}
                on:click={() => select(project)}
              >
                <span class="project-option-marker"></span>
                <span class="project-option-copy">
                  <strong>{project.name}</strong>
                  <small>{project.root}</small>
                </span>
              </button>
              <button
                class:favorite={project.favorite}
                class="project-favorite-toggle"
                type="button"
                {disabled}
                title={project.favorite ? `Remove ${project.name} from favorites` : `Add ${project.name} to favorites`}
                aria-label={project.favorite ? `Remove ${project.name} from favorites` : `Add ${project.name} to favorites`}
                on:click={() => onToggleFavorite(project)}
              >{project.favorite ? '★' : '☆'}</button>
              <button
                class="project-remove-toggle"
                type="button"
                {disabled}
                title={`Unregister ${project.name}`}
                aria-label={`Unregister ${project.name}`}
                on:click={() => onUnregister(project)}
              >×</button>
            </div>
          {/each}
        {/if}
      </div>
      <button class="register-project-action" type="button" {disabled} on:click={register}>
        <span>＋</span>
        <span>Register Git repository</span>
      </button>
    </div>
  {/if}
</div>

{#if showFavorites}
  <div class="favorite-projects" aria-label="Favorite projects">
    {#each favorites as project}
      <button
        class:active={project.root === activeProjectRoot}
        class="favorite-project-button"
        type="button"
        {disabled}
        title={project.root}
        on:click={() => onSelect(project)}
      ><span>★</span><strong>{project.name}</strong></button>
    {/each}
  </div>
{/if}
