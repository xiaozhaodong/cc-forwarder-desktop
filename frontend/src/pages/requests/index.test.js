import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './index.jsx');

test('requests page switches accounts by pinning the selected account instead of promoting its tier', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(source, /pinUpstreamAccountSelection/);
  assert.match(source, /已固定使用|固定账号|固定到/);
  assert.doesNotMatch(source, /moveUpstreamAccountToTier\(accountId,\s*'primary'\)/);
  assert.doesNotMatch(source, /已经是主组账号/);
});

test('requests page surfaces upstream account load failures to users', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(source, /fetchUpstreamAccounts\(\)\.catch\(\(err\)\s*=>\s*\{/);
  assert.match(source, /fetchUpstreamAccounts\(\)\.catch\(\(err\)\s*=>\s*\{[^}]*showNotice\('error',/);
});
