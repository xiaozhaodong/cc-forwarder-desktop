import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './AccountRow.jsx');

test('AccountRow no longer exposes legacy quick promote buttons', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.doesNotMatch(source, /设为主组/);
  assert.doesNotMatch(source, /设为备组/);
  assert.doesNotMatch(source, /当前主组/);
  assert.doesNotMatch(source, /当前备组/);
});

test('AccountRow matches busyKey by exact account suffix instead of substring includes', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.doesNotMatch(source, /busyKey\.includes\(String\(accountId\)\)/);
  assert.match(source, /endsWith\(`-\$\{String\(accountId\)\}`\)/);
});
