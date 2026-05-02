import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './index.jsx');

test('PricingForm supports 0.001 price precision', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(
    source,
    /const PRICE_INPUT_STEP = '0\.001'/,
    'expected pricing inputs to allow values like 3.125 without native step validation errors'
  );
});

test('PricingForm keeps cache price suggestions at 3 decimal places', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(source, /const CACHE_PRICE_DECIMALS = 3/);
  assert.match(source, /toFixed\(CACHE_PRICE_DECIMALS\)/);
  assert.doesNotMatch(source, /input \* 1\.25\)\.toFixed\(2\)/);
});
