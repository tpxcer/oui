import { lazy, useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Button,
  Card,
  Col,
  ConfigProvider,
  Layout,
  message,
  Modal,
  Progress,
  Row,
  Space,
  Spin,
  Tag,
  Tooltip,
} from 'antd';
import {
  BarsOutlined,
  ControlOutlined,
  CloudServerOutlined,
  CloudDownloadOutlined,
  ArrowDownOutlined,
  AreaChartOutlined,
  LineChartOutlined,
  EyeOutlined,
  EyeInvisibleOutlined,
  CopyOutlined,
  SyncOutlined,
} from '@ant-design/icons';

import { HttpUtil, SizeFormatter, TimeFormatter, ClipboardManager, FileManager, PromiseUtil } from '@/utils';
import { useTheme } from '@/hooks/useTheme';
import { useStatusQuery } from '@/api/queries/useStatusQuery';
import { useMediaQuery } from '@/hooks/useMediaQuery';
import AppSidebar from '@/components/AppSidebar';
import LazyMount from '@/components/LazyMount';
import { setMessageInstance } from '@/utils/messageBus';
import type { PanelUpdateInfo } from '@/lib/panelUpdate';
import {
  clearPanelUpdateTarget,
  fetchFreshPanelShellVersion,
  getLivePanelStatus,
  getPanelUpdateInfoNoCache,
  reloadPanelPage,
  waitForUpdatedPanel,
} from '@/lib/panelUpdate';
import StatusCard from './StatusCard';
import XrayStatusCard from './XrayStatusCard';
const JsonEditor = lazy(() => import('@/components/JsonEditor'));
const LogModal = lazy(() => import('./LogModal'));
const BackupModal = lazy(() => import('./BackupModal'));
const SystemHistoryModal = lazy(() => import('./SystemHistoryModal'));
const XrayMetricsModal = lazy(() => import('./XrayMetricsModal'));
const XrayLogModal = lazy(() => import('./XrayLogModal'));
const VersionModal = lazy(() => import('./VersionModal'));
import './IndexPage.css';

function panelReleaseNotes(notes?: string) {
  const text = (notes || '').trim();
  if (!text) return null;
  return (
    <div className="panel-release-notes">
      <strong>本版本更新内容</strong>
      <pre>{text}</pre>
    </div>
  );
}

function panelUpdateContent(description: string, notes?: string) {
  return (
    <div className="panel-update-dialog-content">
      <p>{description}</p>
      {panelReleaseNotes(notes)}
    </div>
  );
}

function PanelDownloadIcon({ active = false }: { active?: boolean }) {
  return (
    <span className={`panel-download-icon${active ? ' is-active' : ''}`} aria-hidden="true">
      <ArrowDownOutlined className="panel-download-arrow" />
      <span className="panel-download-tray" />
    </span>
  );
}

