import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './AccountFormDialog.jsx');

test('AccountFormDialog supports 0.01 multiplier precision for api_key accounts', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(
    source,
    /step="0\.01"/,
    'expected multiplier inputs to use step="0.01" so values like 0.05 are not rejected by native form validation'
  );
});
