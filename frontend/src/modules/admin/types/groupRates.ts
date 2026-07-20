export interface GroupRate {
  siteId: string
  siteName: string
  groupId: string
  groupName: string
  type: string | null
  platform: string | null
  mapped: boolean
  connected?: boolean
  pricingMapped?: boolean
  deleted: boolean
  upstreamMultiplier?: number | null
  rechargeRate?: number | null
  currentMultiplier: number | null
  delta: number | null
  deltaPercent: number | null
  updatedAt: string | null
}

export interface GroupRatesQuery {
  page: number
  search: string
  type: string
  platform: string
  status: GroupRateStatusFilter
  sort: GroupRateSort
}

export type GroupRateStatusFilter = 'all' | 'mapped' | 'unmapped' | 'deleted'
export type GroupRateSort = 'multiplierAsc' | 'multiplierDesc' | 'siteNameAsc' | 'groupNameAsc'

export interface GroupRateStatusCounts {
  all: number
  mapped: number
  unmapped: number
  deleted: number
}

export interface PaginatedGroupRatesResponse {
  items: GroupRate[]
  total: number
  page: number
  pageSize: number
  totalPages: number
  types: string[]
  platforms: string[]
  statusCounts?: GroupRateStatusCounts
}

export interface GroupRateHistoryQuery {
  siteId: string
  groupId: string
  groupName: string
  platform: string | null
}

export interface GroupRateRef {
  siteId: string
  groupName: string
}

export interface UpdateGroupRateTypeRequest extends GroupRateRef {
  type: string
}

export interface GroupRateHistoryRow {
  siteId: string
  siteName: string
  groupId: string
  groupName: string
  type: string | null
  platform: string | null
  multiplier: number | null
  currentMultiplier: number | null
  deleted: boolean
  delta: number | null
  deltaPercent: number | null
  createdAt: string | null
}
