import { Alert, Progress, Tag } from 'antd';

import { SizeFormatter } from '@/utils';
import type { ServerInfo } from '@/models/status';
import { percent, unixDate, usageColor } from './server-info';
import './ServerInfo.css';

export function InfoField({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="preview-field">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

// Provider plan details shared by the dashboard and the info preview page.
// IP-like values carry `ip-value` so a parent can blur them.
export function ServerPlanDetails({ info }: { info: ServerInfo }) {
  const hasPlanTraffic = info.planMonthlyData > 0 || info.dataCounter > 0;
  const planPercent = percent(info.dataCounter, info.planMonthlyData);
  const nodeText = [info.nodeAlias, info.nodeLocation].filter(Boolean).join(' / ');
  const planResources = [
    info.planRam ? `RAM ${SizeFormatter.sizeFormat(info.planRam)}` : '',
    info.planSwap ? `Swap ${SizeFormatter.sizeFormat(info.planSwap)}` : '',
    info.planDisk ? `Disk ${SizeFormatter.sizeFormat(info.planDisk)}` : '',
  ].filter(Boolean);
  const ptrText = Object.entries(info.ptr || {})
    .map(([ip, ptr]) => `${ip} → ${ptr}`)
    .join('；');
  const hasProviderDetails = hasPlanTraffic || !!(
    nodeText || info.plan || planResources.length || info.ipAddresses?.length || ptrText || info.rdnsApiAvailable
  );

  return (
    <>
      {hasPlanTraffic && (
        <div className="plan-meter">
          <div className="plan-meter-head">
            <span>本月流量</span>
            <strong>
              {SizeFormatter.sizeFormat(info.dataCounter)}
              {' / '}
              {info.planMonthlyData ? SizeFormatter.sizeFormat(info.planMonthlyData) : '不限'}
            </strong>
          </div>
          {info.planMonthlyData > 0 && (
            <Progress percent={planPercent} strokeColor={usageColor(planPercent)} />
          )}
          <div className="plan-meter-foot">下次重置：{unixDate(info.dataNextReset)}</div>
        </div>
      )}
      {hasProviderDetails ? (
        <dl className="preview-fields">
          {nodeText && <InfoField label="节点">{nodeText}</InfoField>}
          {info.plan && <InfoField label="套餐">{info.plan}</InfoField>}
          {planResources.length > 0 && (
            <InfoField label="套餐资源">
              {planResources.map((item) => <Tag key={item}>{item}</Tag>)}
            </InfoField>
          )}
          {info.ipAddresses?.length > 0 && (
            <InfoField label="服务商 IP"><span className="ip-value">{info.ipAddresses.join('，')}</span></InfoField>
          )}
          {(ptrText || info.rdnsApiAvailable) && (
            <InfoField label="PTR"><span className="ip-value">{ptrText || '可用'}</span></InfoField>
          )}
        </dl>
      ) : (
        <div className="preview-empty">
          未获取到服务器商套餐信息，可在“面板设置 → 服务器商”中配置接口。
        </div>
      )}
      {info.error && <Alert className="preview-alert" type="warning" showIcon title={info.error} />}
    </>
  );
}
