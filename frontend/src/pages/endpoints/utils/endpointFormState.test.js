import test from 'node:test';
import assert from 'node:assert/strict';

import {
  buildEndpointFormPayload,
  createEndpointFormState,
  summarizeEndpointAuthentication,
  validateEndpointFormState
} from './endpointFormState.js';

test('editing endpoint never places stored secrets in form state', () => {
  const state = createEndpointFormState({
    name: 'claude-primary',
    url: 'https://api.example.com',
    tokenMasked: 'sk-••••',
    apiKeyMasked: 'key-••••',
    headers: { 'X-Tenant': 'tenant-a' }
  });

  assert.equal(state.token, '');
  assert.equal(state.apiKey, '');
  assert.equal(state.hasStoredToken, true);
  assert.equal(state.hasStoredApiKey, true);
  assert.deepEqual(state.headerRows, [{ name: 'X-Tenant', value: 'tenant-a' }]);
  assert.equal(summarizeEndpointAuthentication({ tokenMasked: 'x', apiKeyMasked: 'y', headers: { A: 'b' } }), 'Token + API Key + 1 Header');
});

test('secret removal is explicit and headers are serialized structurally', () => {
  const payload = buildEndpointFormPayload({
    ...createEndpointFormState(),
    name: 'claude-primary',
    url: 'https://api.example.com/v1',
    clearToken: true,
    headerRows: [{ name: ' X-Tenant ', value: ' tenant-a ' }, { name: '', value: '' }]
  });

  assert.equal(payload.clearToken, true);
  assert.equal(payload.token, '');
  assert.deepEqual(payload.headers, { 'X-Tenant': 'tenant-a' });
  assert.equal('channel' in payload, false);
  assert.equal('group' in payload, false);
});

test('URL and header validation reject malformed structured input', () => {
  assert.deepEqual(validateEndpointFormState({ name: '', url: 'javascript:alert(1)', headerRows: [] }), {
    name: '请输入端点名称',
    url: '请输入不含账号密码的 HTTP(S) URL'
  });
  assert.equal(validateEndpointFormState({
    name: 'ep',
    url: 'https://user:pass@example.com',
    headerRows: [{ name: 'X-Test', value: '1' }, { name: 'x-test', value: '2' }]
  }).headers, 'Header 名称不能重复');
});
