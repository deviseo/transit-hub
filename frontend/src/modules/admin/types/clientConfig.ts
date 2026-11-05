// TransitLink 客户端配置（「客户端」页面）。字段与后端 client_config 模块一致。

export interface ClientBrand {
  name: string
  /** https 图片地址，或上传后转成的 data URL。 */
  logo: string
  color: string
  welcome: string
}

export interface ClientToolRecommend {
  /** new-api 是分组名，sub2api 是分组 ID；空表示不指定。 */
  group: string
  /** 空表示不指定（Claude Code 用它的默认模型，Codex 自动选）。 */
  model: string
}

/** TransitLink 支持的工具（「客户端」页面推荐设置的顺序）。键和 TransitLink 里工具的 ID 一致。 */
export const CLIENT_TOOLS = ['claudeCode', 'codex', 'geminiCli', 'grokBuild', 'opencode', 'deepseekHarness'] as const

export type ClientTool = (typeof CLIENT_TOOLS)[number]

export type ClientRecommend = Record<ClientTool, ClientToolRecommend>

export type ClientSupportType = 'telegram' | 'qq' | 'wechat' | 'discord' | 'email' | 'url' | 'other'

export interface ClientSupportLink {
  type: ClientSupportType
  label: string
  value: string
}

export interface ClientConfig {
  enabled: boolean
  /** 专属链接代码；第一次保存后才有。 */
  code: string
  /** 公开配置接口的路径，例如 /api/client/v1/sites/abc；第一次保存后才有。 */
  configPath: string
  hubUrl: string
  siteUrl: string
  platform: string
  brand: ClientBrand
  recommend: ClientRecommend
  notice: string
  noticeUpdatedAt: string | null
  support: ClientSupportLink[]
  /** TransitLink 最低版本（例如 0.2.0），低于它的客户端提示更新；空表示不限制。 */
  minClientVersion: string
  tickets: ClientTicketSettings
  updatedAt: string | null
  /** TransitHub 对外地址和站点地址在同一个域名下；不在的话，用户第一次打开链接会看到确认提示。 */
  linkSameDomain: boolean
}

/** 工单：开启后 TransitLink 里出现「工单」，categories 是用户能选的问题分类。 */
export interface ClientTicketSettings {
  enabled: boolean
  categories: string[]
}

export interface ClientConfigUpdate {
  enabled: boolean
  hubUrl: string
  siteUrl: string
  brand: ClientBrand
  recommend: ClientRecommend
  notice: string
  support: ClientSupportLink[]
  minClientVersion: string
  tickets: ClientTicketSettings
}

export interface ClientGroup {
  id: string
  name: string
  platform: string | null
  multiplier: number | null
}
