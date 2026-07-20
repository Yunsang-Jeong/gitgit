<script lang="ts">
  import RemoteBadgeIcon from './RemoteBadgeIcon.svelte'
  import { isEmbeddedRemoteBadgeIcon, normalizeRemoteBadgeIcon, remoteBadgeIconOptions, resolveRefBadge } from '../lib/remotes'
  import type { ChangedFilesView, CommitFilterAction, CommitFilterField, CommitFilterPreset, IDEPreference, RegisteredProject, RemoteBadgeRule, RemoteInfo, TerminalPreference } from '../lib/types'

  export let open = false
  export let projects: RegisteredProject[] = []
  export let historyBatchSize = 0
  export let ide: IDEPreference = 'vscode'
  export let terminal: TerminalPreference = 'terminal'
  export let changedFilesView: ChangedFilesView = 'list'
  export let presets: CommitFilterPreset[] = []
  export let remotes: RemoteInfo[] = []
  export let remoteBadgeRules: RemoteBadgeRule[] = []
  export let discovering = false
  export let discoveryMessage = ''
  export let onClose: () => void
  export let onRegisterProject: () => void
  export let onDiscoverProjects: () => void
  export let onToggleFavorite: (project: RegisteredProject) => void
  export let onHistoryBatchSizeChange: (value: number) => void
  export let onIDEChange: (value: IDEPreference) => void
  export let onTerminalChange: (value: TerminalPreference) => void
  export let onChangedFilesViewChange: (value: ChangedFilesView) => void
  export let onPresetsChange: (value: CommitFilterPreset[]) => void
  export let onResetPresets: () => void
  export let onRemoteBadgeRulesChange: (value: RemoteBadgeRule[]) => void

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
    }
  }

  function changeBatchSize(event: Event): void {
    onHistoryBatchSizeChange(Number((event.currentTarget as HTMLSelectElement).value))
  }

  function changeIDE(event: Event): void {
    onIDEChange((event.currentTarget as HTMLSelectElement).value as IDEPreference)
  }

  function changeTerminal(event: Event): void {
    onTerminalChange((event.currentTarget as HTMLSelectElement).value as TerminalPreference)
  }

  function changeChangedFilesView(event: Event): void {
    onChangedFilesViewChange((event.currentTarget as HTMLSelectElement).value as ChangedFilesView)
  }

  function updatePresetLabel(id: string, label: string): void {
    onPresetsChange(presets.map((preset) => preset.id === id ? { ...preset, label } : preset))
  }

  function updatePresetRule(presetID: string, ruleID: string, key: 'action' | 'field' | 'pattern', value: string): void {
    onPresetsChange(presets.map((preset) => preset.id !== presetID ? preset : {
      ...preset,
      rules: preset.rules.map((rule) => rule.id === ruleID ? { ...rule, [key]: value } : rule),
    }))
  }

  function addPresetRule(presetID: string): void {
    const ruleID = `rule-${Date.now()}-${Math.random()}`
    onPresetsChange(presets.map((preset) => preset.id !== presetID ? preset : {
      ...preset,
      rules: [...preset.rules, { id: ruleID, action: 'highlight', field: 'message', pattern: '' }],
    }))
  }

  function removePresetRule(presetID: string, ruleID: string): void {
    onPresetsChange(presets.map((preset) => preset.id !== presetID ? preset : {
      ...preset,
      rules: preset.rules.filter((rule) => rule.id !== ruleID),
    }))
  }

  function addPreset(): void {
    const id = `preset-${Date.now()}-${Math.random()}`
    onPresetsChange([...presets, {
      id,
      label: 'New Preset',
      rules: [{ id: `${id}-rule`, action: 'highlight', field: 'message', pattern: '' }],
    }])
  }

  function removePreset(id: string): void {
    onPresetsChange(presets.filter((preset) => preset.id !== id))
  }

  function updateRemoteBadgeRule(id: string, key: 'pattern' | 'icon', value: string): void {
    onRemoteBadgeRulesChange(remoteBadgeRules.map((rule) => rule.id === id ? { ...rule, [key]: value } : rule))
  }

  function addRemoteBadgeRule(): void {
    onRemoteBadgeRulesChange([...remoteBadgeRules, {
      id: `remote-badge-${Date.now()}-${Math.random()}`,
      pattern: '',
      icon: 'remote',
    }])
  }

  function removeRemoteBadgeRule(id: string): void {
    onRemoteBadgeRulesChange(remoteBadgeRules.filter((rule) => rule.id !== id))
  }
</script>

