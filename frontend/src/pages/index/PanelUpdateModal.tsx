import { useTranslation } from 'react-i18next';
import { Alert, Button, Modal, Tag } from 'antd';
import { CloudDownloadOutlined } from '@ant-design/icons';

import { HttpUtil, PromiseUtil } from '@/utils';
import type { PanelUpdateInfo } from '@/lib/panelUpdate';
import { reloadPanelPage, waitForUpdatedPanel } from '@/lib/panelUpdate';
import './PanelUpdateModal.css';

interface BusyEvent {
  busy: boolean;
  tip?: string;
}

interface PanelUpdateModalProps {
  open: boolean;
  info: PanelUpdateInfo;
  onClose: () => void;
  onBusy: (e: BusyEvent) => void;
}

export default function PanelUpdateModal({ open, info, onClose, onBusy }: PanelUpdateModalProps) {
  const { t } = useTranslation();
  const [modal, contextHolder] = Modal.useModal();

  function updatePanel() {
    const runUpdate = async () => {
      const baseTip = t('pages.index.dontRefresh');
      const tip = info.latestVersion ? `${baseTip} (${info.latestVersion})` : baseTip;
      onClose();
      onBusy({ busy: true, tip });
      try {
        const result = await HttpUtil.post('/panel/api/server/updatePanel');
        if (!result?.success) {
          onBusy({ busy: false });
          return;
        }
        const updated = await waitForUpdatedPanel(info.latestVersion);
        if (updated) {
          await PromiseUtil.sleep(800);
          reloadPanelPage(info.latestVersion);
        } else {
          onBusy({ busy: false });
        }
      } catch {
        onBusy({ busy: false });
      }
    };
    modal.confirm({
      title: t('pages.index.panelUpdateDialog'),
      content: t('pages.index.panelUpdateDialogDesc').replace('#version#', info.latestVersion || ''),
      okText: t('confirm'),
      cancelText: t('cancel'),
      onOk: () => {
        void runUpdate();
      },
    });
  }

  return (
    <>
      {contextHolder}
      <Modal
        open={open}
        title={t('pages.index.updatePanel')}
        footer={null}
        onCancel={onClose}
      >
        {info.updateAvailable && (
          <Alert
            type="warning"
            className="mb-12"
            title={t('pages.index.panelUpdateDesc')}
            showIcon
          />
        )}

        <div className="version-list">
          <div className="version-list-item">
            <span>{t('pages.index.currentPanelVersion')}</span>
            <Tag color="green">{info.currentVersion || '?'}</Tag>
          </div>
          {info.updateAvailable ? (
            <div className="version-list-item">
              <span>{t('pages.index.latestPanelVersion')}</span>
              <Tag color="purple">{info.latestVersion || '-'}</Tag>
            </div>
          ) : (
            <div className="version-list-item">
              <span>{t('pages.index.panelUpToDate')}</span>
              <Tag color="green">{t('pages.index.panelUpToDate')}</Tag>
            </div>
          )}
        </div>

        <div className="actions-row">
          <Button
            type="primary"
            disabled={!info.updateAvailable}
            onClick={updatePanel}
            icon={<CloudDownloadOutlined />}
          >
            {t('pages.index.updatePanel')}
          </Button>
        </div>
      </Modal>
    </>
  );
}
