import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Collapse,
  Input,
  InputNumber,
  Select,
  Space,
  Switch,
} from 'antd';
import type { AllSetting } from '@/models/setting';
import { HttpUtil, LanguageManager } from '@/utils';
import SettingListItem from '@/components/SettingListItem';

interface ApiMsg<T = unknown> {
  success?: boolean;
  obj?: T;
}

interface GeneralTabProps {
  allSetting: AllSetting;
  updateSetting: (patch: Partial<AllSetting>) => void;
}

const REMARK_MODELS: Record<string, string> = { i: 'Inbound', e: 'Email', o: 'Other' };
const REMARK_SEPARATORS = [' ', '-', '_', '@', ':', '~', '|', ',', '.', '/'];
const DATEPICKER_LIST: { name: string; value: 'gregorian' | 'jalalian' }[] = [
  { name: 'Gregorian (Standard)', value: 'gregorian' },
  { name: 'Jalalian (شمسی)', value: 'jalalian' },
];

export default function GeneralTab({ allSetting, updateSetting }: GeneralTabProps) {
  const { t } = useTranslation();

  const [lang, setLang] = useState<string>(() => LanguageManager.getLanguage());
  const [inboundOptions, setInboundOptions] = useState<{ label: string; value: string }[]>([]);
  const [serverProviderAPIKeyDraft, setServerProviderAPIKeyDraft] = useState('');

  useEffect(() => {
    let cancelled = false;
    (async () => {
      // /options is the slim picker-shaped endpoint — it skips the heavy
      // per-client settings and clientStats payloads that /list ships.
      const msg = await HttpUtil.get('/panel/api/inbounds/options') as ApiMsg<{
        tag: string; protocol: string; port: number;
      }[]>;
      if (cancelled) return;
      if (msg?.success && Array.isArray(msg.obj)) {
        setInboundOptions(msg.obj.map((ib) => ({
          label: `${ib.tag} (${ib.protocol}@${ib.port})`,
          value: ib.tag,
        })));
      } else {
        setInboundOptions([]);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const remarkModel = useMemo(() => {
    const rm = allSetting.remarkModel || '';
    return rm.length > 1 ? rm.substring(1).split('') : [];
  }, [allSetting.remarkModel]);

  const remarkSeparator = useMemo(() => {
    const rm = allSetting.remarkModel || '-';
    return rm.length > 1 ? rm.charAt(0) : '-';
  }, [allSetting.remarkModel]);

  const remarkSample = useMemo(() => {
    const parts = remarkModel.map((k) => REMARK_MODELS[k]);
    return parts.length === 0 ? '' : parts.join(remarkSeparator);
  }, [remarkModel, remarkSeparator]);

  function setRemarkModel(parts: string[]) {
    updateSetting({ remarkModel: remarkSeparator + parts.join('') });
  }

  function setRemarkSeparator(sep: string) {
    const tail = (allSetting.remarkModel || '-').substring(1);
    updateSetting({ remarkModel: sep + tail });
  }

  const ldapInboundTagList = useMemo(() => {
    const csv = allSetting.ldapInboundTags || '';
    return csv.length ? csv.split(',').map((s) => s.trim()).filter(Boolean) : [];
  }, [allSetting.ldapInboundTags]);

  function setLdapInboundTagList(list: string[]) {
    updateSetting({ ldapInboundTags: Array.isArray(list) ? list.join(',') : '' });
  }

  function onLangChange(value: string) {
    setLang(value);
    LanguageManager.setLanguage(value);
  }

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

  return (
    <Collapse defaultActiveKey="1" items={[
      {
        key: '1',
        label: t('pages.settings.panelSettings'),
        children: (
          <>
            <SettingListItem
              paddings="small"
              title={t('pages.settings.remarkModel')}
              description={<>{t('pages.settings.sampleRemark')}: <i>#{remarkSample}</i></>}
            >
              <Space.Compact style={{ width: '100%' }}>
                <Select
                  mode="multiple"
                  value={remarkModel}
                  onChange={setRemarkModel}
                  style={{ paddingRight: '.5rem', minWidth: '80%', width: 'auto' }}
                  options={Object.entries(REMARK_MODELS).map(([k, l]) => ({ value: k, label: l }))}
                />
                <Select
                  value={remarkSeparator}
                  onChange={setRemarkSeparator}
                  style={{ width: '20%' }}
                  options={REMARK_SEPARATORS.map((s) => ({ value: s, label: s }))}
                />
              </Space.Compact>
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelListeningIP')} description={t('pages.settings.panelListeningIPDesc')}>
              <Input value={allSetting.webListen} onChange={(e) => updateSetting({ webListen: e.target.value })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelListeningDomain')} description={t('pages.settings.panelListeningDomainDesc')}>
              <Input value={allSetting.webDomain} onChange={(e) => updateSetting({ webDomain: e.target.value })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelPort')} description={t('pages.settings.panelPortDesc')}>
              <InputNumber value={allSetting.webPort} min={1} max={65535} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ webPort: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelUrlPath')} description={t('pages.settings.panelUrlPathDesc')}>
              <Input value={allSetting.webBasePath} onChange={(e) => updateSetting({ webBasePath: e.target.value })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.sessionMaxAge')} description={t('pages.settings.sessionMaxAgeDesc')}>
              <InputNumber value={allSetting.sessionMaxAge} min={60} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ sessionMaxAge: Number(v) || 0 })} />
            </SettingListItem>

            <SettingListItem
              paddings="small"
              title="可信代理 CIDR"
              description="允许设置转发主机、协议和客户端 IP 头的 IP/CIDR，多个用英文逗号分隔。"
            >
              <Input
                value={allSetting.trustedProxyCIDRs}
                placeholder="127.0.0.1/32,::1/128"
                onChange={(e) => updateSetting({ trustedProxyCIDRs: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.panelProxy')} description={t('pages.settings.panelProxyDesc')}>
              <Input
                value={allSetting.panelProxy}
                placeholder="socks5:// or http://user:pass@host:port"
                onChange={(e) => updateSetting({ panelProxy: e.target.value })}
              />
            </SettingListItem>

            <SettingListItem paddings="small" title="服务器商" description="用于服务器信息页拉取 VPS 商家的套餐、流量和节点位置。">
              <Select
                value={allSetting.serverProvider || ''}
                onChange={(v) => updateSetting({ serverProvider: v })}
                style={{ width: '100%' }}
                options={[
                  { value: '', label: '不启用' },
                  { value: '64clouds', label: '自定义' },
                ]}
              />
            </SettingListItem>

            {allSetting.serverProvider === '64clouds' && (
              <>
                <SettingListItem paddings="small" title="自定义 VEID">
                  <Input
                    value={allSetting.serverProviderVEID}
                    placeholder="请输入 VEID"
                    onChange={(e) => updateSetting({ serverProviderVEID: e.target.value })}
                  />
                </SettingListItem>
                <SettingListItem
                  paddings="small"
                  title="自定义 API KEY"
                  description={allSetting.hasServerProviderAPIKey ? '已配置，输入新密钥才会替换；留空保存会保留当前密钥。' : '请填写服务器商提供的 API KEY。'}
                >
                  <Input.Password
                    value={serverProviderAPIKeyDraft}
                    placeholder={allSetting.hasServerProviderAPIKey ? '留空保留当前密钥，输入新密钥替换' : '请输入 API KEY'}
                    onChange={(e) => {
                      setServerProviderAPIKeyDraft(e.target.value);
                      updateSetting({ serverProviderAPIKey: e.target.value });
                    }}
                  />
                </SettingListItem>
              </>
            )}

            <SettingListItem paddings="small" title={t('pages.settings.pageSize')} description={t('pages.settings.pageSizeDesc')}>
              <InputNumber value={allSetting.pageSize || 25} min={1} max={1000} step={5} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ pageSize: Math.max(1, Number(v) || 25) })} />
            </SettingListItem>

            <SettingListItem paddings="small" title={t('pages.settings.language')}>
              <Select
                value={lang}
                onChange={onLangChange}
                style={{ width: '100%' }}
                options={langOptions}
              />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '2',
        label: t('pages.settings.notifications'),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.expireTimeDiff')} description={t('pages.settings.expireTimeDiffDesc')}>
              <InputNumber value={allSetting.expireDiff} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ expireDiff: Number(v) || 0 })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.trafficDiff')} description={t('pages.settings.trafficDiffDesc')}>
              <InputNumber value={allSetting.trafficDiff} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ trafficDiff: Number(v) || 0 })} />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '3',
        label: t('pages.settings.certs'),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.publicKeyPath')} description={t('pages.settings.publicKeyPathDesc')}>
              <Input value={allSetting.webCertFile} onChange={(e) => updateSetting({ webCertFile: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.privateKeyPath')} description={t('pages.settings.privateKeyPathDesc')}>
              <Input value={allSetting.webKeyFile} onChange={(e) => updateSetting({ webKeyFile: e.target.value })} />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '4',
        label: t('pages.settings.externalTraffic'),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.externalTrafficInformEnable')} description={t('pages.settings.externalTrafficInformEnableDesc')}>
              <Switch checked={allSetting.externalTrafficInformEnable}
                onChange={(v) => updateSetting({ externalTrafficInformEnable: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.externalTrafficInformURI')} description={t('pages.settings.externalTrafficInformURIDesc')}>
              <Input
                value={allSetting.externalTrafficInformURI}
                placeholder="(http|https)://domain[:port]/path/"
                onChange={(e) => updateSetting({ externalTrafficInformURI: e.target.value })}
              />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.restartXrayOnClientDisable')} description={t('pages.settings.restartXrayOnClientDisableDesc')}>
              <Switch checked={allSetting.restartXrayOnClientDisable}
                onChange={(v) => updateSetting({ restartXrayOnClientDisable: v })} />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '5',
        label: t('pages.settings.dateAndTime'),
        children: (
          <>
            <SettingListItem paddings="small" title={t('pages.settings.timeZone')} description={t('pages.settings.timeZoneDesc')}>
              <Input value={allSetting.timeLocation} onChange={(e) => updateSetting({ timeLocation: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title={t('pages.settings.datepicker')} description={t('pages.settings.datepickerDescription')}>
              <Select
                value={allSetting.datepicker || 'gregorian'}
                onChange={(v) => updateSetting({ datepicker: v as 'gregorian' | 'jalalian' })}
                style={{ width: '100%' }}
                options={DATEPICKER_LIST.map((d) => ({ value: d.value, label: d.name }))}
              />
            </SettingListItem>
          </>
        ),
      },
      {
        key: '6',
        label: 'LDAP',
        children: (
          <>
            <SettingListItem paddings="small" title="启用 LDAP 同步">
              <Switch checked={allSetting.ldapEnable} onChange={(v) => updateSetting({ ldapEnable: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="LDAP 主机">
              <Input value={allSetting.ldapHost} onChange={(e) => updateSetting({ ldapHost: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="LDAP 端口">
              <InputNumber value={allSetting.ldapPort} min={1} max={65535} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ ldapPort: Number(v) || 0 })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="使用 TLS (LDAPS)">
              <Switch checked={allSetting.ldapUseTLS} onChange={(v) => updateSetting({ ldapUseTLS: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="绑定 DN">
              <Input value={allSetting.ldapBindDN} onChange={(e) => updateSetting({ ldapBindDN: e.target.value })} />
            </SettingListItem>
            <SettingListItem
              paddings="small"
              title={t('password')}
              description={allSetting.hasLdapPassword ? '已配置，留空会保留当前密码。' : '未配置。'}
            >
              <Input.Password
                value={allSetting.ldapPassword}
                placeholder={allSetting.hasLdapPassword ? '已配置，输入新值可替换' : ''}
                onChange={(e) => updateSetting({ ldapPassword: e.target.value })}
              />
            </SettingListItem>
            <SettingListItem paddings="small" title="基础 DN">
              <Input value={allSetting.ldapBaseDN} onChange={(e) => updateSetting({ ldapBaseDN: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="用户过滤器">
              <Input value={allSetting.ldapUserFilter} onChange={(e) => updateSetting({ ldapUserFilter: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="用户属性（用户名/邮箱）">
              <Input value={allSetting.ldapUserAttr} onChange={(e) => updateSetting({ ldapUserAttr: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="VLESS 标记属性">
              <Input value={allSetting.ldapVlessField} onChange={(e) => updateSetting({ ldapVlessField: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="通用标记属性（可选）" description="填写后会覆盖 VLESS 标记，例如 shadowInactive。">
              <Input value={allSetting.ldapFlagField} onChange={(e) => updateSetting({ ldapFlagField: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="启用值" description="多个值用英文逗号分隔，默认 true,1,yes,on。">
              <Input value={allSetting.ldapTruthyValues} onChange={(e) => updateSetting({ ldapTruthyValues: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="反转标记" description="当属性表示禁用时开启，例如 shadowInactive。">
              <Switch checked={allSetting.ldapInvertFlag} onChange={(v) => updateSetting({ ldapInvertFlag: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="同步计划" description="Cron 风格表达式，例如 @every 1m。">
              <Input value={allSetting.ldapSyncCron} onChange={(e) => updateSetting({ ldapSyncCron: e.target.value })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="入站标签" description="LDAP 同步可以在这些入站上自动创建或删除客户端。">
              <>
                <Select
                  mode="multiple"
                  value={ldapInboundTagList}
                  onChange={setLdapInboundTagList}
                  style={{ width: '100%' }}
                  options={inboundOptions}
                />
                {inboundOptions.length === 0 && (
                  <div className="ldap-no-inbounds">没有找到入站，请先在入站列表中创建。</div>
                )}
              </>
            </SettingListItem>
            <SettingListItem paddings="small" title="自动创建客户端">
              <Switch checked={allSetting.ldapAutoCreate} onChange={(v) => updateSetting({ ldapAutoCreate: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="自动删除客户端">
              <Switch checked={allSetting.ldapAutoDelete} onChange={(v) => updateSetting({ ldapAutoDelete: v })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="默认总流量 (GB)">
              <InputNumber value={allSetting.ldapDefaultTotalGB} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ ldapDefaultTotalGB: Number(v) || 0 })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="默认到期天数">
              <InputNumber value={allSetting.ldapDefaultExpiryDays} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ ldapDefaultExpiryDays: Number(v) || 0 })} />
            </SettingListItem>
            <SettingListItem paddings="small" title="默认 IP 限制">
              <InputNumber value={allSetting.ldapDefaultLimitIP} min={0} style={{ width: '100%' }}
                onChange={(v) => updateSetting({ ldapDefaultLimitIP: Number(v) || 0 })} />
            </SettingListItem>
          </>
        ),
      },
    ]} />
  );
}
