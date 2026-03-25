import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const loadSource = async (relativePath) => {
  const sourcePath = path.resolve(__dirname, relativePath);
  return readFile(sourcePath, 'utf8');
};

test('inventory workspace threads busy state into the table and drawer action surfaces', async () => {
  const [sectionSource, tableSource, drawerSource] = await Promise.all([
    loadSource('./AccountInventorySection.jsx'),
    loadSource('./AccountInventoryTable.jsx'),
    loadSource('./AccountDetailsDrawer.jsx')
  ]);

  assert.match(sectionSource, /busyKey\s*=\s*''/, 'expected inventory section props to accept busyKey');
  assert.match(sectionSource, /AccountInventoryTable[\s\S]*?busyKey=\{busyKey\}/, 'expected inventory section to forward busyKey into the table');
  assert.match(sectionSource, /AccountDetailsDrawer[\s\S]*?busyKey=\{busyKey\}/, 'expected inline drawer rendering to forward busyKey');
  assert.match(tableSource, /const actionBusy = Boolean\(busyKey\)/, 'expected table actions to derive a busy flag');
  assert.match(tableSource, /disabled=\{actionBusy\}/, 'expected row-level quick actions to disable while busy');
  assert.match(drawerSource, /const actionBusy = Boolean\(busyKey\)/, 'expected drawer actions to derive a busy flag');
  assert.match(drawerSource, /disabled=\{actionBusy\}/, 'expected drawer actions to disable while busy');
});
