export type InboundTagRemarks = Record<string, string>;

export function formatInboundTagLabel(tag: string, remarks?: InboundTagRemarks): string {
  const remark = (remarks?.[tag] || '').trim();
  if (!remark) return tag;
  return `${tag}（${remark}）`;
}

export function buildInboundTagOptions(tags: string[], remarks?: InboundTagRemarks) {
  const seen = new Set<string>();
  const out: Array<{ value: string; label: string }> = [];
  for (const tag of tags || []) {
    if (!tag || seen.has(tag)) continue;
    seen.add(tag);
    out.push({ value: tag, label: formatInboundTagLabel(tag, remarks) });
  }
  return out;
}
