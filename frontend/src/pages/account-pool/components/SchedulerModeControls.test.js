import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));

const readSource = async (relativePath) => {
  const sourcePath = path.resolve(__dirname, relativePath);
  return readFile(sourcePath, 'utf8');
};

test('scheduler drawer and group board expose manual/auto mode controls', async () => {
  const [drawerSource, groupCardSource, indexSource] = await Promise.all([
    readSource('./SchedulerDrawer.jsx'),
    readSource('./GroupBoardCard.jsx'),
    readSource('../index.jsx')
  ]);

  assert.match(drawerSource, /启用编排/);
  assert.match(drawerSource, /手动模式|Auto 模式|当前为 Auto 模式|当前为手动模式/);
  assert.match(drawerSource, /全局手动|最近命中/);
  assert.match(drawerSource, /onEnableAutoSelection/);
  assert.match(drawerSource, /onPinAccountSelection/);

  assert.match(groupCardSource, /设为本组首选|本组首选/);
  assert.match(groupCardSource, /设为全局手动|全局手动/);
  assert.match(groupCardSource, /<Pin size=\{13\}/);
  assert.match(groupCardSource, /onPinAccountSelection/);

  assert.match(indexSource, /handleSchedulerPinAccount/);
  assert.match(indexSource, /handleSchedulerEnableAutoSelection/);
  assert.match(indexSource, /SchedulerDrawer[\s\S]*onPinAccountSelection=/);
  assert.match(indexSource, /SchedulerDrawer[\s\S]*onEnableAutoSelection=/);
});
