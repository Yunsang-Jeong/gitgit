<script lang="ts">
  import RemoteBadgeIcon from './RemoteBadgeIcon.svelte'
  import { matchingBranches, nextVisibleBranchCount } from '../lib/branch-options'
  import { resolveRefBadge } from '../lib/remotes'
  import type { RemoteBadgeRule, RemoteBranchCatalogEntry } from '../lib/types'

  export let catalog: RemoteBranchCatalogEntry[] = []
  export let repositoryOpen = false
  export let remoteBadgeRules: RemoteBadgeRule[] = []
  export let onRetryRemote: (remote: string) => void = () => undefined

  const pageSize = 25
  let queries: Record<string, string> = {}
  let visibleCounts: Record<string, number> = {}

  function queryFor(remote: string): string {
    return queries[remote] ?? ''
  }

  function visibleCountFor(remote: string): number {
    return visibleCounts[remote] ?? pageSize
  }

  function updateQuery(remote: string, value: string): void {
    queries = { ...queries, [remote]: value }
    visibleCounts = { ...visibleCounts, [remote]: pageSize }
  }

  function matchingBranchNames(entry: RemoteBranchCatalogEntry): string[] {
    return matchingBranches(entry.branches.map((branch) => branch.name), queryFor(entry.remote.name))
  }

  function showMore(entry: RemoteBranchCatalogEntry): void {
    const total = matchingBranchNames(entry).length
    visibleCounts = {
      ...visibleCounts,
      [entry.remote.name]: nextVisibleBranchCount(visibleCountFor(entry.remote.name), total, pageSize),
    }
  }
</script>

<section class="settings-section settings-remotes-section">
  <div class="settings-section-heading">
    <div>
      <h2>Remotes</h2>
      <p>Fetched remote-tracking refs in this repository. Reading this list does not fetch, checkout, or create a local branch.</p>
    </div>
  </div>

  {#if !repositoryOpen}
    <div class="settings-project-empty">Open a project to inspect its remotes.</div>
  {:else if catalog.length === 0}
    <div class="settings-project-empty">This repository has no configured remotes.</div>
  {:else}
    <div class="settings-remote-catalog" aria-label="Remote branch catalog">
      {#each catalog as entry (entry.remote.name)}
        {@const badge = resolveRefBadge(`${entry.remote.name}/branch`, [entry.remote], remoteBadgeRules)}
        {@const matches = matchingBranchNames(entry)}
        {@const shown = matches.slice(0, visibleCountFor(entry.remote.name))}
        <details class:error={Boolean(entry.error)} class="settings-remote-card">
          <summary>
            <span class="settings-remote-summary-copy">
              <RemoteBadgeIcon name={badge.icon} size={17} />
              <strong>{entry.remote.name}</strong>
              <small title={entry.remote.url}>{entry.remote.url}</small>
            </span>
            <span class="settings-remote-summary-facts">
              {#if entry.loading}<b>Loading…</b>
              {:else if entry.error}<b class="error">Read error</b>
              {:else if entry.loaded}<b>{entry.default_branch ? `Default · ${entry.default_branch}` : 'Default · unknown'}</b><b>{entry.count} fetched</b>
              {:else}<b>Not loaded</b>{/if}
            </span>
          </summary>

          <div class="settings-remote-branches">
            {#if entry.error}
              <div class="settings-remote-error" role="alert"><span>{entry.error}</span><button type="button" on:click={() => onRetryRemote(entry.remote.name)}>Retry</button></div>
            {:else if entry.loading || !entry.loaded}
              <div class="settings-remote-state" role="status">Reading local refs…</div>
            {:else if entry.branches.length === 0}
              <div class="settings-remote-state">No fetched branches for this remote.</div>
            {:else}
              <label class="settings-remote-search">
                <span>Search branches</span>
                <input type="search" value={queryFor(entry.remote.name)} on:input={(event) => updateQuery(entry.remote.name, event.currentTarget.value)} placeholder={`Search ${entry.remote.name}…`} />
              </label>
              {#if matches.length === 0}
                <div class="settings-remote-state">No branches match “{queryFor(entry.remote.name).trim()}”.</div>
              {:else}
                <ul>
                  {#each shown as name (name)}
                    {@const branch = entry.branches.find((candidate) => candidate.name === name)}
                    <li><code>{entry.remote.name}/{name}</code>{#if branch?.default}<b>Default</b>{/if}</li>
                  {/each}
                </ul>
                {#if shown.length < matches.length}<button class="settings-remote-more" type="button" on:click={() => showMore(entry)}>Show {Math.min(pageSize, matches.length - shown.length)} more</button>{/if}
              {/if}
            {/if}
          </div>
        </details>
      {/each}
    </div>
  {/if}
</section>
