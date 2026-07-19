import { beforeEach, describe, expect, it, vi } from 'vitest';

import { setClientEnabled } from '@/hooks/useClients';
import { HttpUtil } from '@/utils';

vi.mock('@/utils', () => ({
  HttpUtil: {
    get: vi.fn(),
    post: vi.fn(),
  },
}));

describe('client enable request', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('sends only the desired enable state to the dedicated endpoint', async () => {
    vi.mocked(HttpUtil.post).mockResolvedValue({ success: true, msg: 'ok', obj: null });

    await setClientEnabled('client+one@example.com', false);

    expect(HttpUtil.post).toHaveBeenCalledOnce();
    expect(HttpUtil.post).toHaveBeenCalledWith(
      '/panel/api/clients/setEnable/client%2Bone%40example.com',
      { enable: false },
      { headers: { 'Content-Type': 'application/json' } },
    );
  });
});
