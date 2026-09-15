import { useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Tooltip } from 'antd';
import { AreaChartOutlined } from '@ant-design/icons';
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
  seedStatusCardHistory,
  type StatusCardHistoryPoint,
} from './status-card-history';
import './StatusCard.css';

interface StatusCardProps {
  status: Status;
  isMobile: boolean;
}

interface ChartSeries {
  key: keyof Pick<StatusCardHistoryPoint, 'cpu' | 'up' | 'down' | 'mem' | 'disk'>;
  label: string;
  color: string;
}

interface ResourceChartProps {
  title: string;
  value: string;
  history: StatusCardHistoryPoint[];
  series: ChartSeries[];
  formatter: (value: number) => string;
  valueMax?: number;
  footer: React.ReactNode;
  height: number;
}

function ResourceChart({
  title,
  value,
  history,
  series,
  formatter,
  valueMax,
  footer,
  height,
}: ResourceChartProps) {
  const reactId = useId();
  const chartId = reactId.replace(/[^a-zA-Z0-9]/g, '');

  return (
    <section className="resource-chart" aria-label={`${title}: ${value}`}>
      <div className="resource-chart-heading">
        <span>{title}</span>
        <strong>{value}</strong>
      </div>
      <ResponsiveContainer width="100%" height={height}>
        <AreaChart data={history} margin={{ top: 8, right: 8, bottom: 2, left: 0 }}>
          <defs>
            {series.map((item) => (
              <linearGradient key={item.key} id={`${chartId}-${item.key}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor={item.color} stopOpacity={0.42} />
                <stop offset="100%" stopColor={item.color} stopOpacity={0.08} />
              </linearGradient>
            ))}
          </defs>
          <CartesianGrid stroke="var(--ant-color-border-secondary)" strokeDasharray="3 4" vertical={false} />
          <YAxis
            domain={valueMax == null ? [0, 'auto'] : [0, valueMax]}
            axisLine={false}
            tickLine={false}
            tick={{ fontSize: 10, fill: 'var(--ant-color-text-tertiary)' }}
            tickFormatter={formatter}
            width={58}
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
            formatter={(rawValue, name) => [formatter(Number(rawValue) || 0), name]}
          />
          {series.map((item) => (
            <Area
              key={item.key}
              type="monotone"
              dataKey={item.key}
              name={item.label}
              stroke={item.color}
              strokeWidth={1.7}
              fill={`url(#${chartId}-${item.key})`}
              dot={false}
              activeDot={{ r: 3, fill: item.color, strokeWidth: 0 }}
              isAnimationActive={false}
            />
          ))}
        </AreaChart>
      </ResponsiveContainer>
      <div className="resource-chart-footer">{footer}</div>
    </section>
  );
}

export default function StatusCard({ status, isMobile }: StatusCardProps) {
  const { t } = useTranslation();
  const [history, setHistory] = useState(() => seedStatusCardHistory(status));

  useEffect(() => {
    setHistory((current) => appendStatusCardHistory(current, status));
  }, [status]);

  const percentageFormatter = (value: number) => `${Math.round(value)}%`;
  const speedFormatter = (value: number) => `${SizeFormatter.sizeFormat(value)}/s`;
  const chartHeight = isMobile ? 118 : 132;

  return (
    <Card hoverable className="status-card">
      <div className="resource-chart-grid">
        <ResourceChart
          title={t('pages.index.cpu')}
          value={`${status.cpu.percent}%`}
          history={history}
          series={[{ key: 'cpu', label: t('pages.index.cpu'), color: status.cpu.color }]}
          formatter={percentageFormatter}
          valueMax={100}
          height={chartHeight}
          footer={(
            <span className="resource-chart-summary">
              <span className="resource-chart-dot" style={{ background: status.cpu.color }} />
              {t('pages.index.cpu')}: {CPUFormatter.cpuCoreFormat(status.cpuCores)}
              <Tooltip
                title={(
                  <>
                    <div><b>{t('pages.index.logicalProcessors')}:</b> {status.logicalPro}</div>
                    <div>
                      <b>{t('pages.index.frequency')}:</b>{' '}
                      {CPUFormatter.cpuSpeedFormat(status.cpuSpeedMhz)}
                    </div>
                  </>
                )}
              >
                <AreaChartOutlined className="resource-chart-info" />
              </Tooltip>
            </span>
          )}
        />

        <ResourceChart
          title={t('pages.index.overallSpeed')}
          value={`↑ ${speedFormatter(status.netIO.up)}  ↓ ${speedFormatter(status.netIO.down)}`}
          history={history}
          series={[
            { key: 'up', label: t('pages.index.upload'), color: '#1677ff' },
            { key: 'down', label: t('pages.index.download'), color: '#52c41a' },
          ]}
          formatter={speedFormatter}
          height={chartHeight}
          footer={(
            <span className="resource-chart-legend">
              <span><i style={{ background: '#1677ff' }} />{t('pages.index.upload')}</span>
              <span><i style={{ background: '#52c41a' }} />{t('pages.index.download')}</span>
            </span>
          )}
        />

        <ResourceChart
          title={t('pages.index.memory')}
          value={`${status.mem.percent}%`}
          history={history}
          series={[{ key: 'mem', label: t('pages.index.memory'), color: status.mem.color }]}
          formatter={percentageFormatter}
          valueMax={100}
          height={chartHeight}
          footer={(
            <span className="resource-chart-summary">
              <span className="resource-chart-dot" style={{ background: status.mem.color }} />
              {SizeFormatter.sizeFormat(status.mem.current)} / {SizeFormatter.sizeFormat(status.mem.total)}
            </span>
          )}
        />

        <ResourceChart
          title={t('pages.index.storage')}
          value={`${status.disk.percent}%`}
          history={history}
          series={[{ key: 'disk', label: t('pages.index.storage'), color: status.disk.color }]}
          formatter={percentageFormatter}
          valueMax={100}
          height={chartHeight}
          footer={(
            <span className="resource-chart-summary">
              <span className="resource-chart-dot" style={{ background: status.disk.color }} />
              {SizeFormatter.sizeFormat(status.disk.current)} / {SizeFormatter.sizeFormat(status.disk.total)}
            </span>
          )}
        />
      </div>
    </Card>
  );
}
