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

export interface ClientRecommend {
  claudeCode: ClientToolRecommend
  codex: ClientToolRecommend
}

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
  updatedAt: string | null
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
}

export interface ClientGroup {
  id: string
  name: string
  platform: string | null
  multiplier: number | null
}
