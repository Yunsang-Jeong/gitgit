import assert from 'node:assert/strict'
import test from 'node:test'

import { reviewLinkForCommit } from '../../src/editor/review-links.js'

test('GitHub merge messages link to the configured repository remote', () => {
  assert.deepEqual(
    reviewLinkForCommit(
      'Merge pull request #42 from acme/feature',
      2,
      ['https://github.com/acme/widgets'],
    ),
    { kind: 'pull-request', label: 'Open PR #42', url: 'https://github.com/acme/widgets/pull/42' },
  )
  assert.equal(
    reviewLinkForCommit('Merge pull request #42 from acme/feature', 1, ['https://github.com/acme/widgets']),
    undefined,
  )
  assert.equal(reviewLinkForCommit('fix: retain keyboard focus (#73)', 1, ['https://github.com/acme/widgets']), undefined)
})

test('GitLab merge messages require the repository path to match the configured remote', () => {
  const message = "Merge branch 'feature' into 'main'\n\nSee merge request platform/GitGit!19"
  assert.deepEqual(
    reviewLinkForCommit(message, 2, ['https://gitlab.com/platform/GitGit']),
    {
      kind: 'merge-request',
      label: 'Open MR !19',
      url: 'https://gitlab.com/platform/GitGit/-/merge_requests/19',
    },
  )
  assert.equal(reviewLinkForCommit(message, 2, ['https://gitlab.com/other/fork']), undefined)
  assert.equal(reviewLinkForCommit(message, 1, ['https://gitlab.com/platform/GitGit']), undefined)
  assert.equal(reviewLinkForCommit(
    `${message}\n\nQuoted text after the trailer`,
    2,
    ['https://gitlab.com/platform/GitGit'],
  ), undefined)
})

test('ordinary review references and unknown providers do not claim merge provenance', () => {
  for (const message of [
    'Reviewed in https://github.com/acme/widgets/pull/8.',
    'Reviewed in https://evil.example/acme/widgets/pull/8',
    String.raw`Reviewed in https://github.com\acme/widgets/pull/8`,
    'Reviewed in https://github.com/acme/x/../widgets/pull/8',
    'Reviewed in HTTPS://GITHUB.COM/acme/widgets/pull/8',
  ]) {
    assert.equal(reviewLinkForCommit(message, 1, ['https://github.com/acme/widgets']), undefined)
  }
  assert.equal(reviewLinkForCommit('fix: mention #8', 1, ['file:///tmp/repository']), undefined)
  assert.equal(reviewLinkForCommit('fix: mention #8', 1, ['https://token@example.test/acme/widgets']), undefined)
  assert.equal(
    reviewLinkForCommit('Merge pull request #8 from acme/feature', 2, ['https://code.example.test/acme/widgets']),
    undefined,
  )
  assert.equal(
    reviewLinkForCommit('See merge request acme/widgets!8', 2, ['https://code.example.test/acme/widgets']),
    undefined,
  )
})

test('ambiguous fork and upstream remotes do not guess a GitHub pull request', () => {
  assert.equal(reviewLinkForCommit('feat: ship it (#19)', 1, [
    'https://github.com/acme/widgets',
    'https://github.com/contributor/widgets',
  ]), undefined)
  assert.equal(
    reviewLinkForCommit('feat: ship it (#19)', 1, ['https://gitlab.example.test/acme/widgets']),
    undefined,
  )
  assert.equal(reviewLinkForCommit('fix: no review (#0)', 1, ['https://github.com/acme/widgets']), undefined)
  assert.equal(reviewLinkForCommit('fix: no review (#12345678901)', 1, ['https://github.com/acme/widgets']), undefined)
  assert.equal(
    reviewLinkForCommit(
      'Merge pull request #12 from acme/feature\n\nSee merge request acme/widgets!34',
      2,
      ['https://gitlab.example.test/acme/widgets'],
    ),
    undefined,
  )
  assert.equal(
    reviewLinkForCommit('See merge request acme/widgets!9', 2, ['https://github.com/acme/widgets']),
    undefined,
  )
  assert.equal(
    reviewLinkForCommit('Merge pull request #9 from acme/feature', 2, ['https://gitlab.com/acme/widgets']),
    undefined,
  )
})
