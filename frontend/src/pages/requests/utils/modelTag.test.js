import test from 'node:test';
import assert from 'node:assert/strict';

import { getModelColorClasses } from './modelTag.js';

test('getModelColorClasses supports new model families used in request tracking', () => {
  assert.match(getModelColorClasses('glm-5.2'), /teal/);
  assert.match(getModelColorClasses('claude-fable-5'), /rose/);
  assert.match(getModelColorClasses('claude-sonnet-5'), /amber/);
  assert.match(getModelColorClasses(' Claude-Sonnet-5 '), /amber/);
});

test('getModelColorClasses keeps existing model color mappings', () => {
  assert.match(getModelColorClasses('claude-sonnet-4'), /orange/);
  assert.match(getModelColorClasses('claude-3-5-haiku'), /green/);
  assert.match(getModelColorClasses('claude-3-5-sonnet'), /blue/);
  assert.match(getModelColorClasses('claude-opus-4'), /purple/);
  assert.match(getModelColorClasses('gpt-5.4-mini'), /indigo/);
});

test('getModelColorClasses uses neutral color for missing or unknown models', () => {
  assert.match(getModelColorClasses('unknown'), /slate/);
  assert.match(getModelColorClasses('-'), /slate/);
  assert.match(getModelColorClasses(''), /slate/);
  assert.match(getModelColorClasses(null), /slate/);
});
