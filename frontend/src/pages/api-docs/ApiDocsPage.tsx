import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Badge,
  Button,
  Card,
  ConfigProvider,
  Input,
  Layout,
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
import { InfoField, ServerPlanDetails } from '@/components/ServerInfo';
import { geoAddress, percent } from '@/components/server-info';
import type { NodeGeoLocation } from '@/models/status';
import './ApiDocsPage.css';

const UPLOAD_COLOR = '#1677ff';
const DOWNLOAD_COLOR = '#52c41a';

const XRAY_STATES: Record<string, { key: string; badge: 'success' | 'warning' | 'error' }> = {
  running: { key: 'pages.index.xrayStatusRunning', badge: 'success' },
  stop: { key: 'pages.index.xrayStatusStop', badge: 'warning' },
  error: { key: 'pages.index.xrayStatusError', badge: 'error' },
};

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
        messageApi.error(msg?.msg || '查询失败');
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
                          <InfoField label="IP">{traceGeo?.ip || traceIp || '-'}</InfoField>
                          {traceGeo?.detail && <InfoField label="运营商">{traceGeo.detail}</InfoField>}
                          {traceCoords && <InfoField label="坐标">{traceCoords}</InfoField>}
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
                  <ServerPlanDetails info={info} />
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
                    <InfoField label="面板运行">{TimeFormatter.formatSecond(status.appStats.uptime || status.appUptime)}</InfoField>
                    <InfoField label="面板内存">{SizeFormatter.sizeFormat(status.appStats.mem)}</InfoField>
                    <InfoField label="线程数">{status.appStats.threads || '-'}</InfoField>
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
