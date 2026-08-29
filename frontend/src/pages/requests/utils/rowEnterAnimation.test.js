import test from 'node:test';
import assert from 'node:assert/strict';

import {
  buildRowEnterAnimations,
  collectRequestIds,
  STAGGER_STEP_MS,
  MAX_STAGGER_STEPS,
  BURST_THRESHOLD
} from './rowEnterAnimation.js';

const rows = (...ids) => ids.map((requestId) => ({ requestId }));
const idsOf = (...ids) => new Set(ids);

test('只有新增的行拿到入场动画，已存在的行不重播', () => {
  const result = buildRowEnterAnimations(rows('new-1', 'old-a', 'old-b'), idsOf('old-a', 'old-b'), true);

  assert.equal(result.size, 1);
  assert.deepEqual(result.get('new-1'), { delayMs: 0, withCascade: true });
  assert.equal(result.get('old-a'), undefined);
});

test('多条同时到达时按顺序错峰', () => {
  const result = buildRowEnterAnimations(rows('n1', 'n2', 'n3', 'old'), idsOf('old'), true);

  assert.equal(result.get('n1').delayMs, 0);
  assert.equal(result.get('n2').delayMs, STAGGER_STEP_MS);
  assert.equal(result.get('n3').delayMs, STAGGER_STEP_MS * 2);
  assert.ok([...result.values()].every((entry) => entry.withCascade));
});

test('延迟累计封顶，避免整批入场拖太久', () => {
  const many = Array.from({ length: BURST_THRESHOLD }, (_, i) => `n${i}`);
  const result = buildRowEnterAnimations(rows(...many), idsOf('old'), true);

  const maxDelay = Math.max(...[...result.values()].map((entry) => entry.delayMs));
  assert.equal(maxDelay, MAX_STAGGER_STEPS * STAGGER_STEP_MS);
  // 超过档位上限的行共用同一个最大延迟，不会无限往后排
  assert.equal(result.get(`n${MAX_STAGGER_STEPS}`).delayMs, maxDelay);
  assert.equal(result.get(`n${BURST_THRESHOLD - 1}`).delayMs, maxDelay);
});

test('单批新增超过阈值时整批降级为无逐列接力', () => {
  const burst = Array.from({ length: BURST_THRESHOLD + 1 }, (_, i) => `n${i}`);
  const result = buildRowEnterAnimations(rows(...burst), idsOf('old'), true);

  assert.equal(result.size, BURST_THRESHOLD + 1);
  assert.ok([...result.values()].every((entry) => entry.withCascade === false),
    '整批共用同一个 withCascade，不能一半逐列一半整行');
});

test('恰好等于阈值时仍保留逐列接力', () => {
  const exact = Array.from({ length: BURST_THRESHOLD }, (_, i) => `n${i}`);
  const result = buildRowEnterAnimations(rows(...exact), idsOf('old'), true);

  assert.ok([...result.values()].every((entry) => entry.withCascade === true));
});

test('未启用 / 首次渲染 / 无新增 时都不产出动画', () => {
  assert.equal(buildRowEnterAnimations(rows('n1'), idsOf('old'), false).size, 0, '非实时刷新不播');
  assert.equal(buildRowEnterAnimations(rows('n1'), null, true).size, 0, '首次渲染无上一批可比');
  assert.equal(buildRowEnterAnimations(rows('a'), idsOf('a'), true).size, 0, '没有新增行');
  assert.equal(buildRowEnterAnimations(null, idsOf('a'), true).size, 0, '非数组输入');
});

test('缺少 requestId 的行被跳过，不影响其余行的序号', () => {
  const malformed = [{ requestId: 'n1' }, null, { }, { requestId: 'n2' }];
  const result = buildRowEnterAnimations(malformed, idsOf('old'), true);

  assert.equal(result.size, 2);
  assert.equal(result.get('n1').delayMs, 0);
  assert.equal(result.get('n2').delayMs, STAGGER_STEP_MS);
});

test('collectRequestIds 去重并跳过脏数据', () => {
  const ids = collectRequestIds([{ requestId: 'a' }, { requestId: 'a' }, null, { }, { requestId: 'b' }]);

  assert.deepEqual([...ids].sort(), ['a', 'b']);
  assert.equal(collectRequestIds(null).size, 0);
});
