import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './AccountInventoryFilters.jsx');

test('top inventory toolbar keeps selected-count copy out of the header row so filters do not get squeezed into a wrap', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.doesNotMatch(source, /已选 \{selectedCount\}/);
  assert.match(source, /\{selectedCount\} 个已选/);
});
