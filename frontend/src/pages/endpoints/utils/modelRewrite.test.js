import test from 'node:test';
import assert from 'node:assert/strict';

import {
  CC_MODEL_REWRITE_PATHS,
  createEmptyEndpointModelRewriteRule,
  parseEndpointModelRewriteSettings,
  serializeEndpointModelRewriteRules,
  summarizeEndpointModelRewriteRules
} from './modelRewrite.js';

test('parseEndpointModelRewriteSettings keeps model rewrite disabled by default', () => {
  assert.deepEqual(parseEndpointModelRewriteSettings(''), {
    enabled: false,
    rules: [createEmptyEndpointModelRewriteRule()]
  });
  assert.equal(parseEndpointModelRewriteSettings('{invalid').enabled, false);
});

test('parseEndpointModelRewriteSettings restores exact array and single-object rules', () => {
  const arraySettings = parseEndpointModelRewriteSettings(JSON.stringify([
    { paths: CC_MODEL_REWRITE_PATHS, match: 'exact', from: 'claude-sonnet-4-5', to: 'provider-sonnet' },
    { paths: CC_MODEL_REWRITE_PATHS, match: 'prefix', from: 'legacy-', to: 'legacy-target' }
  ]));

  assert.equal(arraySettings.enabled, true);
  assert.deepEqual(arraySettings.rules, [
    { source: 'claude-sonnet-4-5', target: 'provider-sonnet' }
  ]);

  const singleSettings = parseEndpointModelRewriteSettings(JSON.stringify({
    paths: CC_MODEL_REWRITE_PATHS,
    from: 'source-model',
    to: 'target-model'
  }));
  assert.deepEqual(singleSettings.rules, [{ source: 'source-model', target: 'target-model' }]);
});

test('serializeEndpointModelRewriteRules emits exact rules for both Claude paths', () => {
  const raw = serializeEndpointModelRewriteRules([
    { source: ' claude-sonnet-4-5 ', target: ' provider-sonnet ' },
    { source: '', target: 'ignored' }
  ]);
  const rules = JSON.parse(raw);

  assert.deepEqual(rules, [{
    paths: CC_MODEL_REWRITE_PATHS,
    match: 'exact',
    from: 'claude-sonnet-4-5',
    to: 'provider-sonnet'
  }]);
});

test('summarizeEndpointModelRewriteRules shows the first mapping and remaining count', () => {
  const raw = serializeEndpointModelRewriteRules([
    { source: 'claude-sonnet-4-5', target: 'provider-sonnet' },
    { source: 'claude-opus-4-1', target: 'provider-opus' }
  ]);

  assert.deepEqual(summarizeEndpointModelRewriteRules(raw), {
    count: 2,
    label: 'claude-sonnet-4-5 → provider-sonnet +1',
    title: 'claude-sonnet-4-5 → provider-sonnet\nclaude-opus-4-1 → provider-opus'
  });
  assert.equal(summarizeEndpointModelRewriteRules(''), null);
});
