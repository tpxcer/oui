import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Badge,
  Button,
  Card,
  ConfigProvider,
  Input,
  Layout,
  Progress,
  Spin,
  Tag,
  message,
} from 'antd';
import {
  ArrowDownOutlined,
  ArrowUpOutlined,
  CloudServerOutlined,
  DashboardOutlined,
  EnvironmentOutlined,
  ReloadOutlined,
  SwapOutlined,
} from '@ant-design/icons';

import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { HttpUtil, SizeFormatter, TimeFormatter } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import AppSidebar from '@/components/AppSidebar';
import type { NodeGeoLocation } from '@/models/status';
import './ApiDocsPage.css';

const UPLOAD_COLOR = '#1677ff';
const DOWNLOAD_COLOR = '#52c41a';

const XRAY_STATES: Record<string, { key: string; badge: 'success' | 'warning' | 'error' }> = {
  running: { key: 'pages.index.xrayStatusRunning', badge: 'success' },
  stop: { key: 'pages.index.xrayStatusStop', badge: 'warning' },
  error: { key: 'pages.index.xrayStatusError', badge: 'error' },
};

function percent(current: number, total: number) {
  if (!total) return 0;
  return Math.min(100, Math.max(0, Number(((current / total) * 100).toFixed(1))));
}

function usageColor(value: number) {
  if (value >= 90) return 'var(--ant-color-error)';
  if (value >= 80) return 'var(--ant-color-warning)';
  return 'var(--ant-color-primary)';
}

function geoAddress(geo?: Partial<NodeGeoLocation> | null) {
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

function unixDate(ts?: number) {
  if (!ts) return '-';
  return new Date(ts * 1000).toLocaleString();
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="preview-field">
      <dt>{label}</dt>
      <dd>{children}</dd>
    </div>
  );
}

function Metric({ label, value, dot }: { label: string; value: React.ReactNode; dot?: string }) {
  return (
    <div className="preview-metric">
      <span>{dot && <i className="preview-dot" style={{ background: dot }} />}{label}</span>
      <strong>{value}</strong>
    </div>
  );
}

