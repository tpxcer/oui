import type { Status } from '@/models/status';

export const STATUS_CARD_HISTORY_LIMIT = 60;
export const STATUS_CARD_INITIAL_POINTS = 24;

export interface StatusCardHistoryPoint {
  index: number;
  cpu: number;
  up: number;
  down: number;
  mem: number;
  swap: number;
  disk: number;
}

function nonNegative(value: number): number {
  return Number.isFinite(value) ? Math.max(0, value) : 0;
}

function percentage(value: number): number {
  return Math.min(100, nonNegative(value));
}

export function statusCardHistoryPoint(status: Status, index: number): StatusCardHistoryPoint {
  return {
    index,
    cpu: percentage(status.cpu.percent),
    up: nonNegative(status.netIO.up),
    down: nonNegative(status.netIO.down),
    mem: percentage(status.mem.percent),
    swap: percentage(status.swap.percent),
    disk: percentage(status.disk.percent),
  };
}

export function seedStatusCardHistory(
  status: Status,
  count = STATUS_CARD_INITIAL_POINTS,
): StatusCardHistoryPoint[] {
  const safeCount = Math.max(1, count);
  return Array.from({ length: safeCount }, (_, index) => statusCardHistoryPoint(status, index));
}

export function appendStatusCardHistory(
  history: StatusCardHistoryPoint[],
  status: Status,
  limit = STATUS_CARD_HISTORY_LIMIT,
): StatusCardHistoryPoint[] {
  const nextIndex = (history.at(-1)?.index ?? -1) + 1;
  return [...history, statusCardHistoryPoint(status, nextIndex)].slice(-Math.max(1, limit));
}
