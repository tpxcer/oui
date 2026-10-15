import type { NodeGeoLocation } from '@/models/status';

export function percent(current: number, total: number) {
  if (!total) return 0;
  return Math.min(100, Math.max(0, Number(((current / total) * 100).toFixed(1))));
}

export function usageColor(value: number) {
  if (value >= 90) return 'var(--ant-color-error)';
  if (value >= 80) return 'var(--ant-color-warning)';
  return 'var(--ant-color-primary)';
}

export function geoAddress(geo?: Partial<NodeGeoLocation> | null) {
  if (!geo) return '';
  const parts: string[] = [];
  const addPart = (value?: string) => {
    const part = (value || '').trim();
    if (!part) return;
    const current = parts.join('');
    if (parts.includes(part) || (current && current.includes(part))) return;
    if (current && part.includes(current)) {
      parts.splice(0, parts.length, part);
      return;
    }
    parts.push(part);
  };
  for (const p of [geo.country, geo.province, geo.city, geo.district]) {
    addPart(p);
  }
  const address = parts.join('');
  if (address) return address;
  const location = (geo.location || '').trim();
  const detail = (geo.detail || '').trim();
  if (location && detail && location.endsWith(detail)) {
    return location.slice(0, -detail.length).trim() || location;
  }
  return location;
}

export function unixDate(ts?: number) {
  if (!ts) return '-';
  return new Date(ts * 1000).toLocaleString();
}
