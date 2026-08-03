import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const sourcePath = path.resolve(__dirname, './index.jsx');
const autoRefreshPath = path.resolve(__dirname, './hooks/useAutoRefresh.js');
const autoRefreshControlPath = path.resolve(__dirname, './components/AutoRefreshControl.jsx');

test('requests page switches accounts by pinning the selected account instead of promoting its tier', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(source, /pinUpstreamAccountSelection/);
  assert.match(source, /已固定使用|固定账号|固定到/);
  assert.doesNotMatch(source, /moveUpstreamAccountToTier\(accountId,\s*'primary'\)/);
  assert.doesNotMatch(source, /已经是主组账号/);
});

test('requests page surfaces upstream account load failures to users', async () => {
  const source = await readFile(sourcePath, 'utf8');

  assert.match(source, /fetchUpstreamAccounts\(\)\.catch\(\(err\)\s*=>\s*\{/);
  assert.match(source, /fetchUpstreamAccounts\(\)\.catch\(\(err\)\s*=>\s*\{[^}]*showNotice\('error',/);
});

test('requests page uses Wails request events instead of permanent polling', async () => {
  const [source, hookSource, controlSource] = await Promise.all([
    readFile(sourcePath, 'utf8'),
    readFile(autoRefreshPath, 'utf8'),
    readFile(autoRefreshControlPath, 'utf8')
  ]);

  assert.match(source, /useAutoRefresh\(refreshRequestData\)/);
  assert.match(source, /const loadDataIdRef = useRef\(0\)/);
  assert.match(source, /loadDataIdRef\.current !== loadId/);
  assert.doesNotMatch(source, /refreshRequestData\(true\)/);
  assert.match(hookSource, /subscribeToEvent\('request:update'/);
  assert.match(hookSource, /EVENT_MAX_WAIT_MS = 1000/);
  assert.match(hookSource, /FAILURE_THRESHOLD = 3/);
  assert.match(hookSource, /createRealtimeRefreshScheduler/);
  assert.match(hookSource, /mode !== REALTIME_REFRESH_MODE\.FALLBACK/);
  assert.match(hookSource, /FALLBACK_INTERVAL_SECONDS = 30/);
  assert.doesNotMatch(hookSource, /refreshInterval/);
  assert.match(controlSource, /label: '实时'/);
  assert.match(controlSource, /降级 \$\{fallbackInterval\}s/);
  assert.match(controlSource, /连续刷新失败/);
});
