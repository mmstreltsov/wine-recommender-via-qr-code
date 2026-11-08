// @vitest-environment node

import { expect, it } from 'vitest'
import { resolveConfig } from 'vite'
import config from './vite.config.ts'

it('TestNFR04_effective_dev_proxy_preserves_browser_Host_for_Origin_guard', async () => {
  const effective = await resolveConfig({ ...config, configFile: false }, 'serve')
  for (const path of ['/api', '/static']) {
    expect(effective.server.proxy?.[path]).toMatchObject({
      target: 'http://localhost:8080',
      changeOrigin: false,
    })
    expect(effective.server.proxy?.[path]).not.toHaveProperty('xfwd', true)
  }
})