export default function IndexPage() {
  const { t } = useTranslation();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const { status, fetched, refresh } = useStatusQuery();
  const { isMobile } = useMediaQuery();
  const [messageApi, messageContextHolder] = message.useMessage();
  const [modal, modalContextHolder] = Modal.useModal();
  useEffect(() => { setMessageInstance(messageApi); }, [messageApi]);

  const [ipLimitEnable, setIpLimitEnable] = useState(false);
  const [panelUpdateInfo, setPanelUpdateInfo] = useState<PanelUpdateInfo>({
    currentVersion: '',
    latestVersion: '',
    updateAvailable: false,
    releaseNotes: '',
    releaseUrl: '',
  });

  const basePath = window.X_UI_BASE_PATH || '';

  const [showIp, setShowIp] = useState(false);
  const [logsOpen, setLogsOpen] = useState(false);
  const [backupOpen, setBackupOpen] = useState(false);
  const [sysHistoryOpen, setSysHistoryOpen] = useState(false);
  const [xrayMetricsOpen, setXrayMetricsOpen] = useState(false);
  const [xrayLogsOpen, setXrayLogsOpen] = useState(false);
  const [versionOpen, setVersionOpen] = useState(false);
  const [configTextOpen, setConfigTextOpen] = useState(false);
  const [configText, setConfigText] = useState('');
  const [loading, setLoading] = useState(false);
  const [loadingTip, setLoadingTip] = useState(t('loading'));
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [updatingPanel, setUpdatingPanel] = useState(false);
  const [updateProgress, setUpdateProgress] = useState(0);

  useEffect(() => {
    HttpUtil.post<{ ipLimitEnable?: boolean }>('/panel/setting/defaultSettings').then((msg) => {
      if (msg?.success && msg.obj) setIpLimitEnable(!!msg.obj.ipLimitEnable);
    });
  }, []);

  const displayVersion = useMemo(
    () => panelUpdateInfo.currentVersion || window.X_UI_CUR_VER || '?',
    [panelUpdateInfo.currentVersion],
  );

  const setBusy = useCallback(
    ({ busy, tip }: { busy: boolean; tip?: string }) => {
      setLoading(busy);
      if (tip) setLoadingTip(tip);
    },
    [],
  );

  const stopXray = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/stopXrayService');
    await refresh();
  }, [refresh]);

  const restartXray = useCallback(async () => {
    await HttpUtil.post('/panel/api/server/restartXrayService');
    await refresh();
  }, [refresh]);

  const checkPanelUpdate = useCallback(async (options?: { silent?: boolean }) => {
    setCheckingUpdate(true);
    try {
      const msg = await getPanelUpdateInfoNoCache();
      if (msg?.success && msg.obj) {
        setPanelUpdateInfo(msg.obj);
        if (!options?.silent) {
          if (msg.obj.updateAvailable) {
            messageApi.info(msg.obj.latestVersion ? `发现新版本：${msg.obj.latestVersion}` : '发现新版本');
            modal.info({
              title: msg.obj.latestVersion ? `发现新版本：${msg.obj.latestVersion}` : '发现新版本',
              content: panelReleaseNotes(msg.obj.releaseNotes) || '本版本暂无更新说明。',
              okText: '知道了',
            });
          } else {
            messageApi.success('当前已是最新版');
          }
        }
      } else if (!options?.silent) {
        messageApi.error(msg?.msg || '版本检测失败，请检查服务器是否可以访问 GitHub');
      }
    } finally {
      setCheckingUpdate(false);
    }
  }, [messageApi, modal]);

  useEffect(() => {
    void checkPanelUpdate({ silent: true });
  }, [checkPanelUpdate]);

  const startPanelUpdate = useCallback(async () => {
    const info = panelUpdateInfo;
    if (!info.updateAvailable) {
      await checkPanelUpdate();
      return;
    }
    const runUpdate = async () => {
      setUpdatingPanel(true);
      setUpdateProgress(8);
      try {
        const result = await HttpUtil.post('/panel/api/server/updatePanel', undefined, { silent: true });
        if (!result?.success) {
          messageApi.error(result?.msg || '启动后台更新失败');
          setUpdatingPanel(false);
          return;
        }
        messageApi.success(info.latestVersion ? `已开始后台更新到 ${info.latestVersion}` : '已开始后台更新');
        const updated = info.latestVersion ? await waitForUpdatedPanel(info.latestVersion) : false;
        setUpdateProgress(100);
        if (updated) {
          await PromiseUtil.sleep(800);
          reloadPanelPage(info.latestVersion);
          return;
        }
        messageApi.info('后台更新仍在执行，请稍后手动刷新页面');
      } catch (error) {
        messageApi.error(error instanceof Error ? error.message : '启动后台更新失败');
      } finally {
        setUpdatingPanel(false);
      }
    };
    modal.confirm({
      title: t('pages.index.panelUpdateDialog'),
      content: panelUpdateContent(
        t('pages.index.panelUpdateDialogDesc').replace('#version#', info.latestVersion || '最新版'),
        info.releaseNotes,
      ),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: () => {
        void runUpdate();
      },
    });
  }, [checkPanelUpdate, messageApi, modal, panelUpdateInfo, t]);

  useEffect(() => {
    if (!updatingPanel) {
      setUpdateProgress(0);
      return undefined;
    }
    const timer = window.setInterval(() => {
      setUpdateProgress((value) => {
        if (value >= 95) return value;
        return Math.min(95, value + Math.max(1, Math.round((95 - value) * 0.12)));
      });
    }, 700);
    return () => window.clearInterval(timer);
  }, [updatingPanel]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      const target = new URLSearchParams(window.location.search).get('_ouiTarget');
      if (!target) return;

      const statusMsg = await getLivePanelStatus(3000);
      if (cancelled) return;
      if (statusMsg?.success && statusMsg.obj?.panelVersion === target) {
        clearPanelUpdateTarget();
        setPanelUpdateInfo((prev) => ({
          ...prev,
          currentVersion: target,
          latestVersion: target,
          updateAvailable: false,
        }));
        return;
      }

      const shellVersion = await fetchFreshPanelShellVersion();
      if (!cancelled && shellVersion === target) {
        clearPanelUpdateTarget();
        reloadPanelPage(target);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const panelUpdateButtonLabel = updatingPanel
    ? `更新中 ${Math.round(updateProgress)}%`
    : checkingUpdate
      ? '检测中'
      : panelUpdateInfo.updateAvailable
        ? '一键更新'
        : '检测更新';

  const handlePanelUpdateAction = useCallback(() => {
    if (checkingUpdate || updatingPanel) return;
    if (panelUpdateInfo.updateAvailable) {
      startPanelUpdate();
    } else {
      checkPanelUpdate();
    }
  }, [checkPanelUpdate, checkingUpdate, panelUpdateInfo.updateAvailable, startPanelUpdate, updatingPanel]);

  async function openConfig() {
    setLoading(true);
    try {
      const msg = await HttpUtil.get('/panel/api/server/getConfigJson');
      if (!msg?.success) return;
      setConfigText(JSON.stringify(msg.obj, null, 2));
      setConfigTextOpen(true);
    } finally {
      setLoading(false);
    }
  }

  async function copyConfig() {
    const ok = await ClipboardManager.copyText(configText || '');
    if (ok) messageApi.success(t('copied'));
  }

  function downloadConfig() {
    FileManager.downloadTextFile(configText, 'config.json');
  }

  const pageClass = `index-page ${isDark ? 'is-dark' : ''} ${isUltra ? 'is-ultra' : ''}`.trim();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      {messageContextHolder}
      {modalContextHolder}
      <Layout className={pageClass}>
        <AppSidebar />

        <Layout className="content-shell">
          <Layout.Content className="content-area">
            {updatingPanel && (
              <div className="index-update-lock" role="status" aria-live="polite">
                <div className="index-update-lock-card">
                  <PanelDownloadIcon active />
                  <strong>{panelUpdateInfo.latestVersion ? `正在更新到 ${panelUpdateInfo.latestVersion}` : '正在更新到最新版本'}</strong>
                  <Progress percent={Math.round(updateProgress)} showInfo={false} />
                  <small>{Math.round(updateProgress)}%</small>
                </div>
              </div>
            )}
            <Spin
              spinning={loading || !fetched}
              delay={200}
              description={loading ? loadingTip : t('loading')}
              size="large"
            >
              {!fetched ? (
                <div className="loading-spacer" />
              ) : (
                <Row gutter={[isMobile ? 8 : 16, 12]}>
                  <Col span={24}>
                    <StatusCard status={status} isMobile={isMobile} />
                  </Col>

                  <Col xs={24} lg={12}>
                    <XrayStatusCard
                      status={status}
                      isMobile={isMobile}
                      ipLimitEnable={ipLimitEnable}
                      onStopXray={stopXray}
                      onRestartXray={restartXray}
                      onOpenXrayLogs={() => setXrayLogsOpen(true)}
                      onOpenLogs={() => setLogsOpen(true)}
                      onOpenVersionSwitch={() => setVersionOpen(true)}
                    />
                  </Col>

                  <Col xs={24} lg={12}>
                    <Card
                      title={
                        <Space>
                          <span>OUI</span>
                          {displayVersion && (
                            <Tag color={panelUpdateInfo.updateAvailable ? 'orange' : 'green'}>
                              {panelUpdateInfo.updateAvailable
                                ? `最新 ${panelUpdateInfo.latestVersion || '最新版'}`
                                : displayVersion}
                            </Tag>
                          )}
                        </Space>
                      }
                      hoverable
                      actions={[
                        <Space
                          key="panel-update"
                          className={`action ${panelUpdateInfo.updateAvailable ? 'action-update' : ''}${checkingUpdate || updatingPanel ? ' action-disabled' : ''}`}
                          onClick={handlePanelUpdateAction}
                        >
                          {panelUpdateInfo.updateAvailable ? (
                            <PanelDownloadIcon active={updatingPanel} />
                          ) : (
                            <SyncOutlined spin={checkingUpdate} />
                          )}
                          {!isMobile && <span>{panelUpdateButtonLabel}</span>}
                        </Space>,
                      ]}
                    />
                  </Col>

                  <Col span={24}>
                    <Card
                      title={t('menu.link')}
                      hoverable
                      actions={[
                        <Space className="action" key="logs" onClick={() => setLogsOpen(true)}>
                          <BarsOutlined />
                          {!isMobile && <span>{t('pages.index.logs')}</span>}
                        </Space>,
                        <Space className="action" key="config" onClick={openConfig}>
                          <ControlOutlined />
                          {!isMobile && <span>{t('pages.index.config')}</span>}
                        </Space>,
                        <Space className="action" key="backup" onClick={() => setBackupOpen(true)}>
                          <CloudServerOutlined />
                          {!isMobile && <span>{t('pages.index.backupTitle')}</span>}
                        </Space>,
                        <Space className="action" key="sys-history" onClick={() => setSysHistoryOpen(true)}>
                          <AreaChartOutlined />
                          {!isMobile && <span>{t('pages.index.systemHistoryTitle')}</span>}
                        </Space>,
                        <Space className="action" key="xray-metrics" onClick={() => setXrayMetricsOpen(true)}>
                          <LineChartOutlined />
                          {!isMobile && <span>{t('pages.index.xrayMetricsTitle')}</span>}
                        </Space>,
                      ]}
                    />
                  </Col>

                  <Col xs={24} xl={16}>
                    <Card hoverable className="index-stats-card">
                      <div className="index-stats-grid">
                        <section>
                          <h4>{t('pages.index.operationHours')}</h4>
                          <div className="index-stat"><span>Xray</span><strong>{TimeFormatter.formatSecond(status.appStats.uptime)}</strong></div>
                          <div className="index-stat"><span>OS</span><strong>{TimeFormatter.formatSecond(status.uptime)}</strong></div>
                        </section>
                        <section>
                          <h4>{t('usage')}</h4>
                          <div className="index-stat"><span>{t('pages.index.memory')}</span><strong>{SizeFormatter.sizeFormat(status.appStats.mem)}</strong></div>
                          <div className="index-stat"><span>{t('pages.index.threads')}</span><strong>{status.appStats.threads}</strong></div>
                        </section>
                        <section>
                          <h4>{t('pages.index.totalData')}</h4>
                          <div className="index-stat"><span>{t('pages.index.sent')}</span><strong>{SizeFormatter.sizeFormat(status.netTraffic.sent)}</strong></div>
                          <div className="index-stat"><span>{t('pages.index.received')}</span><strong>{SizeFormatter.sizeFormat(status.netTraffic.recv)}</strong></div>
                        </section>
                        <section>
                          <h4>{t('pages.index.connectionCount')}</h4>
                          <div className="index-stat"><span>TCP</span><strong>{status.tcpCount}</strong></div>
                          <div className="index-stat"><span>UDP</span><strong>{status.udpCount}</strong></div>
                        </section>
                      </div>
                    </Card>
                  </Col>

                  <Col xs={24} xl={8}>
                    <Card
                      title={t('pages.index.ipAddresses')}
                      hoverable
                      className="index-ip-card"
                      extra={
                        <Tooltip
                          title={t('pages.index.toggleIpVisibility')}
                          placement={isMobile ? 'topRight' : 'top'}
                        >
                          {showIp ? (
                            <EyeOutlined
                              className="ip-toggle-icon"
                              onClick={() => setShowIp(false)}
                            />
                          ) : (
                            <EyeInvisibleOutlined
                              className="ip-toggle-icon"
                              onClick={() => setShowIp(true)}
                            />
                          )}
                        </Tooltip>
                      }
                    >
                      <div className={showIp ? 'ip-visible' : 'ip-hidden'}>
                        <div className="index-stat"><span>IPv4</span><strong className="ip-value">{status.publicIP.ipv4}</strong></div>
                        <div className="index-stat"><span>IPv6</span><strong className="ip-value">{status.publicIP.ipv6}</strong></div>
                      </div>
                    </Card>
                  </Col>
                </Row>
              )}
            </Spin>
          </Layout.Content>
        </Layout>

        <LazyMount when={logsOpen}>
          <LogModal open={logsOpen} onClose={() => setLogsOpen(false)} />
        </LazyMount>
        <LazyMount when={backupOpen}>
          <BackupModal
            open={backupOpen}
            basePath={basePath}
            onClose={() => setBackupOpen(false)}
            onBusy={setBusy}
          />
        </LazyMount>
        <LazyMount when={sysHistoryOpen}>
          <SystemHistoryModal
            open={sysHistoryOpen}
            status={status}
            onClose={() => setSysHistoryOpen(false)}
          />
        </LazyMount>
        <LazyMount when={xrayMetricsOpen}>
          <XrayMetricsModal open={xrayMetricsOpen} onClose={() => setXrayMetricsOpen(false)} />
        </LazyMount>
        <LazyMount when={xrayLogsOpen}>
          <XrayLogModal open={xrayLogsOpen} onClose={() => setXrayLogsOpen(false)} />
        </LazyMount>
        <LazyMount when={versionOpen}>
          <VersionModal
            open={versionOpen}
            status={status}
            onClose={() => setVersionOpen(false)}
            onBusy={setBusy}
          />
        </LazyMount>

        <LazyMount when={configTextOpen}>
          <Modal
            open={configTextOpen}
            title={t('pages.index.config')}
            width={isMobile ? '100%' : 900}
            style={isMobile
              ? { top: 20, maxWidth: 'calc(100vw - 16px)' }
              : { top: 20 }}
            onCancel={() => setConfigTextOpen(false)}
            footer={[
              <Button
                key="download"
                onClick={downloadConfig}
                size={isMobile ? 'small' : 'middle'}
                icon={<CloudDownloadOutlined />}
              >
                {isMobile ? t('download') : 'config.json'}
              </Button>,
              <Button
                key="copy"
                type="primary"
                onClick={copyConfig}
                size={isMobile ? 'small' : 'middle'}
                icon={<CopyOutlined />}
              >
                {t('copy')}
              </Button>,
            ]}
          >
            <JsonEditor
              value={configText}
              onChange={setConfigText}
              minHeight={isMobile ? '300px' : 'calc(100vh - 220px)'}
              maxHeight={isMobile ? '70vh' : 'calc(100vh - 220px)'}
              readOnly
            />
          </Modal>
        </LazyMount>
      </Layout>
    </ConfigProvider>
  );
}
