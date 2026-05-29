import { useCallback, useEffect, useMemo, useState } from 'react';
import type { ComponentType } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Drawer, Layout, Menu, message } from 'antd';
import type { MenuProps } from 'antd';
import {
  ApiOutlined,
  CloudDownloadOutlined,
  ClusterOutlined,
  CloseOutlined,
  DashboardOutlined,
  GithubOutlined,
  ImportOutlined,
  LogoutOutlined,
  MenuOutlined,
  MoonFilled,
  MoonOutlined,
  SettingOutlined,
  SunOutlined,
  SyncOutlined,
  TeamOutlined,
  ToolOutlined,
} from '@ant-design/icons';

import { HttpUtil, PromiseUtil } from '@/utils';
import { pauseAnimationsUntilLeave, useTheme } from '@/hooks/useTheme';
import './AppSidebar.css';

const SIDEBAR_COLLAPSED_KEY = 'isSidebarCollapsed';
const REPO_URL = 'https://github.com/tpxcer/oui';
const LOGOUT_KEY = '__logout__';

type IconName = 'dashboard' | 'inbound' | 'team' | 'setting' | 'tool' | 'cluster' | 'logout' | 'apidocs';

interface PanelUpdateInfo {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
}

interface PanelStatus {
  panelVersion?: string;
}

const iconByName: Record<IconName, ComponentType> = {
  dashboard: DashboardOutlined,
  inbound: ImportOutlined,
  team: TeamOutlined,
  setting: SettingOutlined,
  tool: ToolOutlined,
  cluster: ClusterOutlined,
  logout: LogoutOutlined,
  apidocs: ApiOutlined,
};

function readCollapsed(): boolean {
  try {
    return JSON.parse(localStorage.getItem(SIDEBAR_COLLAPSED_KEY) || 'false');
  } catch {
    return false;
  }
}

function VersionBadge({ version, collapsed }: { version: string; collapsed?: boolean }) {
  if (!version) return null;
  const label = version;
  return (
    <a
      href={REPO_URL}
      target="_blank"
      rel="noopener noreferrer"
      className={`sider-version${collapsed ? ' is-collapsed' : ''}`}
      aria-label={`GitHub ${label}`}
      title={label}
    >
      <GithubOutlined />
      {!collapsed && <span className="sider-version-text">{label}</span>}
    </a>
  );
}

function SidebarUpdateButton({
  collapsed,
  info,
  checking,
  updating,
  progress,
  onCheck,
  onUpdate,
}: {
  collapsed?: boolean;
  info: PanelUpdateInfo | null;
  checking: boolean;
  updating: boolean;
  progress: number;
  onCheck: () => void;
  onUpdate: () => void;
}) {
  const updateAvailable = !!info?.updateAvailable;
  const nextVersion = info?.latestVersion || '';
  const title = updateAvailable && nextVersion ? `一键更新到 ${nextVersion}` : updateAvailable ? '一键更新' : '检测并自动更新';
  const label = updating ? `更新 ${Math.round(progress)}%` : checking ? '检测中' : updateAvailable && nextVersion ? `更新 ${nextVersion}` : updateAvailable ? '一键更新' : '检测更新';
  const Icon = updateAvailable ? CloudDownloadOutlined : SyncOutlined;
  const safeProgress = Math.max(0, Math.min(100, progress));
  return (
    <button
      type="button"
      className={`sidebar-update${collapsed ? ' is-collapsed' : ''}${updateAvailable ? ' has-update' : ''}${updating ? ' is-updating' : ''}`}
      title={title}
      aria-label={title}
      disabled={checking || updating}
      onClick={updateAvailable ? onUpdate : onCheck}
    >
      {updating && <span className="sidebar-update-progress" style={{ width: `${safeProgress}%` }} />}
      <span className="sidebar-update-content">
        <Icon spin={checking || updating} />
        {!collapsed && <span>{label}</span>}
      </span>
    </button>
  );
}

function ThemeCycleButton({ id, isDark, isUltra, onCycle, ariaLabel }: {
  id: string;
  isDark: boolean;
  isUltra: boolean;
  onCycle: () => void;
  ariaLabel: string;
}) {
  const icon = !isDark ? <SunOutlined /> : !isUltra ? <MoonOutlined /> : <MoonFilled />;
  return (
    <button
      id={id}
      type="button"
      className="sidebar-theme-cycle"
      aria-label={ariaLabel}
      title={ariaLabel}
      onClick={onCycle}
    >
      {icon}
    </button>
  );
}

