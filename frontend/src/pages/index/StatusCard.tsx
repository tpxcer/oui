import { useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Progress, Tooltip } from 'antd';
import { ArrowDownOutlined, ArrowUpOutlined, InfoCircleOutlined } from '@ant-design/icons';
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip as ChartTooltip,
  YAxis,
} from 'recharts';

import { CPUFormatter, SizeFormatter } from '@/utils';
import type { Status } from '@/models/status';
import {
  appendStatusCardHistory,
  resourceMeterTone,
  resourceSparklinePath,
  seedStatusCardHistory,
} from './status-card-history';
import './StatusCard.css';

interface StatusCardProps {
  status: Status;
  isMobile: boolean;
}

interface ResourceMeterProps {
  label: string;
  detail: React.ReactNode;
  percent: number;
  color: string;
  values: number[];
}

const UPLOAD_COLOR = '#1677ff';
const DOWNLOAD_COLOR = '#52c41a';

function ResourceMeter({ label, detail, percent, color, values }: ResourceMeterProps) {
  const value = Math.min(100, Math.max(0, Number.isFinite(percent) ? percent : 0));
  const tone = resourceMeterTone(value, color);
  const line = resourceSparklinePath(values);

  return (
    <div className="resource-meter" role="group" aria-label={`${label}: ${Math.round(value)}%`}>
      <div className="resource-meter-head">
        <span className="resource-meter-label">{label}</span>
        <span className="resource-meter-detail">{detail}</span>
      </div>
      <svg className="resource-meter-spark" viewBox="0 0 100 100" preserveAspectRatio="none" aria-hidden="true">
        {line && (
          <>
            <path d={`${line} L100,100 L0,100 Z`} fill={color} fillOpacity={0.14} />
            <path d={line} fill="none" stroke={color} strokeWidth={1.5} vectorEffect="non-scaling-stroke" />
          </>
        )}
      </svg>
      <strong className="resource-meter-value" style={{ color: tone }}>{Math.round(value)}%</strong>
      <Progress
        className="resource-meter-bar"
        percent={value}
        showInfo={false}
        strokeColor={tone}
        size="small"
      />
    </div>
  );
}

export default function StatusCard({ status, isMobile }: StatusCardProps) {
  const { t } = useTranslation();
  const [history, setHistory] = useState(() => seedStatusCardHistory(status));
  const reactId = useId();
  const chartId = reactId.replace(/[^a-zA-Z0-9]/g, '');

  useEffect(() => {
    setHistory((current) => appendStatusCardHistory(current, status));
  }, [status]);

  const speedFormatter = (value: number) => `${SizeFormatter.sizeFormat(value)}/s`;
  const usage = (value: { current: number; total: number }) =>
    `${SizeFormatter.sizeFormat(value.current)} / ${SizeFormatter.sizeFormat(value.total)}`;
  const networkSeries = [
    { key: 'up', label: t('pages.index.upload'), color: UPLOAD_COLOR, value: status.netIO.up, icon: <ArrowUpOutlined /> },
    { key: 'down', label: t('pages.index.download'), color: DOWNLOAD_COLOR, value: status.netIO.down, icon: <ArrowDownOutlined /> },
  ] as const;

  return (
    <Card hoverable className="status-card">
      <div className="status-overview">
        <section className="status-network" aria-label={t('pages.index.overallSpeed')}>
          <div className="status-network-title">{t('pages.index.overallSpeed')}</div>
          <div className="status-network-rates">
            {networkSeries.map((item) => (
              <div className="status-network-rate" key={item.key}>
                <span className="status-network-rate-label">
                  <i style={{ background: item.color }} />
                  {item.label}
                </span>
                <strong style={{ color: item.color }}>
                  {item.icon}
                  {speedFormatter(item.value)}
                </strong>
              </div>
            ))}
          </div>
          <ResponsiveContainer width="100%" height={isMobile ? 132 : 164}>
            <AreaChart data={history} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <defs>
                {networkSeries.map((item) => (
                  <linearGradient key={item.key} id={`${chartId}-${item.key}`} x1="0" y1="0" x2="0" y2="1">
                    <stop offset="0%" stopColor={item.color} stopOpacity={0.36} />
                    <stop offset="100%" stopColor={item.color} stopOpacity={0.04} />
                  </linearGradient>
                ))}
              </defs>
              <CartesianGrid stroke="var(--ant-color-border-secondary)" strokeDasharray="3 4" vertical={false} />
              <YAxis
                domain={[0, 'auto']}
                axisLine={false}
                tickLine={false}
                tick={{ fontSize: 10, fill: 'var(--ant-color-text-tertiary)' }}
                // Non-breaking space keeps recharts from wrapping "12.34 KB/s" onto two lines.
                tickFormatter={(value: number) => speedFormatter(value).replace(' ', ' ')}
                width={76}
              />
              <ChartTooltip
                cursor={{ stroke: 'var(--ant-color-border)', strokeDasharray: '2 4' }}
                contentStyle={{
                  background: 'var(--ant-color-bg-elevated)',
                  border: '1px solid var(--ant-color-border-secondary)',
                  borderRadius: 6,
                  boxShadow: '0 4px 14px rgba(0, 0, 0, 0.12)',
                  fontSize: 12,
                }}
                labelFormatter={() => ''}
                formatter={(rawValue, name) => [speedFormatter(Number(rawValue) || 0), name]}
              />
              {networkSeries.map((item) => (
                <Area
                  key={item.key}
                  type="monotone"
                  dataKey={item.key}
                  name={item.label}
                  stroke={item.color}
                  strokeWidth={1.8}
                  fill={`url(#${chartId}-${item.key})`}
                  dot={false}
                  activeDot={{ r: 3, fill: item.color, strokeWidth: 0 }}
                  isAnimationActive={false}
                />
              ))}
            </AreaChart>
          </ResponsiveContainer>
        </section>

        <section className="status-resources">
          <ResourceMeter
            label={t('pages.index.cpu')}
            detail={(
              <Tooltip
                title={(
                  <>
                    <div><b>{t('pages.index.cpu')}:</b> {CPUFormatter.cpuCoreFormat(status.cpuCores)}</div>
                    <div><b>{t('pages.index.logicalProcessors')}:</b> {status.logicalPro}</div>
                    <div>
                      <b>{t('pages.index.frequency')}:</b>{' '}
                      {CPUFormatter.cpuSpeedFormat(status.cpuSpeedMhz)}
                    </div>
                  </>
                )}
              >
                <span className="resource-meter-info">
                  {status.cpuSpeedMhz > 0 && CPUFormatter.cpuSpeedFormat(status.cpuSpeedMhz)}
                  <InfoCircleOutlined />
                </span>
              </Tooltip>
            )}
            percent={status.cpu.percent}
            color="#1677ff"
            values={history.map((point) => point.cpu)}
          />
          <ResourceMeter
            label={t('pages.index.memory')}
            detail={usage(status.mem)}
            percent={status.mem.percent}
            color="#13c2c2"
            values={history.map((point) => point.mem)}
          />
          <ResourceMeter
            label={t('pages.index.swap')}
            detail={usage(status.swap)}
            percent={status.swap.percent}
            color="#fa8c16"
            values={history.map((point) => point.swap)}
          />
          <ResourceMeter
            label={t('pages.index.storage')}
            detail={usage(status.disk)}
            percent={status.disk.percent}
            color="#722ed1"
            values={history.map((point) => point.disk)}
          />
        </section>
      </div>
    </Card>
  );
}
