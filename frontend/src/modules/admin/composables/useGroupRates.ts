import { ref } from 'vue'
import { listAllGroupRates, listGroupRateHistory, listGroupRates, updateGroupRateType } from '../api/groupRates'
import type { GroupRate, GroupRateHistoryQuery, GroupRateHistoryRow, GroupRateSort, GroupRateStatusCounts, GroupRateStatusFilter } from '../types/groupRates'

export const useGroupRates = () => {
  const rates = ref<GroupRate[]>([])
  const history = ref<GroupRateHistoryRow[]>([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(10)
  const totalPages = ref(1)
  const types = ref<string[]>([])
  const platforms = ref<string[]>([])
  const search = ref('')
  const typeFilter = ref('')
  const platformFilter = ref('')
  const siteFilter = ref('')
  const statusFilter = ref<GroupRateStatusFilter>('all')
  const sortMode = ref<GroupRateSort>('multiplierAsc')
  const statusCounts = ref<GroupRateStatusCounts>({ all: 0, mapped: 0, unmapped: 0, deleted: 0 })
  const serverSupportsStatusFilters = ref(false)
  const isLoading = ref(false)
  const isHistoryLoading = ref(false)
  const isActionLoading = ref(false)
  const errorKey = ref<string | null>(null)
  const historyErrorKey = ref<string | null>(null)
  const siteFilteredRates = ref<GroupRate[]>([])
  let ratesRequestId = 0

  const applySiteFilteredPage = () => {
    total.value = siteFilteredRates.value.length
    totalPages.value = total.value > 0 ? Math.ceil(total.value / pageSize.value) : 0
    page.value = Math.min(Math.max(page.value, 1), Math.max(totalPages.value, 1))
    const offset = (page.value - 1) * pageSize.value
    rates.value = siteFilteredRates.value.slice(offset, offset + pageSize.value)
  }

  const loadSiteFilteredRates = async (requestId: number) => {
    const baseQuery = {
      search: search.value,
      type: typeFilter.value,
      platform: platformFilter.value,
      sort: sortMode.value,
    }
    const [activeRows, deletedRows, metadata] = await Promise.all([
      listAllGroupRates({ ...baseQuery, status: 'all' }),
      listAllGroupRates({ ...baseQuery, status: 'deleted' }),
      listGroupRates({ ...baseQuery, page: 1, status: 'all' }),
    ])
    if (requestId !== ratesRequestId) return
    const filterBySite = (items: GroupRate[]) => items.filter(item => item.siteId === siteFilter.value)
    const activeSiteRows = filterBySite(activeRows)
    const deletedSiteRows = filterBySite(deletedRows)

    statusCounts.value = {
      all: activeSiteRows.length,
      mapped: activeSiteRows.filter(item => item.mapped).length,
      unmapped: activeSiteRows.filter(item => !item.mapped).length,
      deleted: deletedSiteRows.length,
    }
    types.value = metadata.types
    platforms.value = metadata.platforms
    siteFilteredRates.value = statusFilter.value === 'deleted'
      ? deletedSiteRows
      : statusFilter.value === 'mapped'
        ? activeSiteRows.filter(item => item.mapped)
        : statusFilter.value === 'unmapped'
          ? activeSiteRows.filter(item => !item.mapped)
          : activeSiteRows
    serverSupportsStatusFilters.value = true
    pageSize.value = metadata.pageSize || 30
    applySiteFilteredPage()
  }

  const loadRates = async () => {
    const requestId = ++ratesRequestId
    isLoading.value = true
    errorKey.value = null
    try {
      if (siteFilter.value) {
        await loadSiteFilteredRates(requestId)
        return
      }

      const response = await listGroupRates({
        page: page.value,
        search: search.value,
        type: typeFilter.value,
        platform: platformFilter.value,
        status: statusFilter.value,
        sort: sortMode.value,
      })

      if (requestId !== ratesRequestId) return

      siteFilteredRates.value = []
      rates.value = response.items
      total.value = response.total
      page.value = response.page
      pageSize.value = response.pageSize
      totalPages.value = response.totalPages
      types.value = response.types
      platforms.value = response.platforms
      serverSupportsStatusFilters.value = response.statusCounts != null
      statusCounts.value = response.statusCounts ?? {
        all: response.items.filter(item => !item.deleted).length,
        mapped: response.items.filter(item => item.mapped && !item.deleted).length,
        unmapped: response.items.filter(item => !item.mapped && !item.deleted).length,
        deleted: response.items.filter(item => item.deleted).length,
      }
    } catch (error) {
      if (requestId !== ratesRequestId) return
      errorKey.value = error instanceof Error ? error.message : 'admin.groupRates.errors.unknown'
    } finally {
      if (requestId === ratesRequestId) {
        isLoading.value = false
      }
    }
  }

  const resetPageAndLoadRates = async () => {
    page.value = 1
    await loadRates()
  }

  const setSearch = async (value: string) => {
    search.value = value
    await resetPageAndLoadRates()
  }

  const setTypeFilter = async (value: string) => {
    typeFilter.value = value
    await resetPageAndLoadRates()
  }

  const setPlatformFilter = async (value: string) => {
    platformFilter.value = value
    await resetPageAndLoadRates()
  }

  const setSiteFilter = async (value: string) => {
    siteFilter.value = value
    await resetPageAndLoadRates()
  }

  const setStatusFilter = async (value: GroupRateStatusFilter) => {
    statusFilter.value = value
    await resetPageAndLoadRates()
  }

  const setSortMode = async (value: GroupRateSort) => {
    sortMode.value = value
    await resetPageAndLoadRates()
  }

  const goToPage = async (targetPage: number) => {
    const nextPage = Math.min(Math.max(targetPage, 1), totalPages.value || 1)
    if (nextPage === page.value) return

    page.value = nextPage
    if (siteFilter.value) {
      applySiteFilteredPage()
      return
    }
    await loadRates()
  }

  const focusGroup = (groupName: string): boolean => {
    if (!siteFilter.value || !groupName.trim()) return false
    const index = siteFilteredRates.value.findIndex(item => item.groupName === groupName)
    if (index < 0) return false
    page.value = Math.floor(index / pageSize.value) + 1
    applySiteFilteredPage()
    return true
  }

  const loadHistory = async (query: GroupRateHistoryQuery) => {
    isHistoryLoading.value = true
    historyErrorKey.value = null
    try {
      history.value = await listGroupRateHistory(query)
    } catch (error) {
      historyErrorKey.value = error instanceof Error ? error.message : 'admin.groupRates.errors.unknown'
    } finally {
      isHistoryLoading.value = false
    }
  }

  const saveType = async (rate: GroupRate, groupType: string) => {
    isActionLoading.value = true
    errorKey.value = null
    try {
      await updateGroupRateType({ siteId: rate.siteId, groupName: rate.groupName, type: groupType })
      await loadRates()
    } catch (error) {
      errorKey.value = error instanceof Error ? error.message : 'admin.groupRates.errors.unknown'
      throw error
    } finally {
      isActionLoading.value = false
    }
  }

  return {
    rates,
    history,
    total,
    page,
    pageSize,
    totalPages,
    types,
    platforms,
    search,
    typeFilter,
    platformFilter,
    siteFilter,
    statusFilter,
    sortMode,
    statusCounts,
    serverSupportsStatusFilters,
    isLoading,
    isHistoryLoading,
    isActionLoading,
    errorKey,
    historyErrorKey,
    loadRates,
    loadHistory,
    saveType,
    setSearch,
    setTypeFilter,
    setPlatformFilter,
    setSiteFilter,
    setStatusFilter,
    setSortMode,
    goToPage,
    focusGroup,
  }
}
