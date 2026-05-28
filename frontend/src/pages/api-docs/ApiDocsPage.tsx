import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  ConfigProvider,
  Input,
  Layout,
  Progress,
  Spin,
  Statistic,
  Tag,
  message,
} from 'antd';
import {
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

function percent(current: number, total: number) {
  if (!total) return 0;
  return Math.min(100, Math.max(0, Number(((current / total) * 100).toFixed(1))));
}

function geoLine(geo?: Partial<NodeGeoLocation> | null) {
  if (!geo) return '-';
  const address = geoAddress(geo);
  const parts = [
    geo.ip ? `IP：${geo.ip}` : '',
    address ? `地址：${address}` : '',
    geo.latitude && geo.longitude ? `坐标：${geo.latitude.toFixed(5)}, ${geo.longitude.toFixed(5)}` : '',
    geo.source ? `来源：${geo.source}` : '',
    geo.error ? `错误：${geo.error}` : '',
  ].filter(Boolean);
  return parts.join('；') || '-';
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
  for (const p of [geo.country, geo.province, geo.city, geo.district, geo.detail, geo.location]) {
    addPart(p);
  }
  return parts.join('');
}

function unixDate(ts?: number) {
  if (!ts) return '-';
  return new Date(ts * 1000).toLocaleString();
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

  const trafficTotal = status.netTraffic.sent + status.netTraffic.recv;
  const providerTrafficPercent = percent(status.serverInfo.dataCounter, status.serverInfo.planMonthlyData);
  const ptrText = Object.entries(status.serverInfo.ptr || {})
    .map(([ip, ptr]) => `${ip} -> ${ptr}`)
    .join('；');

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className={pageClass}>
        {contextHolder}
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content className="content-area">
            <div className="preview-shell">
              <div className="preview-header">
                <div>
                  <h1>{t('menu.apiDocs')}</h1>
                  <div className="preview-subtitle">
                    <Tag color={status.xray.color}>{status.xray.state}</Tag>
                    <span>Xray {status.xray.version || '-'}</span>
                    <span>{status.serverInfo.hostname || '-'}</span>
                  </div>
                </div>
                <Button icon={<ReloadOutlined />} onClick={refresh} loading={!fetched}>
                  {t('refresh')}
                </Button>
              </div>

              <div className="preview-grid">
                <section className="preview-panel trace-panel">
                  <div className="panel-title">
                    <EnvironmentOutlined />
                    <span>VPN 溯源</span>
                  </div>
                  <div className="trace-search">
                    <Input.Search
                      value={traceIp}
                      onChange={(e) => setTraceIp(e.target.value)}
                      onSearch={lookupGeo}
                      enterButton="查询"
                      loading={traceLoading}
                      placeholder="IPv4"
                    />
                  </div>
                  <Spin spinning={traceLoading}>
                    <div className="trace-result">
                      <strong>{geoAddress(traceGeo) || traceIp || '-'}</strong>
                      <span>{geoLine(traceGeo)}</span>
                    </div>
                  </Spin>
                </section>

                <section className="preview-panel server-panel">
                  <div className="panel-title">
                    <CloudServerOutlined />
                    <span>服务器</span>
                  </div>
                  <div className="server-list">
                    <div><span>服务器商</span><strong>{status.serverInfo.provider || '-'}</strong></div>
                    <div><span>系统</span><strong>{status.serverInfo.os || '-'}</strong></div>
                    <div><span>虚拟化</span><strong>{status.serverInfo.vmType || '-'}</strong></div>
                    <div><span>节点</span><strong>{[status.serverInfo.nodeAlias, status.serverInfo.nodeLocation].filter(Boolean).join(' / ') || '-'}</strong></div>
                    <div><span>套餐</span><strong>{status.serverInfo.plan || '-'}</strong></div>
                    {(status.serverInfo.planMonthlyData > 0 || status.serverInfo.dataCounter > 0) && (
                      <div className="server-traffic-row">
                        <span>套餐流量</span>
                        <strong>
                          <span>{SizeFormatter.sizeFormat(status.serverInfo.dataCounter)} / {status.serverInfo.planMonthlyData ? SizeFormatter.sizeFormat(status.serverInfo.planMonthlyData) : '不限'}</span>
                          <Progress
                            percent={status.serverInfo.planMonthlyData ? providerTrafficPercent : 100}
                            showInfo={!!status.serverInfo.planMonthlyData}
                            strokeColor="#1677ff"
                          />
                        </strong>
                      </div>
                    )}
                    <div><span>下次重置</span><strong>{unixDate(status.serverInfo.dataNextReset)}</strong></div>
                    <div><span>套餐资源</span><strong>{[
                      status.serverInfo.planRam ? `RAM ${SizeFormatter.sizeFormat(status.serverInfo.planRam)}` : '',
                      status.serverInfo.planSwap ? `Swap ${SizeFormatter.sizeFormat(status.serverInfo.planSwap)}` : '',
                      status.serverInfo.planDisk ? `Disk ${SizeFormatter.sizeFormat(status.serverInfo.planDisk)}` : '',
                    ].filter(Boolean).join(' / ') || '-'}</strong></div>
                    <div><span>运行</span><strong>{TimeFormatter.formatSecond(status.uptime)}</strong></div>
                    <div><span>公网 IPv4</span><strong>{publicIPv4 || '-'}</strong></div>
                    <div><span>服务商 IP</span><strong>{status.serverInfo.ipAddresses?.length ? status.serverInfo.ipAddresses.join('，') : '-'}</strong></div>
                    <div><span>PTR</span><strong>{ptrText || (status.serverInfo.rdnsApiAvailable ? '可用' : '-')}</strong></div>
                    {status.serverInfo.error && <div><span>服务商错误</span><strong>{status.serverInfo.error}</strong></div>}
                  </div>
                </section>

                <section className="preview-panel usage-panel">
                  <div className="panel-title">
                    <SwapOutlined />
                    <span>流量使用情况</span>
                  </div>
                  <div className="traffic-strip">
                    <Statistic title="实时上行" value={SizeFormatter.sizeFormat(status.netIO.up)} />
                    <Statistic title="实时下行" value={SizeFormatter.sizeFormat(status.netIO.down)} />
                    <Statistic title="累计流量" value={SizeFormatter.sizeFormat(trafficTotal)} />
                  </div>
                  <div className="traffic-bars">
                    <div>
                      <span>已发送 {SizeFormatter.sizeFormat(status.netTraffic.sent)}</span>
                      <Progress percent={trafficTotal ? percent(status.netTraffic.sent, trafficTotal) : 0} showInfo={false} />
                    </div>
                    <div>
                      <span>已接收 {SizeFormatter.sizeFormat(status.netTraffic.recv)}</span>
                      <Progress percent={trafficTotal ? percent(status.netTraffic.recv, trafficTotal) : 0} showInfo={false} strokeColor="#22a06b" />
                    </div>
                  </div>
                </section>

                <section className="preview-panel overview-panel">
                  <div className="panel-title">
                    <DashboardOutlined />
                    <span>面板概览</span>
                  </div>
                  <div className="overview-grid">
                    <div><span>面板运行</span><strong>{TimeFormatter.formatSecond(status.appStats.uptime || status.appUptime)}</strong></div>
                    <div><span>面板内存</span><strong>{SizeFormatter.sizeFormat(status.appStats.mem)}</strong></div>
                    <div><span>线程数</span><strong>{status.appStats.threads || '-'}</strong></div>
                    <div><span>TCP 连接</span><strong>{status.tcpCount}</strong></div>
                    <div><span>UDP 连接</span><strong>{status.udpCount}</strong></div>
                    <div><span>系统负载</span><strong>{status.loads.slice(0, 3).join(' / ')}</strong></div>
                  </div>
                </section>
              </div>
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
