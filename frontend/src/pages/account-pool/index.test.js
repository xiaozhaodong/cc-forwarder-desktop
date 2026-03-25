import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './index.jsx');

test('AccountPoolPage wires inventory workspace, scheduler drawer, and header actions together', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(source, /AccountInventorySection/);
  assert.match(source, /SchedulerDrawer/);
  assert.match(source, /buildAccountPoolDashboardModel/);
  assert.match(source, /openCreateAccount/);
  assert.match(source, /openEditAccount/);
  assert.match(source, /PageHeader[\s\S]*onCreate=/);
  assert.match(source, /PageHeader[\s\S]*onRefresh=/);
  assert.match(source, /PageHeader[\s\S]*onOpenScheduler=/);
  assert.match(source, /AccountDetailsDrawer[\s\S]*onEditAccount=/);
  assert.match(source, /SchedulerDrawer[\s\S]*busyKey=/);
  assert.match(source, /AccountInventorySection[\s\S]*inventory=\{dashboardModel\.inventory\}[\s\S]*busyKey=\{busyKey\}/);
  assert.match(source, /AccountInventorySection[\s\S]*externalViewRequest=/);
  assert.match(source, /SchedulerDrawer[\s\S]*onViewInInventory=/);
  assert.match(source, /setGroupActiveAccount/);
  assert.doesNotMatch(source, /handleMoveAccountToTier\(account,\s*'primary'\)/);
  assert.doesNotMatch(source, /handleMoveAccountToTier\(account,\s*'backup'\)/);
  assert.doesNotMatch(source, /label: '概览'/);
  assert.doesNotMatch(source, /label: '异常中心'/);
  assert.doesNotMatch(source, /AccountPoolOverviewSection/);
  assert.doesNotMatch(source, /AccountPoolAnomalySection/);
});