export default function ApiDocsPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { status, fetched, refresh } = useStatusQuery();
  const [traceIp, setTraceIp] = useState('');
  const [traceGeo, setTraceGeo] = useState<Partial<NodeGeoLocation> | null>(null);
  const [traceLoading, setTraceLoading] = useState(false);
  const [messageApi, contextHolder] = message.useMessage();

  const publicIPv4 = String(status.publicIP.ipv4 || '').replace(/^0$/, '');

  useEffect(() => {
    if (!traceIp && publicIPv4 && publicIPv4 !== 'N/A') {
      setTraceIp(publicIPv4);
      setTraceGeo(status.serverInfo.geo);
    }
  }, [publicIPv4, status.serverInfo.geo, traceIp]);

  const pageClass = useMemo(() => {
    const classes = ['api-docs-page'];
    if (isDark) classes.push('is-dark');
    if (isUltra) classes.push('is-ultra');
    return classes.join(' ');
  }, [isDark, isUltra]);

  const lookupGeo = async (ip = traceIp) => {
    const value = ip.trim();
    if (!value) return;
    setTraceLoading(true);
    try {
      const msg = await HttpUtil.get(`/panel/api/server/geoIp/${encodeURIComponent(value)}`, undefined, { silent: true });
      if (msg?.success) {
        setTraceGeo(msg.obj as NodeGeoLocation);
      } else {
        messageApi.error(msg?.msg || 'Lookup failed');
      }
    } finally {
      setTraceLoading(false);
    }
  };

  const info = status.serverInfo;
  const xrayState = XRAY_STATES[status.xray.state];
  const xrayVersion = status.xray.version && status.xray.version !== 'Unknown' ? status.xray.version : '';

  const traceAddress = geoAddress(traceGeo);
  const traceCoords = traceGeo?.latitude && traceGeo?.longitude
    ? `${traceGeo.latitude.toFixed(5)}, ${traceGeo.longitude.toFixed(5)}`
    : '';
  const hasTraceResult = !!(traceAddress || traceGeo?.detail || traceCoords);

  const trafficTotal = status.netTraffic.sent + status.netTraffic.recv;
  const sentShare = percent(status.netTraffic.sent, trafficTotal);

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
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        {contextHolder}
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <div className="preview-shell">
              <Card className="preview-hero">
                <div className="preview-hero-top">
                  <div className="preview-hero-identity">
                    <span className="preview-hero-icon"><CloudServerOutlined /></span>
                    <div className="preview-hero-name">
                      <h1>{info.hostname || t('menu.apiDocs')}</h1>
                      <div className="preview-hero-meta">
                        {[info.provider, info.os, info.vmType].filter(Boolean).map((item) => (
                          <span key={item}>{item}</span>
                        ))}
                      </div>
                    </div>
                  </div>
                  <div className="preview-hero-actions">
                    <Badge
                      status={xrayState?.badge ?? 'default'}
                      text={`Xray ${t(xrayState?.key ?? 'pages.index.xrayStatusUnknown')}`}
                    />
                    {xrayVersion && <Tag>v{xrayVersion}</Tag>}
                    <Button icon={<ReloadOutlined />} onClick={refresh} loading={!fetched}>
                      {t('refresh')}
                    </Button>
                  </div>
                </div>
                <div className="preview-hero-metrics">
                  <Metric label="公网 IPv4" value={publicIPv4 || '-'} />
                  <Metric label="系统运行" value={TimeFormatter.formatSecond(status.uptime)} />
                  <Metric label="系统负载" value={status.loads.slice(0, 3).join(' / ')} />
                  <Metric label="连接数" value={`TCP ${status.tcpCount} · UDP ${status.udpCount}`} />
                </div>
              </Card>

              <div className="preview-grid">
                <Card
                  className="preview-card span-half"
                  title={<span className="preview-card-title"><EnvironmentOutlined />公网归属信息</span>}
                >
                  <Input.Search
                    value={traceIp}
                    onChange={(e) => setTraceIp(e.target.value)}
                    onSearch={lookupGeo}
                    enterButton="查询"
                    loading={traceLoading}
                    placeholder="IP 地址"
                  />
                  <Spin spinning={traceLoading}>
                    {hasTraceResult ? (
                      <div className="geo-result">
                        <div className="geo-place">{traceAddress || '未知归属地'}</div>
                        <dl className="preview-fields">
                          <Field label="IP">{traceGeo?.ip || traceIp || '-'}</Field>
                          {traceGeo?.detail && <Field label="运营商">{traceGeo.detail}</Field>}
                          {traceCoords && <Field label="坐标">{traceCoords}</Field>}
                        </dl>
                      </div>
                    ) : (
                      <div className="preview-empty">
                        {traceGeo?.error ? '查询失败或暂无归属信息' : '输入 IP 地址后查询归属地和运营商'}
                      </div>
                    )}
                  </Spin>
                </Card>

                <Card
                  className="preview-card span-half"
                  title={<span className="preview-card-title"><CloudServerOutlined />服务器套餐</span>}
                >
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
                      {nodeText && <Field label="节点">{nodeText}</Field>}
                      {info.plan && <Field label="套餐">{info.plan}</Field>}
                      {planResources.length > 0 && (
                        <Field label="套餐资源">
                          {planResources.map((item) => <Tag key={item}>{item}</Tag>)}
                        </Field>
                      )}
                      {info.ipAddresses?.length > 0 && <Field label="服务商 IP">{info.ipAddresses.join('，')}</Field>}
                      {(ptrText || info.rdnsApiAvailable) && <Field label="PTR">{ptrText || '可用'}</Field>}
                    </dl>
                  ) : (
                    <div className="preview-empty">
                      未获取到服务器商套餐信息，可在“面板设置 → 服务器商”中配置接口。
                    </div>
                  )}
                  {info.error && <Alert className="preview-alert" type="warning" showIcon title={info.error} />}
                </Card>

                <Card
                  className="preview-card span-wide"
                  title={<span className="preview-card-title"><SwapOutlined />流量统计</span>}
                >
                  <div className="traffic-rates">
                    <div className="traffic-rate">
                      <span>实时上行</span>
                      <strong style={{ color: UPLOAD_COLOR }}>
                        <ArrowUpOutlined />
                        {SizeFormatter.sizeFormat(status.netIO.up)}/s
                      </strong>
                    </div>
                    <div className="traffic-rate">
                      <span>实时下行</span>
                      <strong style={{ color: DOWNLOAD_COLOR }}>
                        <ArrowDownOutlined />
                        {SizeFormatter.sizeFormat(status.netIO.down)}/s
                      </strong>
                    </div>
                  </div>
                  <div className="traffic-split" aria-label={`已发送 ${sentShare}%`}>
                    <i style={{ width: `${trafficTotal ? sentShare : 0}%`, background: UPLOAD_COLOR }} />
                    <i style={{ width: `${trafficTotal ? 100 - sentShare : 0}%`, background: DOWNLOAD_COLOR }} />
                  </div>
                  <div className="preview-metrics-row">
                    <Metric label="累计发送" dot={UPLOAD_COLOR} value={SizeFormatter.sizeFormat(status.netTraffic.sent)} />
                    <Metric label="累计接收" dot={DOWNLOAD_COLOR} value={SizeFormatter.sizeFormat(status.netTraffic.recv)} />
                    <Metric label="合计" value={SizeFormatter.sizeFormat(trafficTotal)} />
                  </div>
                </Card>

                <Card
                  className="preview-card span-narrow"
                  title={<span className="preview-card-title"><DashboardOutlined />面板进程</span>}
                >
                  <dl className="preview-fields is-stats">
                    <Field label="面板运行">{TimeFormatter.formatSecond(status.appStats.uptime || status.appUptime)}</Field>
                    <Field label="面板内存">{SizeFormatter.sizeFormat(status.appStats.mem)}</Field>
                    <Field label="线程数">{status.appStats.threads || '-'}</Field>
                  </dl>
                </Card>
              </div>
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
