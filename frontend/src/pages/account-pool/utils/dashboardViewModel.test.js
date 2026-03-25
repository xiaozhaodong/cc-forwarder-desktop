import test from 'node:test';
import assert from 'node:assert/strict';

import {
  buildAccountPoolDashboardModel,
  DEFAULT_SAVED_VIEWS
} from './dashboardViewModel.js';

test('buildAccountPoolDashboardModel returns full inventory rows and mapped scheduler fields without pre-filtering', () => {
  const model = buildAccountPoolDashboardModel({
    accounts: [
      {
        id: 1,
        account_name: 'main-oauth',
        enabled: true,
        provider_type: 'chatgpt_refresh_token',
        priority: 10,
        state: 'active',
        quota_status: 'ok',
        plan_type: 'plus',
        quota_5h_used_percent: 40,
        quota_5h_reset_at: '2026-03-21T12:00:00Z',
        quota_weekly_used_percent: 12,
        quota_weekly_reset_at: '2026-03-24T00:00:00Z',
        quota_refreshed_at: '2026-03-21T10:05:00Z',
        last_success_at: '2026-03-21T10:00:00Z'
      },
      {
        id: 2,
        account_name: 'main-broken',
        enabled: true,
        provider_type: 'chatgpt_refresh_token',
        priority: 10,
        state: 'disabled_auth',
        quota_status: 'auth_invalid',
        plan_type: 'team',
        fail_count: 2,
        last_error: 'refresh token expired',
        quota_refreshed_at: '2026-03-21T08:00:00Z'
      },
      {
        id: 3,
        account_name: 'backup-key',
        enabled: true,
        provider_type: 'api_key',
        priority: 20,
        is_active_selection: true,
        state: 'active',
        quota_status: 'ok',
        plan_type: 'enterprise'
      },
      {
        id: 4,
        account_name: 'cold-free',
        enabled: true,
        provider_type: 'chatgpt_refresh_token',
        priority: 30,
        state: 'active',
        quota_status: 'pending',
        plan_type: 'free',
        quota_weekly_used_percent: 88,
        quota_weekly_reset_at: '2026-03-29T00:00:00Z',
        quota_refreshed_at: '2026-03-10T09:00:00Z'
      }
    ],
    latestScheduleSnapshot: {
      hasSnapshot: true,
      selected_account_name: 'backup-key',
      selected_tier_label: '备组',
      selected_priority: 20,
      degraded_to_lower_priority: true,
      final_outcome: 'success',
      updated_at: '2026-03-21T10:08:00Z'
    },
    searchTerm: 'main',
    filters: {
      auth: 'all',
      plan: 'all',
      group: 'all',
      status: 'all',
      risk: 'all',
      sort: 'risk_desc',
      savedView: 'all'
    }
  });

  assert.deepEqual(Object.keys(model).sort(), ['inventory', 'scheduler']);
  assert.equal(model.scheduler.groups[0].label, '主组');
  assert.equal(model.scheduler.groups[1].label, '备组');
  assert.equal(model.scheduler.groups[2].label, '冷备');
  assert.equal(model.scheduler.summary.degraded, true);
  assert.equal(model.scheduler.groups[1].accounts[0].isActive, true);
  assert.equal(model.scheduler.groups[1].accounts[0].name, 'backup-key');
  assert.deepEqual(
    model.scheduler.groups[0].actions.map((item) => item.key),
    ['swap-down', 'reorder', 'set-active']
  );
  assert.deepEqual(
    model.scheduler.groups[1].actions.map((item) => item.key),
    ['swap-up', 'swap-down', 'reorder', 'set-active']
  );
  assert.equal(model.inventory.rows.length, 4);
  assert.ok(model.inventory.rows.some((item) => item.name === 'main-broken'));
  assert.equal(model.inventory.rows.find((item) => item.name === 'main-broken')?.lastSuccessText, '-');
  assert.match(
    model.inventory.rows.find((item) => item.name === 'main-oauth')?.lastSuccessText || '',
    /2026\/3\/21/
  );
  assert.deepEqual(
    model.inventory.savedViews.map((item) => item.key),
    DEFAULT_SAVED_VIEWS.map((item) => item.key)
  );
  assert.equal(model.inventory.rows.find((item) => item.name === 'main-broken')?.detail.lastErrorText, 'refresh token expired');
  assert.match(model.inventory.rows.find((item) => item.name === 'main-broken')?.detail.nextResetText || '', /未设置|重置|暂无/);
  assert.match(model.inventory.rows.find((item) => item.name === 'main-oauth')?.detail.quota5hResetText || '', /2026\/3\/21/);
  assert.match(model.inventory.rows.find((item) => item.name === 'main-oauth')?.detail.quota7dResetText || '', /2026\/3\/24/);
  assert.match(model.inventory.rows.find((item) => item.name === 'main-broken')?.detail.healthLabel || '', /异常|待观察|正常/);
  assert.match(model.inventory.rows.find((item) => item.name === 'main-broken')?.detail.groupOrderLabel || '', /当前活跃账号|组内候选/);
  assert.match(model.inventory.rows.find((item) => item.name === 'main-broken')?.detail.routingNote || '', /先按组别.*主组.*备组.*冷备/);
  assert.doesNotMatch(model.inventory.rows.find((item) => item.name === 'main-broken')?.detail.routingNote || '', /相同 priority 视为同一调度层/);
});

