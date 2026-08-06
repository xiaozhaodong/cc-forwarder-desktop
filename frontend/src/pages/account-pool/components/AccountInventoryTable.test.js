import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const tableSourcePath = path.resolve(__dirname, './AccountInventoryTable.jsx');
const stylesSourcePath = path.resolve(__dirname, './inventoryBadgeStyles.js');

test('AccountInventoryTable keeps badge style constants in the shared inventoryBadgeStyles module', async () => {
  const source = await readFile(tableSourcePath, 'utf8');

  assert.equal(
    (source.match(/const UNKNOWN_LABELS =/g) || []).length,
    0,
    'expected UNKNOWN_LABELS to live in inventoryBadgeStyles.js, not in AccountInventoryTable.jsx'
  );
  assert.match(source, /from '\.\/inventoryBadgeStyles\.js'/);
});

test('inventoryBadgeStyles declares UNKNOWN_LABELS only once', async () => {
  const source = await readFile(stylesSourcePath, 'utf8');

  assert.equal(
    (source.match(/const UNKNOWN_LABELS =/g) || []).length,
    1,
    'expected inventoryBadgeStyles helper constants to avoid duplicate UNKNOWN_LABELS declarations'
  );
});
