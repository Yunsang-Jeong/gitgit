import assert from 'node:assert/strict'
import test from 'node:test'

import { DEFAULT_SEARCH_DRAFT } from '../../src/search/model.js'
import {
  contentSecurityPolicy,
  escapeHtml,
  parseWebviewMessage,
} from '../../src/search/webview.js'

test('Search Webview validates drafts and result selections', () => {
  assert.deepEqual(parseWebviewMessage({
    type: 'search',
    draft: { ...DEFAULT_SEARCH_DRAFT, expression: 'MSG: *cache*' },
  }), {
    type: 'search',
    draft: { ...DEFAULT_SEARCH_DRAFT, expression: 'MSG: *cache*' },
  })
  assert.deepEqual(parseWebviewMessage({
    type: 'selectCommitFile',
    commit: 'a'.repeat(40),
    path: 'src/main.go',
  }), {
    action: 'selectCommitFile',
    commit: 'a'.repeat(40),
    path: 'src/main.go',
  })
  assert.equal(parseWebviewMessage({ type: 'search', draft: { ...DEFAULT_SEARCH_DRAFT, engine: 'fixed' } }), undefined)
  assert.equal(parseWebviewMessage({ type: 'openFile', commit: 'bad', path: '../secret' }), undefined)
  assert.equal(parseWebviewMessage({ type: 'openFile', commit: 'a'.repeat(40), path: 'src\\secret.ts' }), undefined)
  assert.equal(parseWebviewMessage({ type: 'openFile', commit: 'a'.repeat(40), path: '.git/config' }), undefined)
  assert.equal(parseWebviewMessage({ type: 'openFile', commit: 'a'.repeat(40), path: `src/evil\0.ts` }), undefined)
  assert.equal(parseWebviewMessage({ type: 'unknown' }), undefined)
})

test('Search Webview escapes result and status text', () => {
  assert.equal(escapeHtml(`<script>alert("x")</script>&'`), '&lt;script&gt;alert(&quot;x&quot;)&lt;/script&gt;&amp;&#039;')
  assert.equal(
    contentSecurityPolicy('NONCE123456789012'),
    "default-src 'none'; style-src 'nonce-NONCE123456789012'; script-src 'nonce-NONCE123456789012';",
  )
  assert.throws(() => contentSecurityPolicy(`bad' unsafe-inline`), /nonce/)
})
