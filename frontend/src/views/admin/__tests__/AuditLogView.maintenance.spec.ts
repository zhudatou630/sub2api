import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import AuditLogView from '../AuditLogView.vue'

const { list, clear, getTotpStatus } = vi.hoisted(() => ({
  list: vi.fn(), clear: vi.fn(), getTotpStatus: vi.fn(),
}))

vi.mock('@/api/admin', () => ({ adminAPI: { audit: { list, clear } } }))
vi.mock('@/api', () => ({ totpAPI: { getStatus: getTotpStatus } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({ t: (key: string) => key }),
}))

const layout = { template: '<div><slot /></div>' }

describe('AuditLogView maintenance policy', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    list.mockResolvedValue({ items: [], total: 0 })
    getTotpStatus.mockResolvedValue({ enabled: true })
  })

  it('keeps audit queries working but never offers a TOTP opt-in for clear-all', async () => {
    const wrapper = mount(AuditLogView, {
      global: { stubs: {
        AppLayout: layout,
        TablePageLayout: { template: '<div><slot name="filters" /><slot name="table" /></div>' },
        Select: true, DataTable: true, Pagination: true, Icon: true,
        BaseDialog: true, ConfirmDialog: true,
      } },
    })
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(1)
    const clearButton = wrapper.findAll('button').find(button => button.text() === 'admin.audit.clearAll')!
    expect(clearButton.attributes('disabled')).toBeDefined()
    await clearButton.trigger('click')
    await flushPromises()
    expect(getTotpStatus).not.toHaveBeenCalled()
    expect(clear).not.toHaveBeenCalled()

    const search = wrapper.findAll('button').find(button => button.text() === 'common.search')!
    await search.trigger('click')
    await flushPromises()
    expect(list).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })
})
