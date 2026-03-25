import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

import { mapEndpointRecord, normalizeUpstreamAccount } from './wailsApi.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './wailsApi.js');

test('mapEndpointRecord preserves cacheCreationCostMultiplier1h from detail payload', () => {
  const result = mapEndpointRecord({
    id: 1,
    name: 'ep-cache-1h',
    cache_creation_cost_multiplier_1h: 1.75
  });

  assert.equal(result.cacheCreationCostMultiplier1h, 1.75);
});

test('normalizeUpstreamAccount keeps mirrored aliases in sync for mixed-case upstream payloads', () => {
  const result = normalizeUpstreamAccount({
    ID: '12',
    ProviderType: 'api_key',
    accountName: 'primary-key',
    isGroupPreferred: true,
    credentialRaw: 'sk-live',
    baseURL: 'https://example.com',
    costMultiplier: '1.5',
    input_cost_multiplier: '1.2',
    outputCostMultiplier: '2.4',
    cache_creation_cost_multiplier: '0',
    cacheCreationCostMultiplier1h: '3.5',
    cache_read_cost_multiplier: '0.8',
    groupKey: 'primary',
    Priority: '10',
    Enabled: true,
    State: 'active',
    cooldownUntil: '2026-03-25T00:00:00Z',
    failCount: 2,
    lastSuccessAt: '2026-03-24T12:00:00Z',
    lastError: 'none',
    planType: 'plus',
    ChatGPTAccountID: 'acct-1',
    chatgptUserId: 'user-2',
    OrganizationID: 'org-3',
    quota5hUsedPercent: '12.5',
    quota_weekly_used_percent: '22.5',
    quota5hResetAt: '2026-03-25T05:00:00Z',
    quota_weekly_reset_at: '2026-03-27T05:00:00Z',
    quotaStatus: 'ok',
    quotaRefreshedAt: '2026-03-24T13:00:00Z',
    Fingerprint: 'fp-1',
    createdAt: '2026-03-20T00:00:00Z',
    UpdatedAt: '2026-03-24T13:05:00Z'
  });

  assert.equal(result.id, 12);
  assert.equal(result.provider_type, 'api_key');
  assert.equal(result.providerType, 'api_key');
  assert.equal(result.account_name, 'primary-key');
  assert.equal(result.accountName, 'primary-key');
  assert.equal(result.is_group_preferred, true);
  assert.equal(result.isGroupPreferred, true);
  assert.equal(result.cost_multiplier, 1.5);
  assert.equal(result.costMultiplier, 1.5);
  assert.equal(result.input_cost_multiplier, 1.2);
  assert.equal(result.inputCostMultiplier, 1.2);
  assert.equal(result.output_cost_multiplier, 2.4);
  assert.equal(result.outputCostMultiplier, 2.4);
  assert.equal(result.cache_creation_cost_multiplier, 1);
  assert.equal(result.cacheCreationCostMultiplier, 1);
  assert.equal(result.cache_creation_cost_multiplier_1h, 3.5);
  assert.equal(result.cacheCreationCostMultiplier1h, 3.5);
  assert.equal(result.cache_read_cost_multiplier, 0.8);
  assert.equal(result.cacheReadCostMultiplier, 0.8);
  assert.equal(result.group_key, 'primary');
  assert.equal(result.groupKey, 'primary');
  assert.equal(result.priority, 10);
  assert.equal(result.enabled, true);
  assert.equal(result.state, 'active');
  assert.equal(result.cooldown_until, '2026-03-25T00:00:00Z');
  assert.equal(result.cooldownUntil, '2026-03-25T00:00:00Z');
  assert.equal(result.fail_count, 2);
  assert.equal(result.failCount, 2);
  assert.equal(result.plan_type, 'plus');
  assert.equal(result.planType, 'plus');
  assert.equal(result.chatgpt_account_id, 'acct-1');
  assert.equal(result.chatgptAccountId, 'acct-1');
  assert.equal(result.chatgpt_user_id, 'user-2');
  assert.equal(result.chatgptUserId, 'user-2');
  assert.equal(result.organization_id, 'org-3');
  assert.equal(result.organizationId, 'org-3');
  assert.equal(result.quota_5h_used_percent, 12.5);
  assert.equal(result.quota5hUsedPercent, 12.5);
  assert.equal(result.quota_weekly_used_percent, 22.5);
  assert.equal(result.quotaWeeklyUsedPercent, 22.5);
  assert.equal(result.quota_5h_reset_at, '2026-03-25T05:00:00Z');
  assert.equal(result.quota5hResetAt, '2026-03-25T05:00:00Z');
  assert.equal(result.quota_weekly_reset_at, '2026-03-27T05:00:00Z');
  assert.equal(result.quotaWeeklyResetAt, '2026-03-27T05:00:00Z');
  assert.equal(result.quota_status, 'ok');
  assert.equal(result.quotaStatus, 'ok');
  assert.equal(result.quota_refreshed_at, '2026-03-24T13:00:00Z');
  assert.equal(result.quotaRefreshedAt, '2026-03-24T13:00:00Z');
  assert.equal(result.fingerprint, 'fp-1');
  assert.equal(result.created_at, '2026-03-20T00:00:00Z');
  assert.equal(result.createdAt, '2026-03-20T00:00:00Z');
  assert.equal(result.updated_at, '2026-03-24T13:05:00Z');
  assert.equal(result.updatedAt, '2026-03-24T13:05:00Z');
});

test('normalizeUpstreamAccount uses alias helpers instead of repeating inline field chains', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(source, /const pickAccountValue = \(account,\s*keys,\s*fallback = ''\)/);
  assert.match(source, /const mirrorAliasedField = \(target,\s*snakeKey,\s*camelKey,\s*value\)/);
  assert.doesNotMatch(source, /provider_type:\s*account\.provider_type \|\| account\.providerType \|\| account\.ProviderType/);
});
