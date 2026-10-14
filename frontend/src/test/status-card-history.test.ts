import { describe, expect, it } from 'vitest';

import { Status } from '@/models/status';
import {
  appendStatusCardHistory,
  resourceMeterTone,
  resourceSparklinePath,
  seedStatusCardHistory,
  statusCardHistoryPoint,
} from '@/pages/index/status-card-history';

function statusFixture() {
  return new Status({
    cpu: 25,
    mem: { current: 50, total: 100 },
    swap: { current: 10, total: 100 },
    disk: { current: 75, total: 100 },
    netIO: { up: 1024, down: 2048 },
  });
}

describe('status card history', () => {
  it('maps server status to a dashboard history point', () => {
    expect(statusCardHistoryPoint(statusFixture(), 3)).toEqual({
      index: 3,
      cpu: 25,
      up: 1024,
      down: 2048,
      mem: 50,
      swap: 10,
      disk: 75,
    });
  });

  it('seeds a non-empty chart and keeps only the requested number of samples', () => {
    const status = statusFixture();
    const seeded = seedStatusCardHistory(status, 3);
    expect(seeded).toHaveLength(3);

    const appended = appendStatusCardHistory(seeded, status, 3);
    expect(appended).toHaveLength(3);
    expect(appended.map((point) => point.index)).toEqual([1, 2, 3]);
  });

  it('switches resource meters to warning and error tones at 80% and 90%', () => {
    expect(resourceMeterTone(79.99, '#13c2c2')).toBe('#13c2c2');
    expect(resourceMeterTone(80, '#13c2c2')).toBe('var(--ant-color-warning)');
    expect(resourceMeterTone(90, '#13c2c2')).toBe('var(--ant-color-error)');
  });

  it('draws resource sparklines across the full width with clamped values', () => {
    expect(resourceSparklinePath([50])).toBe('');
    expect(resourceSparklinePath([0, 100, 150])).toBe('M0.00,96.00 L50.00,4.00 L100.00,4.00');
  });

  it('clamps invalid and percentage values before charting', () => {
    const status = new Status({
      cpu: 120,
      mem: { current: -1, total: 100 },
      swap: { current: 101, total: 100 },
      disk: { current: 1, total: 0 },
      netIO: { up: -10, down: Number.NaN },
    });

    expect(statusCardHistoryPoint(status, 0)).toMatchObject({
      cpu: 100,
      up: 0,
      down: 0,
      mem: 0,
      swap: 100,
      disk: 0,
    });
  });
});
