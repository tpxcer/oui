import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Collapse, Input, InputNumber, message, Select, Switch } from 'antd';
import { LanguageManager } from '@/utils';
import type { AllSetting } from '@/models/setting';
import SettingListItem from '@/components/SettingListItem';
import { fetchSettingSecret } from '@/api/settingSecrets';

interface TelegramTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

const telegramNotifyTimeOptions = [
  { value: '@daily', label: '每天' },
  { value: '@weekly', label: '每周' },
  { value: '@every 360h', label: '15天' },
  { value: '@monthly', label: '每月' },
];

export default function TelegramTab({ allSetting, updateSetting }: TelegramTabProps) {
  const { t } = useTranslation();
  const [tgBotTokenDraft, setTgBotTokenDraft] = useState('');
  const [tgBotTokenVisible, setTgBotTokenVisible] = useState(false);
  const [tgBotTokenLoading, setTgBotTokenLoading] = useState(false);
  const tgTokenRequired = allSetting.tgBotEnable && !allSetting.hasTgBotToken && tgBotTokenDraft.trim() === '';
  const tgChatIdRequired = allSetting.tgBotEnable && allSetting.tgBotChatId.trim() === '';

  const langOptions = useMemo(
    () => LanguageManager.supportedLanguages.map((l: { value: string; name: string; icon: string }) => ({
      value: l.value,
      label: (
        <>
          <span role="img" aria-label={l.name}>{l.icon}</span>
          &nbsp;&nbsp;<span>{l.name}</span>
        </>
      ),
    })),
    [],
  );

  function handleTgBotTokenVisibleChange(visible: boolean) {
    if (!visible) {
      setTgBotTokenVisible(false);
      return;
    }
    if (tgBotTokenLoading) return;
    if (!tgBotTokenDraft && allSetting.hasTgBotToken) {
      setTgBotTokenLoading(true);
      fetchSettingSecret('tgBotToken')
        .then((value) => {
          setTgBotTokenDraft(value);
          setTgBotTokenVisible(true);
          if (!value) message.warning('未读取到已保存的机器人API');
        })
        .catch((error) => {
          message.error(error instanceof Error ? error.message : '读取已保存机器人API失败');
        })
        .finally(() => {
          setTgBotTokenLoading(false);
        });
      return;
    }
    setTgBotTokenVisible(true);
  }

  return (
    <Collapse defaultActiveKey="1" items={[
      {
        key: '1',
        label: t('pages.settings.panelSettings'),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.telegramBotEnable')} description={t('pages.settings.telegramBotEnableDesc')}>
              <Switch checked={allSetting.tgBotEnable} onChange={(v) => updateSetting({ tgBotEnable: v })} />
            </SettingListItem>

            {!allSetting.tgBotEnable ? (
              <Alert className="mt-12" type="info" showIcon message="Telegram 机器人未启用，相关配置已隐藏。" />
            ) : (
              <>
                <SettingListItem
                  paddings="small"
                  title="输入 Telegram 机器人API"
                  description="启用 Telegram 机器人时，机器人API和聊天 ID 必填；已保存过机器人API时可留空沿用。"
                >
                  <Input.Password
                    value={tgBotTokenDraft}
                    status={tgTokenRequired ? 'error' : undefined}
                    placeholder={
                      tgBotTokenLoading
                        ? '正在读取已保存机器人API...'
                        : allSetting.hasTgBotToken
                          ? '已配置，点右侧眼睛查看；输入新机器人API替换'
                          : '输入 Telegram 机器人API'
                    }
                    visibilityToggle={{
                      visible: tgBotTokenVisible,
                      onVisibleChange: handleTgBotTokenVisibleChange,
                    }}
                    onChange={(e) => {
                      setTgBotTokenDraft(e.target.value);
                      updateSetting({ tgBotToken: e.target.value });
                    }}
                  />
                </SettingListItem>

                <SettingListItem paddings="small" title={t('pages.settings.telegramChatId')} description={t('pages.settings.telegramChatIdDesc')}>
                  <Input
                    value={allSetting.tgBotChatId}
                    status={tgChatIdRequired ? 'error' : undefined}
                    onChange={(e) => updateSetting({ tgBotChatId: e.target.value })}
                  />
                </SettingListItem>

                <SettingListItem paddings="small" title={t('pages.settings.telegramBotLanguage')}>
                  <Select
                    value={allSetting.tgLang}
                    onChange={(v) => updateSetting({ tgLang: v })}
                    style={{ width: '100%' }}
                    options={langOptions}
                  />
                </SettingListItem>

                <SettingListItem paddings="small" title={t('pages.settings.telegramAPIServer')} description={t('pages.settings.telegramAPIServerDesc')}>
                  <Input value={allSetting.tgBotAPIServer} placeholder="https://api.example.com"
                    onChange={(e) => updateSetting({ tgBotAPIServer: e.target.value })} />
                </SettingListItem>
              </>
            )}
          </>
        ),
      },
      allSetting.tgBotEnable ? {
        key: '2',
        label: t('pages.settings.notifications'),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.telegramNotifyTime')} description={t('pages.settings.telegramNotifyTimeDesc')}>
              <Select
                value={allSetting.tgRunTime || '@daily'}
                onChange={(v) => updateSetting({ tgRunTime: v })}
                style={{ width: '100%' }}
                options={telegramNotifyTimeOptions}
              />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.tgNotifyBackup')} description={t('pages.settings.tgNotifyBackupDesc')}>
              <Switch checked={allSetting.tgBotBackup} onChange={(v) => updateSetting({ tgBotBackup: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.tgNotifyLogin')} description={t('pages.settings.tgNotifyLoginDesc')}>
              <Switch checked={allSetting.tgBotLoginNotify} onChange={(v) => updateSetting({ tgBotLoginNotify: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.tgNotifyCpu')} description={t('pages.settings.tgNotifyCpuDesc')}>
              <InputNumber value={allSetting.tgCpu} min={0} max={100} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ tgCpu: Number(v) || 0 })} />
            </SettingListItem>
          </>
        ),
      } : null,
    ].filter((item): item is NonNullable<typeof item> => item !== null)} />
  );
}
