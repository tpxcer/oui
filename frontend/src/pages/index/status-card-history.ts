import type { Status } from '@/models/status';

export const STATUS_CARD_HISTORY_LIMIT = 60;
export const STATUS_CARD_INITIAL_POINTS = 1;

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

export function resourceMeterTone(percent: number, color: string): string {
  if (percent >= 90) return 'var(--ant-color-error)';
  if (percent >= 80) return 'var(--ant-color-warning)';
  return color;
}

// Path in a 0-100 viewBox; the y range is inset so 0% and 100% lines stay visible.
export function resourceSparklinePath(values: number[]): string {
  if (values.length < 2) return '';
  const step = 100 / (values.length - 1);
  return values
    .map((value, index) => {
      const y = 96 - percentage(value) * 0.92;
      return `${index === 0 ? 'M' : 'L'}${(index * step).toFixed(2)},${y.toFixed(2)}`;
    })
    .join(' ');
}

export function appendStatusCardHistory(
  history: StatusCardHistoryPoint[],
  status: Status,
  limit = STATUS_CARD_HISTORY_LIMIT,
): StatusCardHistoryPoint[] {
  const nextIndex = (history.at(-1)?.index ?? -1) + 1;
  return [...history, statusCardHistoryPoint(status, nextIndex)].slice(-Math.max(1, limit));
}
