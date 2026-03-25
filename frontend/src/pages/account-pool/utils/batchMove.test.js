import test from 'node:test';
import assert from 'node:assert/strict';

import { partitionRowsByTargetTier } from './batchMove.js';

test('partitionRowsByTargetTier skips rows already in the target tier', () => {
  const rows = [
    { id: 1, groupKey: 'primary' },
    { id: 2, groupKey: 'backup' },
    { id: 3, groupKey: 'cold-standby' },
    { id: 4, groupKey: 'cold' }
  ];

  assert.deepEqual(
    partitionRowsByTargetTier(rows, 'primary'),
    {
      eligibleRows: [{ id: 2, groupKey: 'backup' }, { id: 3, groupKey: 'cold-standby' }, { id: 4, groupKey: 'cold' }],
      skippedRows: [{ id: 1, groupKey: 'primary' }]
    }
  );

  assert.deepEqual(
    partitionRowsByTargetTier(rows, 'cold'),
    {
      eligibleRows: [{ id: 1, groupKey: 'primary' }, { id: 2, groupKey: 'backup' }],
      skippedRows: [{ id: 3, groupKey: 'cold-standby' }, { id: 4, groupKey: 'cold' }]
    }
  );
});
