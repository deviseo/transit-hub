export type LeaderboardPeriod = 'today' | '7d' | '30d'

export interface LeaderboardRow {
  rank: number
  userId: string
  email: string
  requests: number
  totalTokens: number
  actualCost: number
}

export interface LeaderboardResponse {
  startDate: string
  endDate: string
  timezone: string
  sortBy: string
  limit: number
  rows: LeaderboardRow[]
}

export interface LeaderboardDateRange {
  startDate: string
  endDate: string
}

export interface LeaderboardEmbedConfig {
  embedToken: string
  sub2apiSourceOrigin: string
  createdAt: string
  updatedAt: string
}

export interface CreateLeaderboardEmbedSessionRequest {
  embedToken: string
  sub2apiToken: string
  srcHost: string
  userId: string
}

export interface CreateLeaderboardEmbedSessionResponse {
  sessionToken: string
  expiresIn: number
}
