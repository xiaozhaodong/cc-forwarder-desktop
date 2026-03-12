import test from 'node:test';
import assert from 'node:assert/strict';

import { resolveDisplayedActiveAccount } from './accountSwitcherState.js';

test('resolveDisplayedActiveAccount prefers runtime active selection over primary tier first account', () => {
  const accounts = [
    { id: 1, account_name: 'primary-a', priority: 10, enabled: true, state: 'active' },
    { id: 2, account_name: 'primary-b', priority: 10, enabled: true, state: 'active', is_active_selection: true }
  ];

  const resolved = resolveDisplayedActiveAccount({
    accounts,
    recentSelectedAccountId: null
  });

  assert.equal(resolved?.id, 2);
});

test('resolveDisplayedActiveAccount falls back to recent selected snapshot when runtime selection is missing', () => {
  const accounts = [
    { id: 1, account_name: 'primary-a', priority: 10, enabled: true, state: 'active' },
    { id: 2, account_name: 'primary-b', priority: 10, enabled: true, state: 'active' }
  ];

  const resolved = resolveDisplayedActiveAccount({
    accounts,
    recentSelectedAccountId: 2
  });

  assert.equal(resolved?.id, 2);
});
