import { Alert, Typography } from 'antd';

export default function MigrationNotice() {
  return (
    <Alert
      type="warning"
      showIcon
      closable
      title="OUI 后续不再维护，建议升级至 o-ui"
      description={<>
        后续功能更新与维护将集中在 o-ui。现有 OUI 可继续使用，是否迁移由你决定。{' '}
        <Typography.Link href="https://github.com/tpxcer/o-ui" target="_blank" rel="noopener noreferrer" style={{ whiteSpace: 'nowrap' }}>查看 o-ui 项目</Typography.Link>
      </>}
    />
  );
}
