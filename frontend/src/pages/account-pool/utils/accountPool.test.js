import test from 'node:test';
import assert from 'node:assert/strict';

import { buildManualSwitchSuccessMessage } from './accountPool.js';

test('buildManualSwitchSuccessMessage avoids promising immediate effect', () => {
  const message = buildManualSwitchSuccessMessage('primary-a', 'primary');

  assert.ok(message.includes('当前可调度状态优先使用'));
  assert.ok(message.includes('恢复后会自动回切'));
  assert.ok(!message.includes('立即生效'));
});

test('buildManualSwitchSuccessMessage can include requests path hint', () => {
  const message = buildManualSwitchSuccessMessage('primary-a', 'primary', { includeRequestPath: true });

  assert.ok(message.includes('/v1/responses'));
  assert.ok(message.includes('当前可调度状态优先使用'));
});
