import type { PickerLocale } from 'antd/es/date-picker/generatePicker';
import zhCN from 'antd/es/date-picker/locale/zh_CN';
import zhTW from 'antd/es/date-picker/locale/zh_TW';
import 'dayjs/locale/zh-cn';
import 'dayjs/locale/zh-tw';

const DATE_PICKER_LOCALES: Readonly<Record<string, PickerLocale>> = {
  'zh-CN': zhCN,
  'zh-TW': zhTW,
};

export function dateTimePickerLocale(language: string): PickerLocale | undefined {
  return DATE_PICKER_LOCALES[language];
}
