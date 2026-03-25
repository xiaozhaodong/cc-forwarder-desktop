import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './AccountPoolSwitcher.jsx');

test('AccountPoolSwitcher uses explicit group semantics instead of legacy tier labels', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(source, /group_key|groupKey/);
  assert.match(source, /Auto\s*\/\s*按编排|按编排自动调度|切回 Auto/);
  assert.match(source, /bg-sky-400|bg-sky-500|bg-cyan-400|bg-cyan-500/);
  assert.doesNotMatch(source, /const activeIndicatorClass = isAutoMode \? 'bg-slate-400'/);
  assert.match(source, /组内顺序/);
  assert.doesNotMatch(source, /const activePriority/);
  assert.doesNotMatch(source, /顺序 \{activePriority\}/);
  assert.doesNotMatch(source, /buildManualFailoverTierSummary/);
  assert.doesNotMatch(source, /compareAccountsByManualPriority/);
  assert.doesNotMatch(source, /P\$\{priority\}/);
  assert.doesNotMatch(source, /第 \$\{index \+ 1\} 层/);
});
