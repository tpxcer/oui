/// <reference types="vite/client" />
import { describe, expect, it } from 'vitest';

import { SecuritySettingsSchema } from '@/schemas/protocols';
import {
  createDefaultRealityStreamSettings,
  RealityStreamSettingsSchema,
} from '@/schemas/protocols/security/reality';

const securityFixtures = import.meta.glob<unknown>(
  './golden/fixtures/security/*.json',
  { eager: true, import: 'default' },
);

function fixtureName(path: string): string {
  const file = path.split('/').pop() ?? path;
  return file.replace(/\.json$/, '');
}

describe('SecuritySettingsSchema fixtures', () => {
  const entries = Object.entries(securityFixtures).sort(([a], [b]) => a.localeCompare(b));
  expect(entries.length, 'expected at least one fixture under golden/fixtures/security').toBeGreaterThan(0);

  for (const [path, raw] of entries) {
    it(`parses ${fixtureName(path)} byte-stably`, () => {
      const parsed = SecuritySettingsSchema.parse(raw);
      expect(parsed).toMatchSnapshot();
    });
  }
});

describe('Reality creation defaults', () => {
  it('defaults new inbounds to minimum client version 0.0.0', () => {
    expect(createDefaultRealityStreamSettings().minClientVer).toBe('0.0.0');
  });

  it('preserves an empty minimum version in existing configurations', () => {
    expect(RealityStreamSettingsSchema.parse({ minClientVer: '' }).minClientVer).toBe('');
  });
});
