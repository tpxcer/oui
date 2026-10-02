import dayjs from 'dayjs';
import localeData from 'dayjs/plugin/localeData';
import { describe, expect, it } from 'vitest';

import { dateTimePickerLocale } from '@/components/date-time-picker-locale';

dayjs.extend(localeData);

describe('dateTimePickerLocale', () => {
  it('localizes simplified Chinese month, weekday and actions', () => {
    const locale = dateTimePickerLocale('zh-CN');

    expect(locale?.lang.now).toBe('此刻');
    expect(locale?.lang.ok).toBe('确定');
    expect(dayjs('2027-03-01').locale('zh-cn').format('MMM YYYY')).toBe('3月 2027');
    expect(dayjs().locale('zh-cn').localeData().weekdaysMin()).toEqual([
      '日', '一', '二', '三', '四', '五', '六',
    ]);
  });

  it('localizes traditional Chinese and leaves other languages unchanged', () => {
    const locale = dateTimePickerLocale('zh-TW');

    expect(locale?.lang.now).toBe('此刻');
    expect(locale?.lang.ok).toBe('確 定');
    expect(dayjs('2027-03-01').locale('zh-tw').format('MMM YYYY')).toBe('3月 2027');
    expect(dateTimePickerLocale('en-US')).toBeUndefined();
  });
});