export default function AppSidebar() {
  const { t } = useTranslation();
  const { isDark, isUltra, toggleTheme, toggleUltra } = useTheme();
  const navigate = useNavigate();
  const { pathname } = useLocation();

  const [collapsed, setCollapsed] = useState<boolean>(() => readCollapsed());
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [updateInfo, setUpdateInfo] = useState<PanelUpdateInfo | null>(null);
  const [checkingUpdate, setCheckingUpdate] = useState(false);
  const [updatingPanel, setUpdatingPanel] = useState(false);
  const [updateProgress, setUpdateProgress] = useState(0);

  const currentTheme: 'light' | 'dark' = isDark ? 'dark' : 'light';
  const panelVersion = window.X_UI_CUR_VER || '';

  const pollUntilUpdated = useCallback(async (targetVersion: string): Promise<boolean> => {
    await PromiseUtil.sleep(5000);
    const deadline = Date.now() + 180_000;
    while (Date.now() < deadline) {
      const msg = await HttpUtil.get<PanelStatus>('/panel/api/server/status', undefined, { timeout: 2000, silent: true });
      if (msg?.success && msg.obj?.panelVersion === targetVersion) return true;
      await PromiseUtil.sleep(2000);
    }
    return false;
  }, []);

  const reloadPanelPage = useCallback(() => {
    const url = new URL(window.location.href);
    url.searchParams.set('_ouiUpdated', String(Date.now()));
    window.location.replace(url.toString());
  }, []);

  const updatePanel = useCallback(async (info?: PanelUpdateInfo | null) => {
    if (info) setUpdateInfo(info);
    setUpdatingPanel(true);
    setUpdateProgress(8);
    try {
      const result = await HttpUtil.post('/panel/api/server/updatePanel');
      if (result?.success) {
        message.success(info?.latestVersion ? `已开始后台更新到 ${info.latestVersion}` : '已开始后台更新');
        const updated = info?.latestVersion ? await pollUntilUpdated(info.latestVersion) : false;
        setUpdateProgress(100);
        if (updated) {
          await PromiseUtil.sleep(800);
          reloadPanelPage();
          return;
        }
        message.info('后台更新仍在执行，请稍后手动刷新页面');
        setUpdatingPanel(false);
      } else {
        message.error(result?.msg || '启动后台更新失败');
        setUpdatingPanel(false);
      }
    } catch (error) {
      message.error(error instanceof Error ? error.message : '启动后台更新失败');
      setUpdatingPanel(false);
    }
  }, [pollUntilUpdated, reloadPanelPage]);

  const checkPanelUpdate = useCallback(async () => {
    setCheckingUpdate(true);
    try {
      const msg = await HttpUtil.get<PanelUpdateInfo>('/panel/api/server/getPanelUpdateInfo', undefined, { silent: true });
      if (msg?.success && msg.obj) {
        setUpdateInfo(msg.obj);
        if (msg.obj.updateAvailable) {
          message.info(msg.obj.latestVersion ? `发现新版本：${msg.obj.latestVersion}，开始自动更新` : '发现新版本，开始自动更新');
          await updatePanel(msg.obj);
        } else {
          message.success('当前已是最新版');
        }
      } else {
        message.error(msg?.msg || '版本检测失败，请检查服务器是否可以访问 GitHub');
      }
    } finally {
      setCheckingUpdate(false);
    }
  }, [updatePanel]);

  useEffect(() => {
    if (!updatingPanel) return undefined;
    const timer = window.setInterval(() => {
      setUpdateProgress((value) => {
        if (value >= 95) return value;
        return Math.min(95, value + Math.max(1, Math.round((95 - value) * 0.12)));
      });
    }, 700);
    return () => window.clearInterval(timer);
  }, [updatingPanel]);

  useEffect(() => {
    if (!updatingPanel) {
      setUpdateProgress(0);
    }
  }, [updatingPanel]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setCheckingUpdate(true);
      try {
        const msg = await HttpUtil.get<PanelUpdateInfo>('/panel/api/server/getPanelUpdateInfo', undefined, { silent: true });
        if (!cancelled && msg?.success && msg.obj) setUpdateInfo(msg.obj);
      } finally {
        if (!cancelled) setCheckingUpdate(false);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const tabs = useMemo<{ key: string; icon: IconName; title: string }[]>(() => [
    { key: '/', icon: 'dashboard', title: t('menu.dashboard') },
    { key: '/inbounds', icon: 'inbound', title: t('menu.inbounds') },
    { key: '/clients', icon: 'team', title: t('menu.clients') },
    { key: '/nodes', icon: 'cluster', title: t('menu.nodes') },
    { key: '/settings', icon: 'setting', title: t('menu.settings') },
    { key: '/xray', icon: 'tool', title: t('menu.xray') },
    { key: '/api-docs', icon: 'apidocs', title: t('menu.apiDocs') },
    { key: LOGOUT_KEY, icon: 'logout', title: t('logout') },
  ], [t]);

  const navItems = useMemo(() => tabs.filter((tab) => tab.icon !== 'logout'), [tabs]);
  const utilItems = useMemo(() => tabs.filter((tab) => tab.icon === 'logout'), [tabs]);

  const selectedKey = pathname === '' ? '/' : pathname;

  const toMenuItems = useCallback((items: typeof tabs): MenuProps['items'] =>
    items.map((tab) => {
      const Icon = iconByName[tab.icon];
      return {
        key: tab.key,
        icon: <Icon />,
        label: tab.title,
      };
    }),
  []);

  const openLink = useCallback(async (key: string) => {
    if (key === LOGOUT_KEY) {
      await HttpUtil.post('/logout');
      window.location.href = window.X_UI_BASE_PATH || '/';
      return;
    }
    navigate(key);
  }, [navigate]);

  const onMenuClick = useCallback<NonNullable<MenuProps['onClick']>>(({ key }) => {
    openLink(String(key));
  }, [openLink]);

  const onSiderCollapse = useCallback((isCollapsed: boolean, type: 'clickTrigger' | 'responsive') => {
    if (type === 'clickTrigger') {
      localStorage.setItem(SIDEBAR_COLLAPSED_KEY, String(isCollapsed));
      setCollapsed(isCollapsed);
    }
  }, []);

  const cycleTheme = useCallback((id: string) => {
    pauseAnimationsUntilLeave(id);
    if (!isDark) {
      toggleTheme();
      if (isUltra) toggleUltra();
    } else if (!isUltra) {
      toggleUltra();
    } else {
      toggleUltra();
      toggleTheme();
    }
  }, [isDark, isUltra, toggleTheme, toggleUltra]);

  return (
    <div className="ant-sidebar">
      <Layout.Sider
        theme={currentTheme}
        collapsible
        collapsed={collapsed}
        breakpoint="md"
        onCollapse={onSiderCollapse}
      >
        <div className={`sider-brand${collapsed ? ' sider-brand-collapsed' : ''}`}>
          <div className="brand-block">
            <span className="brand-text">OUI</span>
          </div>
          {!collapsed && (
            <div className="brand-actions">
              <ThemeCycleButton
                id="theme-cycle"
                isDark={isDark}
                isUltra={isUltra}
                onCycle={() => cycleTheme('theme-cycle')}
                ariaLabel={t('menu.theme')}
              />
            </div>
          )}
        </div>
        <Menu
          theme={currentTheme}
          mode="inline"
          selectedKeys={[selectedKey]}
          className="sider-nav"
          items={toMenuItems(navItems)}
          onClick={onMenuClick}
        />
        <div className="sider-footer">
          <VersionBadge version={panelVersion} collapsed={collapsed} />
          <SidebarUpdateButton
            collapsed={collapsed}
            info={updateInfo}
            checking={checkingUpdate}
            updating={updatingPanel}
            progress={updateProgress}
            onCheck={checkPanelUpdate}
            onUpdate={updatePanel}
          />
        </div>
        <Menu
          theme={currentTheme}
          mode="inline"
          selectedKeys={[selectedKey]}
          className="sider-utility"
          items={toMenuItems(utilItems)}
          onClick={onMenuClick}
        />
      </Layout.Sider>

      <Drawer
        placement="left"
        closable={false}
        open={drawerOpen}
        rootClassName={currentTheme}
        size="min(82vw, 320px)"
        styles={{
          wrapper: { padding: 0 },
          body: { padding: 0, display: 'flex', flexDirection: 'column', height: '100%' },
          header: { display: 'none' },
        }}
        onClose={() => setDrawerOpen(false)}
      >
        <div className="drawer-header">
          <div className="brand-block">
            <span className="drawer-brand">OUI</span>
          </div>
          <div className="drawer-header-actions">
            <ThemeCycleButton
              id="theme-cycle-drawer"
              isDark={isDark}
              isUltra={isUltra}
              onCycle={() => cycleTheme('theme-cycle-drawer')}
              ariaLabel={t('menu.theme')}
            />
            <button
              className="drawer-close"
              type="button"
              aria-label={t('close')}
              onClick={() => setDrawerOpen(false)}
            >
              <CloseOutlined />
            </button>
          </div>
        </div>
        <Menu
          theme={currentTheme}
          mode="inline"
          selectedKeys={[selectedKey]}
          className="drawer-menu drawer-nav"
          items={toMenuItems(navItems)}
          onClick={(info) => { onMenuClick(info); setDrawerOpen(false); }}
        />
        <div className="drawer-footer">
          <VersionBadge version={panelVersion} />
          <SidebarUpdateButton
            info={updateInfo}
            checking={checkingUpdate}
            updating={updatingPanel}
            progress={updateProgress}
            onCheck={checkPanelUpdate}
            onUpdate={updatePanel}
          />
        </div>
        <Menu
          theme={currentTheme}
          mode="inline"
          selectedKeys={[selectedKey]}
          className="drawer-menu drawer-utility"
          items={toMenuItems(utilItems)}
          onClick={(info) => { onMenuClick(info); setDrawerOpen(false); }}
        />
      </Drawer>

      {!drawerOpen && (
        <button
          className="drawer-handle"
          type="button"
          aria-label={t('menu.dashboard')}
          onClick={() => setDrawerOpen(true)}
        >
          <MenuOutlined />
        </button>
      )}

      {updatingPanel && (
        <div className="panel-update-lock" role="status" aria-live="polite">
          <div className="panel-update-lock-card">
            <CloudDownloadOutlined spin />
            <strong>{updateInfo?.latestVersion ? `正在更新到 ${updateInfo.latestVersion}` : '正在更新到最新版本'}</strong>
            <div className="panel-update-lock-bar">
              <span style={{ width: `${Math.max(0, Math.min(100, updateProgress))}%` }} />
            </div>
            <small>{Math.round(updateProgress)}%</small>
          </div>
        </div>
      )}
    </div>
  );
}
