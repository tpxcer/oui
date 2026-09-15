import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card, Tooltip } from 'antd';
import { AreaChartOutlined } from '@ant-design/icons';
import {
  CartesianGrid,
  Line,
  LineChart,
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

type ChartSeriesKey = keyof Pick<
  StatusCardHistoryPoint,
  'cpu' | 'up' | 'down' | 'mem' | 'swap' | 'disk'
>;

interface ChartSeries {
  key: ChartSeriesKey;
  label: string;
  color: string;
  axis: 'percentage' | 'speed';
  value: string;
  details: React.ReactNode;
  showInfo?: boolean;
}

export default function StatusCard({ status, isMobile }: StatusCardProps) {
  const { t } = useTranslation();
  const [history, setHistory] = useState(() => seedStatusCardHistory(status));

  useEffect(() => {
    setHistory((current) => appendStatusCardHistory(current, status));
  }, [status]);

  const percentageFormatter = (value: number) => `${Math.round(value)}%`;
  const speedFormatter = (value: number) => `${SizeFormatter.sizeFormat(value)}/s`;
  const series: ChartSeries[] = [
    {
      key: 'cpu',
      label: t('pages.index.cpu'),
      color: '#1677ff',
      axis: 'percentage',
      value: `${status.cpu.percent}%`,
      details: (
        <>
          <div><b>{t('pages.index.cpu')}:</b> {CPUFormatter.cpuCoreFormat(status.cpuCores)}</div>
          <div><b>{t('pages.index.logicalProcessors')}:</b> {status.logicalPro}</div>
          <div>
            <b>{t('pages.index.frequency')}:</b>{' '}
            {CPUFormatter.cpuSpeedFormat(status.cpuSpeedMhz)}
          </div>
        </>
      ),
      showInfo: true,
    },
    {
      key: 'disk',
      label: t('pages.index.storage'),
      color: '#722ed1',
      axis: 'percentage',
      value: `${status.disk.percent}%`,
      details: `${SizeFormatter.sizeFormat(status.disk.current)} / ${SizeFormatter.sizeFormat(status.disk.total)}`,
    },
    {
      key: 'mem',
      label: t('pages.index.memory'),
      color: '#13c2c2',
      axis: 'percentage',
      value: `${status.mem.percent}%`,
      details: `${SizeFormatter.sizeFormat(status.mem.current)} / ${SizeFormatter.sizeFormat(status.mem.total)}`,
    },
    {
      key: 'swap',
      label: t('pages.index.swap'),
      color: '#fa8c16',
      axis: 'percentage',
      value: `${status.swap.percent}%`,
      details: `${SizeFormatter.sizeFormat(status.swap.current)} / ${SizeFormatter.sizeFormat(status.swap.total)}`,
    },
    {
      key: 'up',
      label: t('pages.index.upload'),
      color: '#eb2f96',
      axis: 'speed',
      value: speedFormatter(status.netIO.up),
      details: speedFormatter(status.netIO.up),
    },
    {
      key: 'down',
      label: t('pages.index.download'),
      color: '#52c41a',
      axis: 'speed',
      value: speedFormatter(status.netIO.down),
      details: speedFormatter(status.netIO.down),
    },
  ];
  const chartHeight = isMobile ? 190 : 230;

  return (
    <Card hoverable className="status-card">
      <section
        className="resource-chart resource-chart-combined"
        aria-label={series.map((item) => `${item.label}: ${item.value}`).join(', ')}
      >
        <div className="resource-chart-heading">
          <span>{t('pages.index.trendLast2Min')}</span>
        </div>
        <ResponsiveContainer width="100%" height={chartHeight}>
          <LineChart data={history} margin={{ top: 8, right: 4, bottom: 2, left: 0 }}>
            <CartesianGrid stroke="var(--ant-color-border-secondary)" strokeDasharray="3 4" vertical={false} />
            <YAxis
              yAxisId="percentage"
              domain={[0, 100]}
              axisLine={false}
              tickLine={false}
              tick={{ fontSize: 10, fill: 'var(--ant-color-text-tertiary)' }}
              tickFormatter={percentageFormatter}
              width={46}
            />
            <YAxis
              yAxisId="speed"
              orientation="right"
              domain={[0, 'auto']}
              axisLine={false}
              tickLine={false}
              tick={{ fontSize: 10, fill: 'var(--ant-color-text-tertiary)' }}
              tickFormatter={speedFormatter}
              width={70}
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
              formatter={(rawValue, name) => {
                const item = series.find((candidate) => candidate.label === String(name));
                const formatter = item?.axis === 'speed' ? speedFormatter : percentageFormatter;
                return [formatter(Number(rawValue) || 0), name];
              }}
            />
            {series.map((item) => (
              <Line
                key={item.key}
                yAxisId={item.axis}
                type="monotone"
                dataKey={item.key}
                name={item.label}
                stroke={item.color}
                strokeWidth={1.8}
                dot={false}
                activeDot={{ r: 3, fill: item.color, strokeWidth: 0 }}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ResponsiveContainer>
        <div className="resource-chart-footer">
          <span className="resource-chart-legend resource-chart-legend-combined">
            {series.map((item) => (
              <Tooltip key={item.key} title={item.details}>
                <span>
                  <i style={{ background: item.color }} />
                  {item.label} {item.value}
                  {item.showInfo && <AreaChartOutlined className="resource-chart-info" />}
                </span>
              </Tooltip>
            ))}
          </span>
        </div>
      </section>
    </Card>
  );
}
