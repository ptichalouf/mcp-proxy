import assert from 'node:assert/strict';
import test from 'node:test';
import { catalogEnvRequirements, missingRequiredEnv } from './catalogEnv.ts';
import type { CatalogItem } from '../types.ts';

const server: CatalogItem = {
  id: 'demo', name: 'Demo', description: 'Demo', category: 'development',
  transportType: 'stdio',
  defaultConfig: { command: 'uvx', args: ['demo'], env: { API_TOKEN: '', HOST: 'localhost' } },
  env: [
    { name: 'API_TOKEN', description: 'Token from a read-only account', required: true, isSecret: true },
    { name: 'HOST', description: 'Server hostname', required: false },
  ],
};

test('API env metadata is used instead of bare defaultConfig keys', () => {
  assert.deepEqual(catalogEnvRequirements(server), [
    { key: 'API_TOKEN', value: '', description: 'Token from a read-only account', required: true, isSecret: true },
    { key: 'HOST', value: 'localhost', description: 'Server hostname', required: false, isSecret: false },
  ]);
});

test('missing required values are rejected, including whitespace', () => {
  const requirements = catalogEnvRequirements(server);
  assert.deepEqual(missingRequiredEnv(requirements), ['API_TOKEN']);
  assert.deepEqual(missingRequiredEnv(requirements.map((r) => ({ ...r, value: '  ' }))), ['API_TOKEN']);
  assert.deepEqual(missingRequiredEnv(requirements.map((r) => ({ ...r, value: '${API_TOKEN}' }))), []);
  assert.deepEqual(missingRequiredEnv(requirements, { HOST: 'localhost' }), ['API_TOKEN']);
  assert.deepEqual(missingRequiredEnv(requirements, { API_TOKEN: 'ok', HOST: 'localhost' }), []);
});

test('blank custom entries and older envRequirements entries remain usable', () => {
  assert.deepEqual(catalogEnvRequirements(null), []);
  assert.deepEqual(catalogEnvRequirements({ ...server, env: undefined, envRequirements: [{ key: 'LEGACY', description: '', required: false }] }), [
    { key: 'LEGACY', value: '', description: '', required: false, isSecret: false },
  ]);
});
