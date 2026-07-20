import type { Author, CommitFilterLogic, CommitFilterPreset, CommitFilterRule } from './types'

export const defaultFilterLogic: CommitFilterLogic = { show: 'and', hide: 'or' }

export function defaultFilterPresets(): CommitFilterPreset[] {
  return [
    {
      id: 'my-jobs',
      label: 'My Jobs',
      rules: [{ id: 'my-jobs-author', action: 'show', field: 'author', pattern: '$me' }],
    },
    {
      id: '3-days',
      label: '3 Days',
      rules: [{ id: '3-days-date', action: 'show', field: 'date', pattern: 'last:3d' }],
    },
  ]
}

export function cloneFilterPresets(presets: CommitFilterPreset[]): CommitFilterPreset[] {
  return presets.map((preset) => ({
    ...preset,
    rules: preset.rules.map((rule) => ({ ...rule })),
  }))
}

export function resolvePresetRules(presets: CommitFilterPreset[], activeIDs: string[], author: Author): CommitFilterRule[] {
  const active = new Set(activeIDs)
  const currentAuthor = author.name || author.email
  return presets
    .filter((preset) => active.has(preset.id))
    .flatMap((preset) => preset.rules.map((rule) => ({
      ...rule,
      id: `preset:${preset.id}:${rule.id}`,
      pattern: rule.field === 'author' && rule.pattern.trim().toLocaleLowerCase() === '$me'
        ? currentAuthor
        : rule.pattern,
    })))
    .filter((rule) => rule.pattern.trim().length > 0)
}

export function presetUnavailable(preset: CommitFilterPreset, author: Author): boolean {
  return preset.rules.some((rule) => rule.field === 'author' && rule.pattern.trim().toLocaleLowerCase() === '$me')
    && !(author.name || author.email)
}
