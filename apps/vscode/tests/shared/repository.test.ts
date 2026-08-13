import assert from 'node:assert/strict'
import * as path from 'node:path'
import test from 'node:test'

import {
  isCommitObjectID,
  isSafeRepositoryRelativePath,
  repositoryRelativePath,
} from '../../src/shared/repository.js'

test('selection identifiers stay inside the discovered repository', () => {
  assert.equal(isCommitObjectID('a'.repeat(40)), true)
  assert.equal(isCommitObjectID('HEAD'), false)
  assert.equal(isSafeRepositoryRelativePath('src/main.go'), true)
  assert.equal(isSafeRepositoryRelativePath('../outside.go'), false)
  assert.equal(isSafeRepositoryRelativePath('.git/config'), false)
  assert.equal(isSafeRepositoryRelativePath('.GIT/config'), false)
  assert.equal(isSafeRepositoryRelativePath('src\\main.go'), false)
  assert.equal(isSafeRepositoryRelativePath('src/line\nbreak.go'), false)
})

test('repositoryRelativePath returns portable paths and rejects escapes', () => {
  const root = path.resolve(path.join('fixture', 'repository'))
  assert.equal(repositoryRelativePath(root, path.join(root, 'src', 'main.go')), 'src/main.go')
  assert.equal(repositoryRelativePath(root, root), undefined)
  assert.equal(repositoryRelativePath(root, path.resolve(root, '..', 'outside.go')), undefined)
})
