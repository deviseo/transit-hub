export type MassEmailUserStatus = 'active' | 'disabled' | 'inactive' | 'banned'

export type MassEmailUserRole = 'admin' | 'user'

export type MassEmailUserSortBy = 'email' | 'username' | 'status' | 'role' | 'created_at'

export type MassEmailSortOrder = 'asc' | 'desc'

export type MassEmailBatchStatus = 'queued' | 'running' | 'completed' | 'completed_with_errors' | 'failed' | 'cancelled' | 'cancelling'

export type MassEmailItemStatus = 'pending' | 'sending' | 'sent' | 'failed' | 'uncertain' | 'cancelled'

export type MassEmailSelectionMode = 'selected' | 'all'

export interface MassEmailUser {
  id: string
  email: string
  status: string
  role: string
  name?: string | null
  username?: string | null
  createdAt?: string | null
  updatedAt?: string | null
}

export interface MassEmailUsersQuery {
  page: number
  pageSize: number
  status: string
  role: string
  search: string
  sortBy: MassEmailUserSortBy
  sortOrder: MassEmailSortOrder
  timezone: string
}

export interface PaginatedMassEmailUsersResponse {
  items: MassEmailUser[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface MassEmailBatchFilters {
  status: string
  role: string
  search?: string
}

export interface CreateMassEmailBatchRequest {
  templateId: string
  selectionMode: MassEmailSelectionMode
  userIds?: string[]
  filters: MassEmailBatchFilters
  requestId: string
}

export interface MassEmailBatch {
  id: string
  requestId: string
  templateId: string
  templateName: string
  templateSubject: string
  selectionMode: MassEmailSelectionMode | string
  filters: MassEmailBatchFilters
  status: MassEmailBatchStatus | string
  recipientCount: number
  skippedCount: number
  sentCount: number
  failedCount: number
  uncertainCount: number
  cancelledCount: number
  createdAt: string
  updatedAt: string
  startedAt?: string | null
  finishedAt?: string | null
  cancelledAt?: string | null
}

export interface PaginatedMassEmailBatchesResponse {
  items: MassEmailBatch[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface MassEmailBatchItem {
  id: string
  batchId: string
  upstreamUserId: string
  recipientEmail: string
  username: string
  status: MassEmailItemStatus | string
  errorKey?: string | null
  createdAt?: string | null
  updatedAt?: string | null
  claimedAt?: string | null
  sentAt?: string | null
  finishedAt?: string | null
}

export interface PaginatedMassEmailBatchItemsResponse {
  items: MassEmailBatchItem[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}
