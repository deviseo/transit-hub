import { ref } from 'vue'
import { listTickets } from '../api/tickets'
import type { AdminTicketListItem } from '../types/tickets'

export const useTickets = () => {
  const tickets = ref<AdminTicketListItem[]>([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(20)
  const totalPages = ref(1)
  const statusFilter = ref('')
  const isLoading = ref(false)
  const errorKey = ref<string | null>(null)

  const loadTickets = async () => {
    isLoading.value = true
    errorKey.value = null
    try {
      const response = await listTickets({
        page: page.value,
        pageSize: pageSize.value,
        status: statusFilter.value,
      })

      tickets.value = response.items
      total.value = response.total
      page.value = response.page
      pageSize.value = response.pageSize
      totalPages.value = response.totalPages
    } catch (error) {
      errorKey.value = error instanceof Error ? error.message : 'admin.tickets.errors.unknown'
    } finally {
      isLoading.value = false
    }
  }

  const resetPageAndLoadTickets = async () => {
    page.value = 1
    await loadTickets()
  }

  const setStatusFilter = async (value: string) => {
    statusFilter.value = value
    await resetPageAndLoadTickets()
  }

  const goToPage = async (targetPage: number) => {
    const nextPage = Math.min(Math.max(targetPage, 1), totalPages.value || 1)
    if (nextPage === page.value) return

    page.value = nextPage
    await loadTickets()
  }

  void loadTickets()

  return {
    tickets,
    total,
    page,
    pageSize,
    totalPages,
    statusFilter,
    isLoading,
    errorKey,
    loadTickets,
    setStatusFilter,
    goToPage,
  }
}
