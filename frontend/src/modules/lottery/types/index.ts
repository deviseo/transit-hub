export type LotteryCampaignStatus =
  | 'draft'
  | 'scheduled'
  | 'open'
  | 'closed'
  | 'drawing'
  | 'drawn'
  | 'fulfilling'
  | 'completed'
  | 'partial'
  | 'cancelled'

export type LotteryDrawMode = 'manual' | 'scheduled'
export type LotteryPrizeType = 'balance' | 'subscription'
export type LotteryPrizeDeliveryMode = 'sub2api_auto' | 'voucher' | 'manual'
export type LotteryEntryStatus = 'active' | 'withdrawn'
export type LotteryRewardStatusValue =
  | 'pending'
  | 'processing'
  | 'fulfilled'
  | 'retryable_failed'
  | 'manual_attention'
  | 'failed'

export interface LotteryEmbedConfig {
  embedToken: string
  sub2apiSourceOrigin: string
  createdAt: string
  updatedAt: string
}

export interface LotterySubscriptionGroup {
  id: string
  name: string
  multiplier: string
}

export interface LotterySubscriptionGroupsResponse {
  items: LotterySubscriptionGroup[]
}

export interface LotteryPrize {
  id: string
  campaignId: string
  type: LotteryPrizeType
  name: string
  quantity: number
  sortOrder: number
  balanceAmount?: string
  groupId?: string
  groupName?: string
  multiplier?: string
  validityDays?: number
  deliveryMode?: LotteryPrizeDeliveryMode
  manualContact?: string
  voucherCodes?: string[]
  valueMarker: number
}

export interface LotteryPrizeRequest {
  type: LotteryPrizeType
  name: string
  quantity: number
  sortOrder: number
  balanceAmount: string
  groupId: string
  groupName: string
  multiplier: string
  validityDays: number | null
  deliveryMode: LotteryPrizeDeliveryMode
  manualContact: string
  voucherCodes: string[]
}

export interface LotteryCampaignRequest {
  name: string
  description: string
  registrationStart: string
  registrationEnd: string
  drawAt: string
  drawMode: LotteryDrawMode
  publicWinners: boolean
  prizes: LotteryPrizeRequest[]
}

export interface LotteryWinner {
  id: string
  prizeId: string
  entryId?: string
  maskedEmail: string
  prizeSlot: number
}

export interface LotteryRewardStatus {
  id: string
  winnerId: string
  prizeId: string
  status: LotteryRewardStatusValue
  errorKey?: string
  errorDetail?: string
}

export interface LotteryMyRewardStatus {
  id: string
  winnerId: string
  prizeId: string
  status: LotteryRewardStatusValue
  errorKey?: string
  deliveryMode?: LotteryPrizeDeliveryMode
  voucherCode?: string
  manualContact?: string
}

export interface LotteryCampaign {
  id: string
  name: string
  description: string
  status: LotteryCampaignStatus
  registrationStart?: string
  registrationEnd?: string
  drawAt?: string
  drawMode: LotteryDrawMode
  publicWinners: boolean
  seedCommitment?: string
  entrySnapshotHash?: string
  revealedSeed?: string
  algorithmVersion: string
  entryCount: number
  winnerCount: number
  prizes: LotteryPrize[]
  entries?: LotteryEntry[]
  winners?: LotteryWinner[]
  rewardStatuses?: LotteryRewardStatus[]
  myEntry?: LotteryEntry
  myWinner?: LotteryWinner
  myRewardStatus?: LotteryMyRewardStatus
  createdAt: string
  updatedAt: string
}

export interface LotteryEntry {
  id: string
  campaignId: string
  maskedEmail: string
  receiptHash: string
  status: LotteryEntryStatus
  createdAt: string
}

export interface LotteryAuditLog {
  ID: string
  CampaignID: string
  UserID: string
  AdminAccountID: string
  ActorType: string
  ActorID: string
  Event: string
  Detail: Record<string, unknown> | null
  CreatedAt: string
}

export interface LotteryCampaignsResponse {
  items: LotteryCampaign[]
}

export interface LotteryEntriesResponse {
  items: LotteryEntry[]
}

export interface LotteryAuditResponse {
  items: LotteryAuditLog[]
}

export interface LotteryOkResponse {
  ok: boolean
}
