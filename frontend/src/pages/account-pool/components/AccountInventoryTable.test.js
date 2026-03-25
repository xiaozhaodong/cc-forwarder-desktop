import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './AccountInventoryTable.jsx');

test('AccountInventoryTable declares UNKNOWN_LABELS only once', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.equal(
    (source.match(/const UNKNOWN_LABELS =/g) || []).length,
    1,
    'expected AccountInventoryTable helper constants to avoid duplicate UNKNOWN_LABELS declarations'
  );
});