{#if open}
  <div class="settings-backdrop" role="presentation" on:mousedown={onClose} on:keydown={handleKeydown}>
    <div class="settings-modal" role="dialog" aria-modal="true" aria-labelledby="settings-title" tabindex="-1" on:mousedown|stopPropagation>
      <header class="settings-header">
        <div>
          <h1 id="settings-title">Settings</h1>
          <span>GitGit preferences</span>
        </div>
        <kbd>⌘,</kbd>
        <button type="button" on:click={onClose} aria-label="Close settings">×</button>
      </header>

      <div class="settings-content">
        <section class="settings-section">
          <div class="settings-section-heading">
            <div>
              <h2>Projects</h2>
              <p>Register repositories individually or discover them recursively under a directory.</p>
            </div>
            <div class="settings-project-actions">
              <button type="button" on:click={onRegisterProject}>＋ Register</button>
              <button class="primary" type="button" on:click={onDiscoverProjects} disabled={discovering}>
                {discovering ? 'Discovering…' : '⌕ Discover recursively'}
              </button>
            </div>
          </div>

          {#if discoveryMessage}<p class="settings-notice">{discoveryMessage}</p>{/if}

          <div class="settings-project-list" aria-label="Registered projects">
            {#each projects as project}
              <div class="settings-project-row">
                <span class="settings-project-copy">
                  <strong>{project.name}</strong>
                  <small>{project.root}</small>
                </span>
                <button
                  class:favorite={project.favorite}
                  class="settings-favorite-button"
                  type="button"
                  on:click={() => onToggleFavorite(project)}
                  aria-label={project.favorite ? `Remove ${project.name} from favorites` : `Add ${project.name} to favorites`}
                ><span>{project.favorite ? '★' : '☆'}</span>Favorite</button>
              </div>
            {:else}
              <div class="settings-project-empty">No registered projects</div>
            {/each}
          </div>
        </section>

        <section class="settings-section settings-history-section">
          <div>
            <h2>Inspector</h2>
            <p>Sets the initial layout used for Changed Files when GitGit starts.</p>
          </div>
          <label class="settings-field">
            <span>Changed files view</span>
            <select bind:value={changedFilesView} on:change={changeChangedFilesView}>
              <option value="list">List</option>
              <option value="tree">File tree</option>
            </select>
          </label>
        </section>

        <section class="settings-section settings-history-section">
          <div>
            <h2>Commit loading</h2>
            <p>Controls how many commits Refresh and Load more request at once.</p>
          </div>
          <label class="settings-field">
            <span>Commits per refresh</span>
            <select bind:value={historyBatchSize} on:change={changeBatchSize}>
              <option value={0}>Automatic · about 2× viewport</option>
              <option value={50}>50 commits</option>
              <option value={100}>100 commits</option>
              <option value={200}>200 commits</option>
              <option value={500}>500 commits</option>
            </select>
          </label>
        </section>

        <section class="settings-section settings-preset-section">
          <div class="settings-section-heading">
            <div>
              <h2>Commit presets</h2>
              <p>Edit the buttons shown immediately above the commit table. Use <code>$me</code> for the current Git user, <code>last:3d</code> for a relative date, or <code>2026. 7. 19.</code> for a calendar date.</p>
            </div>
            <div class="settings-project-actions">
              <button type="button" on:click={onResetPresets}>Reset defaults</button>
              <button class="primary" type="button" on:click={addPreset}>＋ Add preset</button>
            </div>
          </div>

          <div class="settings-preset-list" aria-label="Commit filter presets">
            {#each presets as preset (preset.id)}
              <article class="settings-preset-card">
                <header>
                  <input value={preset.label} on:input={(event) => updatePresetLabel(preset.id, (event.currentTarget as HTMLInputElement).value)} aria-label="Preset name" placeholder="Preset name" />
                  <button type="button" on:click={() => removePreset(preset.id)} aria-label={`Delete ${preset.label || 'preset'}`}>Delete</button>
                </header>
                <div class="settings-preset-rules">
                  {#each preset.rules as rule (rule.id)}
                    <div class="settings-preset-rule">
                      <select value={rule.action} on:change={(event) => updatePresetRule(preset.id, rule.id, 'action', (event.currentTarget as HTMLSelectElement).value as CommitFilterAction)} aria-label="Preset filter action">
                        <option value="highlight">Highlight</option>
                        <option value="hide">Hide</option>
                        <option value="show">Show</option>
                      </select>
                      <select value={rule.field} on:change={(event) => updatePresetRule(preset.id, rule.id, 'field', (event.currentTarget as HTMLSelectElement).value as CommitFilterField)} aria-label="Preset filter field">
                        <option value="branch">Branch</option>
                        <option value="author">Author</option>
                        <option value="message">Message</option>
                        <option value="file">Changed file</option>
                        <option value="date">Date</option>
                      </select>
                      <input value={rule.pattern} on:input={(event) => updatePresetRule(preset.id, rule.id, 'pattern', (event.currentTarget as HTMLInputElement).value)} aria-label="Preset filter pattern" placeholder={rule.field === 'date' ? 'last:3d or 2026. 7. 19.' : rule.field === 'author' ? '$me or author' : 'pattern'} />
                      <button type="button" on:click={() => removePresetRule(preset.id, rule.id)} aria-label="Remove preset rule">×</button>
                    </div>
                  {/each}
                  <button class="settings-add-rule" type="button" on:click={() => addPresetRule(preset.id)}>＋ Add rule</button>
                </div>
              </article>
            {:else}
              <div class="settings-project-empty">No commit presets. Add one or reset the defaults.</div>
            {/each}
          </div>
        </section>

        <section class="settings-section settings-remote-section">
          <div class="settings-section-heading">
            <div>
              <h2>Remote badges</h2>
              <p>Match a remote URL by substring and choose the embedded icon shown in the All branches commit view.</p>
            </div>
            <div class="settings-project-actions">
              <button class="primary" type="button" on:click={addRemoteBadgeRule}>＋ Add mapping</button>
            </div>
          </div>

          {#if remotes.length}
            <div class="settings-remote-detected" aria-label="Detected remotes">
              {#each remotes as remote (remote.name)}
                {@const preview = resolveRefBadge(`${remote.name}/branch`, [remote], remoteBadgeRules)}
                <span title={remote.url}><RemoteBadgeIcon name={preview.icon} size={17} /><strong>{remote.name}</strong><small>{remote.url}</small></span>
              {/each}
            </div>
          {/if}

          <div class="settings-remote-rules" aria-label="Remote badge mappings">
            {#each remoteBadgeRules as rule (rule.id)}
              {@const selectedIcon = normalizeRemoteBadgeIcon(rule.icon)}
              <div class="settings-remote-rule">
                <label>
                  <span>Remote URL contains</span>
                  <input value={rule.pattern} on:input={(event) => updateRemoteBadgeRule(rule.id, 'pattern', (event.currentTarget as HTMLInputElement).value)} placeholder="mymy.gitlab.internal" />
                </label>
                <label>
                  <span>Badge icon</span>
                  <div class="settings-remote-icon-control">
                    <RemoteBadgeIcon name={selectedIcon} size={18} />
                    <select value={selectedIcon} on:change={(event) => updateRemoteBadgeRule(rule.id, 'icon', (event.currentTarget as HTMLSelectElement).value)} aria-label={`Badge icon for ${rule.pattern || 'new mapping'}`}>
                      {#if !isEmbeddedRemoteBadgeIcon(selectedIcon)}<option value={selectedIcon}>Legacy custom icon</option>{/if}
                      {#each remoteBadgeIconOptions as option}<option value={option.id}>{option.label}</option>{/each}
                    </select>
                  </div>
                </label>
                <button type="button" on:click={() => removeRemoteBadgeRule(rule.id)} aria-label={`Delete remote badge mapping for ${rule.pattern || 'empty pattern'}`}>Delete</button>
              </div>
            {:else}
              <div class="settings-project-empty">No mappings. Unknown remotes use the generic remote icon.</div>
            {/each}
          </div>
        </section>

        <section class="settings-section settings-history-section">
          <div>
            <h2>Default IDE</h2>
            <p>Used by Open IDE on worktree cards and Inspector files.</p>
          </div>
          <label class="settings-field">
            <span>Application</span>
            <select bind:value={ide} on:change={changeIDE}>
              <option value="vscode">Visual Studio Code</option>
              <option value="cursor">Cursor</option>
              <option value="zed">Zed</option>
              <option value="idea">IntelliJ IDEA</option>
              <option value="xcode">Xcode</option>
            </select>
          </label>
        </section>

        <section class="settings-section settings-history-section">
          <div>
            <h2>Default Terminal</h2>
            <p>Used by Open Terminal in Inspector file actions.</p>
          </div>
          <label class="settings-field">
            <span>Application</span>
            <select bind:value={terminal} on:change={changeTerminal}>
              <option value="terminal">Terminal</option>
              <option value="iterm2">iTerm2</option>
              <option value="warp">Warp</option>
              <option value="ghostty">Ghostty</option>
              <option value="wezterm">WezTerm</option>
            </select>
          </label>
        </section>
      </div>
    </div>
  </div>
{/if}
