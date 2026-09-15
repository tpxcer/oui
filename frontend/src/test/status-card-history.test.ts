import { describe, expect, it } from 'vitest';

import { Status } from '@/models/status';
import {
  appendStatusCardHistory,
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
  it('maps server status to the six-series combined resource chart', () => {
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
