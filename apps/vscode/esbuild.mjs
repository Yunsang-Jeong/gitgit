import { build } from 'esbuild'
import { rm } from 'node:fs/promises'

const shared = {
  bundle: true,
  logLevel: 'info',
  platform: 'node',
  sourcemap: true,
  target: 'node20',
}

await rm('dist-tests', { recursive: true, force: true })

await Promise.all([
  build({
    ...shared,
    entryPoints: ['src/extension.ts'],
    external: ['vscode'],
    outfile: 'dist/extension.js',
    format: 'cjs',
  }),
  build({
    ...shared,
    entryPoints: ['tests/**/*.test.ts'],
    outdir: 'dist-tests',
    outbase: '.',
    format: 'cjs',
    external: ['vscode'],
  }),
])
