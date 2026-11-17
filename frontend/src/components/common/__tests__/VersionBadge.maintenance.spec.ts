import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import VersionBadge from '../VersionBadge.vue'
import { useAppStore } from '@/stores/app'

const { auth, getVersion, checkUpdates, performUpdate, rollback, getRollbackVersions, restartService } = vi.hoisted(() => ({
  auth: { isAdmin: true },
  getVersion: vi.fn(), checkUpdates: vi.fn(), performUpdate: vi.fn(),
  rollback: vi.fn(), getRollbackVersions: vi.fn(), restartService: vi.fn(),
}))

vi.mock('@/api/admin/system', () => ({
  getVersion, checkUpdates, performUpdate, rollback, getRollbackVersions, restartService,
}))
vi.mock('@/stores', async () => {
  const { useAppStore } = await import('@/stores/app')
  return { useAuthStore: () => auth, useAppStore }
})
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

function expectNoMaintenanceCalls() {
  for (const operation of [checkUpdates, performUpdate, rollback, getRollbackVersions, restartService]) {
    expect(operation).not.toHaveBeenCalled()
  }
}

describe('VersionBadge local-only maintenance policy', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
    auth.isAdmin = true
    getVersion.mockResolvedValue({ version: '0.2.22-calm.synthetic' })
  })

  it('never renders update/rollback/restart entries even with stale update state', async () => {
    const app = useAppStore()
    app.hasUpdate = true
    app.buildType = 'release'
    app.latestVersion = '999.0.0'
    const wrapper = mount(VersionBadge)
    await flushPromises()
    expect(wrapper.text()).toBe('v0.2.22-calm.synthetic')
    expect(wrapper.find('button').exists()).toBe(false)
    expect(wrapper.find('a').exists()).toBe(false)
    expect(getVersion).toHaveBeenCalledTimes(1)
    expectNoMaintenanceCalls()
    wrapper.unmount()
  })

  it('forced refresh only queries the running version, never upstream releases', async () => {
    const app = useAppStore()
    for (const force of [false, true, true]) {
      const result = await app.fetchVersion(force)
      expect(result?.has_update).toBe(false)
      expect(result?.latest_version).toBe('0.2.22-calm.synthetic')
    }
    expect(getVersion).toHaveBeenCalledTimes(3)
    expectNoMaintenanceCalls()
  })

  it('shows the supplied version to non-admin users without any version request', async () => {
    auth.isAdmin = false
    const wrapper = mount(VersionBadge, { props: { version: 'calm.synthetic' } })
    await flushPromises()
    expect(wrapper.text()).toBe('vcalm.synthetic')
    expect(getVersion).not.toHaveBeenCalled()
    expectNoMaintenanceCalls()
    wrapper.unmount()
  })
})
