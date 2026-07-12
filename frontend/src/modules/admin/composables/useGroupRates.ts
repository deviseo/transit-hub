import { ref } from 'vue'
import { listGroupRateHistory, listGroupRates, updateGroupRateType } from '../api/groupRates'
import type { GroupRate, GroupRateHistoryQuery, GroupRateHistoryRow } from '../types/groupRates'

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
  const isLoading = ref(false)
  const isHistoryLoading = ref(false)
  const isActionLoading = ref(false)
  const errorKey = ref<string | null>(null)
  const historyErrorKey = ref<string | null>(null)
  let ratesRequestId = 0

  const loadRates = async () => {
    const requestId = ++ratesRequestId
    isLoading.value = true
    errorKey.value = null
    try {
      const response = await listGroupRates({
        page: page.value,
        search: search.value,
        type: typeFilter.value,
        platform: platformFilter.value,
      })

      if (requestId !== ratesRequestId) return

      rates.value = response.items
      total.value = response.total
      page.value = response.page
      pageSize.value = response.pageSize
      totalPages.value = response.totalPages
      types.value = response.types
      platforms.value = response.platforms
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

  const goToPage = async (targetPage: number) => {
    const nextPage = Math.min(Math.max(targetPage, 1), totalPages.value || 1)
    if (nextPage === page.value) return

    page.value = nextPage
    await loadRates()
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

  void loadRates()

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
    goToPage,
  }
}