test('buildAccountPoolDashboardModel leaves saved views to runtime state while keeping batch action defaults', () => {
  const model = buildAccountPoolDashboardModel({
    accounts: [
      {
        id: 1,
        account_name: 'primary-auth-broken',
        enabled: true,
        provider_type: 'chatgpt_refresh_token',
        priority: 10,
        state: 'disabled_auth',
        quota_status: 'auth_invalid'
      },
      {
        id: 2,
        account_name: 'backup-oauth',
        enabled: true,
        provider_type: 'chatgpt_refresh_token',
        priority: 20,
        state: 'active',
        quota_status: 'ok'
      },
      {
        id: 3,
        account_name: 'cold-free',
        enabled: true,
        provider_type: 'chatgpt_refresh_token',
        priority: 30,
        state: 'active',
        quota_status: 'pending',
        plan_type: 'free'
      }
    ],
    latestScheduleSnapshot: null,
    searchTerm: '',
    filters: {
      auth: 'all',
      plan: 'all',
      group: 'all',
      status: 'all',
      risk: 'all',
      sort: 'risk_desc',
      savedView: 'cold-free'
    },
    selectedIds: [3]
  });

  assert.equal(model.inventory.rows.length, 3);
  assert.equal(model.inventory.rows.find((item) => item.id === 3)?.id, 3);
  assert.deepEqual(
    model.inventory.batchActions.map((item) => item.key),
    ['test', 'refresh-profile', 'toggle-enabled', 'move-backup', 'move-cold-standby']
  );
  assert.equal(model.inventory.selection.selectedCount, 1);
  assert.equal(model.inventory.selection.singleSelectedAccountId, 3);
});

test('buildAccountPoolDashboardModel keeps api_key accounts filterable under the prepaid plan bucket', () => {
  const model = buildAccountPoolDashboardModel({
    accounts: [
      {
        id: 11,
        account_name: 'prepaid-key',
        enabled: true,
        provider_type: 'api_key',
        priority: 20,
        state: 'active',
        quota_status: 'ok'
      }
    ],
    filters: {
      auth: 'all',
      plan: 'prepaid',
      group: 'all',
      status: 'all',
      risk: 'all',
      sort: 'risk_desc',
      savedView: 'all'
    }
  });

  assert.deepEqual(
    model.inventory.filters.planOptions.map((item) => item.value),
    ['all', 'prepaid']
  );
  assert.equal(model.inventory.rows.length, 1);
  assert.equal(model.inventory.rows[0].planKey, 'prepaid');
  assert.equal(model.inventory.rows[0].planLabel, 'Prepaid');
});

test('buildAccountPoolDashboardModel emits state tones that the reviewed components can render directly', () => {
  const model = buildAccountPoolDashboardModel({
    accounts: [
      { id: 1, account_name: 'active-a', enabled: true, provider_type: 'api_key', priority: 10, state: 'active', quota_status: 'ok' },
      { id: 2, account_name: 'cooldown-a', enabled: true, provider_type: 'api_key', priority: 20, state: 'cooldown', quota_status: 'ok' },
      { id: 3, account_name: 'auth-broken', enabled: true, provider_type: 'chatgpt_refresh_token', priority: 30, state: 'disabled_auth', quota_status: 'auth_invalid' },
      { id: 4, account_name: 'disabled-a', enabled: false, provider_type: 'api_key', priority: 40, state: 'active', quota_status: 'ok' }
    ],
    filters: {
      auth: 'all',
      plan: 'all',
      group: 'all',
      status: 'all',
      risk: 'all',
      sort: 'group_asc',
      savedView: 'all'
    }
  });

  const toneByName = new Map(model.inventory.rows.map((row) => [row.name, row.stateTone]));
  assert.equal(toneByName.get('active-a'), 'emerald');
  assert.equal(toneByName.get('cooldown-a'), 'amber');
  assert.equal(toneByName.get('auth-broken'), 'rose');
  assert.equal(toneByName.get('disabled-a'), 'slate');
});

test('buildAccountPoolDashboardModel keeps low-quota backup accounts counted as available while they remain schedulable', () => {
  const model = buildAccountPoolDashboardModel({
    accounts: [
      {
        id: 21,
        account_name: 'backup-low-quota',
        enabled: true,
        provider_type: 'chatgpt_refresh_token',
        group_key: 'backup',
        priority: 10,
        state: 'active',
        quota_status: 'ok',
        quota_weekly_used_percent: 82
      },
      {
        id: 22,
        account_name: 'backup-healthy',
        enabled: true,
        provider_type: 'chatgpt_refresh_token',
        group_key: 'backup',
        priority: 20,
        state: 'active',
        quota_status: 'ok',
        quota_weekly_used_percent: 3
      }
    ]
  });

  const backupGroup = model.scheduler.groups.find((group) => group.key === 'backup');
  assert.ok(backupGroup, 'expected backup group to be present');
  assert.equal(backupGroup.healthSummary, '2/2 可用');

  const lowQuotaAccount = backupGroup.accounts.find((account) => account.name === 'backup-low-quota');
  assert.equal(lowQuotaAccount?.riskLevel, 'P2');
  assert.equal(lowQuotaAccount?.isAvailable, true);
});
