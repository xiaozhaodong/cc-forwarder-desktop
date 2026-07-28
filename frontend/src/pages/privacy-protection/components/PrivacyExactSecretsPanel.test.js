import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const panelPath = path.resolve(__dirname, './PrivacyExactSecretsPanel.jsx');
const dialogsPath = path.resolve(__dirname, './PrivacyExactSecretActionDialogs.jsx');

test('exact secret destructive actions use in-app dialogs instead of WebView native prompts', async () => {
  const [panelSource, dialogsSource] = await Promise.all([
    readFile(panelPath, 'utf8'),
    readFile(dialogsPath, 'utf8')
  ]);

  assert.doesNotMatch(panelSource, /window\.(confirm|prompt)/);
  assert.match(panelSource, /<DeleteExactSecretDialog/);
  assert.match(panelSource, /<ClearExactSecretsDialog/);
  assert.match(dialogsSource, /确认删除本地敏感值/);
  assert.match(dialogsSource, /confirmText\.trim\(\) === CLEAR_CONFIRM_TEXT/);
});

test('exact secret list presents mask length without exposing hash metadata', async () => {
  const panelSource = await readFile(panelPath, 'utf8');

  assert.match(panelSource, /secret\.value_length\} 字符/);
  assert.doesNotMatch(panelSource, /hash \{(?:secret|candidate)\.value_hash_short\}/);
  assert.doesNotMatch(panelSource, /len \{(?:secret|candidate)\.value_length\}/);
});
