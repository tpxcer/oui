import { HttpUtil, PromiseUtil } from '@/utils';

export interface PanelStatus {
  panelVersion?: string;
}

export interface PanelUpdateInfo {
  currentVersion: string;
  latestVersion: string;
  updateAvailable: boolean;
}

const UPDATE_TARGET_KEY = 'oui.panelUpdate.targetVersion';

function cacheBustParams() {
  return { _t: Date.now() };
}

function noCacheHeaders() {
  return {
    'Cache-Control': 'no-cache',
    Pragma: 'no-cache',
  };
}

function normalizedBasePath() {
  const base = window.X_UI_BASE_PATH || '/';
  return base.endsWith('/') ? base : `${base}/`;
}

export async function getPanelUpdateInfoNoCache() {
  return HttpUtil.get<PanelUpdateInfo>(
    '/panel/api/server/getPanelUpdateInfo',
    cacheBustParams(),
    { silent: true, headers: noCacheHeaders() },
  );
}

export async function getLivePanelStatus(timeout = 2000) {
  return HttpUtil.get<PanelStatus>(
    '/panel/api/server/status',
    cacheBustParams(),
    { timeout, silent: true, headers: noCacheHeaders() },
  );
}

export async function fetchFreshPanelShellVersion(): Promise<string> {
  try {
    const url = `${normalizedBasePath()}panel/?_ouiShell=${Date.now()}`;
    const resp = await fetch(url, {
      cache: 'no-store',
      credentials: 'same-origin',
      headers: noCacheHeaders(),
    });
    if (!resp.ok) return '';
    const html = await resp.text();
    const match = html.match(/window\.X_UI_CUR_VER="([^"]*)"/);
    return match?.[1] || '';
  } catch {
    return '';
  }
}

export async function waitForUpdatedPanel(targetVersion: string): Promise<boolean> {
  await PromiseUtil.sleep(5000);
  const deadline = Date.now() + 240_000;
  let statusMatched = 0;

  while (Date.now() < deadline) {
    const msg = await getLivePanelStatus();
    if (msg?.success && msg.obj?.panelVersion === targetVersion) {
      statusMatched += 1;
      const shellVersion = await fetchFreshPanelShellVersion();
      if (shellVersion === targetVersion) return true;

      // Some reverse proxies strip request cache directives. Once the backend
      // reports the target version repeatedly, reloading is still better than
      // leaving the operator locked on the progress screen forever.
      if (statusMatched >= 3 && !shellVersion) return true;
    } else {
      statusMatched = 0;
    }
    await PromiseUtil.sleep(2000);
  }
  return false;
}

export function rememberPanelUpdateTarget(targetVersion: string) {
  try {
    sessionStorage.setItem(UPDATE_TARGET_KEY, targetVersion);
  } catch {
    // ignored
  }
}

export function clearPanelUpdateTarget() {
  try {
    sessionStorage.removeItem(UPDATE_TARGET_KEY);
  } catch {
    // ignored
  }
}

export function reloadPanelPage(targetVersion?: string) {
  if (targetVersion) rememberPanelUpdateTarget(targetVersion);
  const url = new URL(window.location.href);
  url.searchParams.set('_ouiUpdated', String(Date.now()));
  if (targetVersion) url.searchParams.set('_ouiTarget', targetVersion);
  window.location.replace(url.toString());
}
