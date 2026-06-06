import { HttpUtil } from '@/utils';

export type SettingSecretKey = 'tgBotToken' | 'serverProviderAPIKey';

interface SettingSecretResponse {
  key: string;
  value: string;
  configured: boolean;
}

export async function fetchSettingSecret(key: SettingSecretKey): Promise<string> {
  const msg = await HttpUtil.post<SettingSecretResponse>(
    '/panel/setting/secret',
    { key },
    { silent: true },
  );
  if (!msg?.success || !msg.obj) {
    throw new Error(msg?.msg || '读取已保存密钥失败');
  }
  return msg.obj.value || '';
}
