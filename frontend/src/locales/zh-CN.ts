export default {
  brand: {
    name: 'TransitHub',
    logoAlt: 'TransitHub 徽标'
  },
  nav: {
    features: '核心特性',
    integrations: '生态集成',
    documentation: '开发文档',
    pricing: '价格方案',
    signIn: '登录',
    getStarted: '立即开始'
  },
  hero: {
    badge: 'TransitHub 2.0 震撼发布',
    title: '终极版',
    highlight: 'API 流量网关',
    subtitle: '统一接管你的 NewAPI 实例，轻松管理密钥并智能调度流量。专为现代 AI 基础设施而生。',
    startBtn: '立即开始构建',
    docBtn: '查阅开发文档'
  },
  features: {
    title: '为性能与规模而设计',
    subtitle: '跨分布式网络管理海量 API 流量所需的一切，全部打包在这个极具美感的控制台中。',
    items: {
      sync: {
        title: '多实例自动同步',
        desc: '在多个 NewAPI 实例间无缝同步，零停机时间，并自动解决配置冲突。'
      },
      fallback: {
        title: '智能降级调度',
        desc: '智能路由与自动降级机制确保即使单个供应商宕机，你的 API 请求也绝不失败。'
      },
      observe: {
        title: '全局可观测性',
        desc: '在全球范围内实时监控你所有的 API 密钥状态、额度消耗以及延迟指标。'
      },
      selfhost: {
        title: '自托管就绪',
        desc: '随处部署。全面支持 Docker、Kubernetes 以及裸金属 VPS 安装。'
      }
    }
  },
  cta: {
    title: '准备好接管一切了吗？',
    subtitle: '加入成千上万使用 TransitHub 驱动其 API 基础设施的开发者行列。今天就免费开始吧。',
    deployBtn: '立即部署',
    salesBtn: '联系销售'
  },
  footer: {
    rights: 'TransitHub 运维团队。保留所有权利。'
  },
  auth: {
    backToHome: '返回主页',
    login: {
      title: '欢迎回来',
      subtitle: '输入您的邮箱和密码登录 TransitHub',
      email: '邮箱',
      emailPlaceholder: "name{'@'}example.com",
      password: '密码',
      passwordPlaceholder: '输入密码',
      submit: '登录',
      submitting: '登录中...',
      success: '登录成功，正在打开管理后台...',
      errors: {
        login: '登录失败，请检查邮箱和密码后重试。'
      },
      noAccount: '还没有账号？',
      registerLink: '去注册'
    },
    register: {
      title: '创建账号',
      subtitle: '输入您的信息以注册 TransitHub',
      email: '邮箱',
      emailPlaceholder: "name{'@'}example.com",
      password: '密码',
      passwordPlaceholder: '设置密码',
      code: '验证码',
      codePlaceholder: '输入 6 位验证码',
      sendCode: '发送验证码',
      sendingCode: '发送中...',
      codeSent: '已发送',
      codeSentSuccess: '验证码已发送，请使用 {code} 完成注册。',
      submit: '注册',
      submitting: '注册中...',
      success: '注册成功，正在打开管理后台...',
      errors: {
        codeRequest: '验证码发送失败，请检查邮箱后重试。',
        register: '注册失败，请检查验证码后重试。'
      },
      hasAccount: '已经有账号？',
      loginLink: '去登录'
    },
    errors: {
      emailRequired: '请先输入邮箱地址。',
      invalidRegister: '请输入邮箱、密码和验证码。',
      invalidLogin: '请输入邮箱和密码。',
      invalidCode: '验证码错误或已过期。',
      emailExists: '该邮箱已注册。',
      invalidCredentials: '邮箱或密码不正确。',
      unauthorized: '登录状态已过期，请重新登录后继续。',
      registrationDisabled: '当前部署已关闭公开注册，请使用管理员账号登录。',
      network: '网络异常，请检查连接后重试。',
      unknown: '操作失败，请稍后重试。'
    }
  },
  admin: {
    layout: {
      toggleLanguage: '切换语言',
      toggleTheme: '切换主题',
      userProfile: '用户资料',
      switchWorkspace: '切换工作区',
      skipToContent: '跳转到主要内容',
      openNavigation: '打开导航',
      closeNavigation: '关闭导航'
    },
    menu: {
      dashboard: '仪表盘',
      upstream: '上游管理',
      groupRates: '分组倍率',
      settings: '系统设置',
      userActivity: '用户分析',
      sales: '销售分析',
      profit: '利润分析',
      massEmail: '群发邮件',
      tickets: '工单',
      client: '客户端',
      signOut: '退出登录'
    },
    adminAccounts: {
      title: '选择工作区',
      subtitle: '选择一个管理员工作区以继续，或添加新的工作区。',
      empty: '暂无工作区，添加第一个工作区开始使用。',
      currentLabel: '当前工作区',
      addWorkspace: '添加工作区',
      addWorkspaceHint: '连接新的站点管理员账号',
      creating: '正在创建工作区...',
      delete: {
        actionLabel: '删除工作区 {name}',
        title: '删除 {name}',
        localDataWarning: '此工作区的所有 TransitHub 本地工作区数据将被永久删除，且无法恢复。',
        remoteResourcesRetained: '远程上游资源和账号会被保留，不会被删除。',
        phraseInstruction: '手动输入下方完全一致的短语以确认：{phrase}',
        inputLabel: '确认短语',
        inputPlaceholder: '请手动输入确认短语',
        cancel: '取消',
        confirm: '删除工作区',
        deleting: '正在删除...',
        cleanupPending: '工作区删除已完成，但本地运行时、缓存或文件清理仍在等待处理，系统会继续重试。'
      },
      errors: {
        noCurrentAccount: '请先选择一个工作区。',
        notFound: '工作区不存在。',
        deleteFailed: '删除工作区失败，请重新输入确认短语后再试。',
        request: '操作失败，请稍后重试。',
        network: '网络异常，请检查连接后重试。'
      }
    },
    dashboard: {
      metrics: {
        todayProfit: '今日营收',
        siteBalance: '站点用户总余额',
        todayPurchase: '今日成本',
        netProfit: '今日净利润',
        upstreamBalance: '上游总余额',
        profitMargin: '今日利润率',
        groupCount: '我的分组总数',
        groupCountCaption: '点击查看分组详情'
      },
      charts: {
        title: '数据趋势统计',
        subtitle: '查看连续的营收、站点用户余额、成本、净利润与上游总余额走势。',
        trendTitle: '{metric}趋势'
      },
      period: {
        label: '统计周期',
        week: '周',
        month: '月'
      },
      delta: {
        vsPrev: '较前一日',
        percentagePoints: '{value} 个百分点'
      },
      common: {
        unavailable: '暂无'
      },
      performance: {
        title: '经营表现',
        subtitle: '营收、成本与净利润的同期变化',
        periodRevenue: '周期营收',
        periodCost: '周期成本',
        periodProfit: '周期净利润',
        chartAria: '营收、成本与净利润组合趋势图'
      },
      capital: {
        title: '资金安全',
        subtitle: '余额覆盖与成本续航',
        siteBalance: '站点用户余额',
        upstreamBalance: '上游可用余额',
        coverage: '余额覆盖率',
        coverageHint: '上游余额相对于站点用户余额的覆盖比例',
        runway: '预计续航',
        runwayHint: '按当前周期日均成本估算',
        runwayValue: '{value} 天'
      },
      groups: {
        title: '分组贡献',
        subtitle: '今日营收最高的分组',
        total: '{count} 个分组',
        amount: '今日营收',
        topThreeShare: '前三分组营收占比',
        empty: '暂无分组营收数据。',
        loadError: '分组贡献数据暂时无法加载。',
        chartAria: '今日分组营收贡献排名图'
      },
      attention: {
        title: '需要关注',
        subtitle: '上游余额与连接异常',
        upstreamTitle: '上游余额待处理',
        upstreamDescription: '存在余额未知或连接异常的上游站点',
        allClearTitle: '当前没有明确异常',
        allClearDescription: '上游余额与连接暂未发现需要处理的问题。',
        unavailableTitle: '运行状态暂时不可用',
        unavailableDescription: '核心经营数据不受影响，请稍后重试上游状态。',
        refresh: '刷新运行状态',
        partialLoadError: '部分运行数据加载失败'
      },
      loading: '正在加载指标数据...',
      loadError: '加载仪表盘指标失败。',
      retry: '重试',
      dataStatus: {
        refreshing: '正在刷新最新数据',
        updatedAt: '更新于 {time}',
        waiting: '等待首次更新',
        failed: '刷新失败，当前显示上次数据',
        refresh: '刷新仪表盘数据'
      },
      loadingModal: {
        title: '正在加载仪表盘数据',
        progress: '加载进度 {progress}%',
        steps: {
          auth: '正在验证管理员身份...',
          data: '正在加载实时指标与历史趋势...',
          done: '正在整理数据并渲染页面...'
        }
      },
      groupUsage: {
        title: '今日营收分组明细',
        subtitle: '共 {count} 个分组，合计 {total}',
        close: '关闭',
        empty: '暂无分组用量数据。',
        loadError: '加载分组用量失败。',
        retry: '重试',
        columns: {
          groupName: '分组名称',
          amount: '今日金额'
        },
        sort: {
          desc: '金额从高到低',
          asc: '金额从低到高'
        }
      },
      cost: {
        syncedAt: '按 {count} 个上游站点计算 · 最早同步于 {time}',
        staleSites: '{count} 个上游站点今天还没有同步成功，未计入（已在后台重试）'
      },
      pricing: {
        title: '我的站点售价',
        subtitle: '把营收和站点用户余额换算成人民币',
        label: '站点余额每 1 美元（1 个单位）售价',
        unit: '元 / $1',
        helpRevenue: '今日营收、站点用户总余额 = 站点金额 × 售价；默认 1，即 1 美元按 1 元计。',
        helpExample: '例如你按 ¥0.8 卖出 $1 额度就填 0.8；按 ¥7.2 卖 $1 就填 7.2。',
        helpCost: '上游成本按各上游站点的「充值倍率」（每 $1 上游额度的实际进货价）换算，两边都是人民币，净利润与利润率才准确。',
        helpHistory: '修改后从今天起生效，之前定稿的历史数据不会重算。',
        cardNote: '按 ¥{price}/$1 换算',
        basis: '营收与站点余额按售价 ¥{price}/$1 换算，成本按各上游「充值倍率」换算。',
        edit: '修改售价',
        save: '保存',
        cancel: '取消',
        close: '关闭',
        errors: {
          invalid: '请输入大于 0、不超过 10000 的售价。',
          request: '售价设置保存失败，请稍后重试。'
        }
      },
      upstreamKeyUsage: {
        title: '今日成本明细',
        subtitle: '共 {count} 个 key，合计 {total}',
        liveNote: '明细于 {time} 实时逐个 key 查询；卡片按各上游站点最近一次同步的账号合计计算。两者查询时间不同，且今天已删除的 key 不在明细中，合计可能略有差异。',
        close: '关闭',
        empty: '暂无今日消费的 key。',
        loadError: '加载今日成本明细失败，请检查上游站点连接后重试。',
        partialWarning: '{failed}/{total} 个上游站点暂时无法读取，当前合计仅包含成功站点。',
        retry: '重试',
        errors: {
          unavailable: '所有上游站点的 Key 用量暂时都无法读取，请检查站点连接或登录凭证。'
        },
        columns: {
          siteName: '上游站点',
          keyName: 'Key 名称',
          groupName: '分组',
          amount: '今日金额'
        },
        sort: {
          desc: '金额从高到低',
          asc: '金额从低到高'
        }
      },
      upstreamBalanceBreakdown: {
        title: '上游总余额明细',
        subtitle: '共 {count} 个站点，合计 {total}',
        close: '关闭',
        empty: '暂无上游站点余额数据。',
        loadError: '加载上游余额明细失败。',
        retry: '重试',
        unknownBalance: '未知余额',
        neverSynced: '尚未同步',
        columns: {
          siteName: '上游站点',
          status: '状态',
          lastSyncedAt: '最近同步时间',
          balance: '余额'
        },
        sort: {
          desc: '余额从高到低',
          asc: '余额从低到高'
        }
      },
      balanceFilter: {
        title: '余额筛选条件',
        subtitle: '配置统计站点用户总余额时的过滤规则。',
        close: '关闭',
        excludeAdmin: '排除管理员账户',
        excludeAdminHelp: '统计时不包含 admin 角色用户的余额。',
        excludeBalances: '排除特定余额值',
        excludeBalancesHelp: '余额等于以下数值的用户将不纳入统计。',
        addPlaceholder: '输入要排除的余额值',
        add: '添加',
        cancel: '取消',
        save: '保存',
        saving: '保存中...',
        loadError: '加载筛选配置失败。',
        saveError: '保存筛选配置失败。'
      },
      adminAuth: {
        loggedInAs: '当前 admin：{identity}',
        logout: '退出当前 admin 账户',
        notLoggedIn: '尚未登录 admin 账户',
        login: '登录 admin 账户',
        expiresAt: '过期',
        timeUnknown: '未知',
        updateCredentials: '更新管理员凭证',
        updatingCredentials: '正在更新...',
        logoutConfirm: {
          title: '退出当前 admin 账户？',
          description: '退出后需要重新登录并校验 admin 身份才能查看仪表盘数据。',
          confirm: '确认退出',
          cancel: '取消'
        },
        dataLocked: {
          title: '请先登录 admin 账户',
          description: '登录并校验通过具备 admin 权限的站点账号后，才能查看仪表盘统计数据。'
        },
        modal: {
          title: '登录 admin 账户',
          subtitle: '仪表盘需要一个具备 admin 权限的站点账户。',
          close: '关闭',
          platformLabel: '选择平台',
          platform: {
            sub2api: 'Sub2API',
            newapi: 'New-API'
          },
          comingSoon: '即将支持',
          newApiPasswordOnly: 'New-API 仅支持账号密码登录。',
          siteUrlLabel: '站点地址（域名或 IP）',
          siteUrlPlaceholder: '如 https://sub.example.com 或 http://1.2.3.4:5555',
          methodLabel: '登录方式',
          method: {
            password: '邮箱密码',
            token: 'RT + AT',
            admin_key: 'Admin Key'
          },
          fields: {
            emailPlaceholder: '管理员邮箱',
            usernamePlaceholder: '管理员账号',
            passwordPlaceholder: '管理员密码',
            accessTokenPlaceholder: 'Access Token（可选，可留空）',
            refreshTokenPlaceholder: 'Refresh Token（与 Access Token 至少填写一个）',
            tokenTypePlaceholder: 'Token Type（可选，默认 Bearer）',
            tokenHelp: 'Access Token 与 Refresh Token 至少填写一个；提供 Refresh Token 时系统会自动刷新临期凭证。',
            sub2apiAdminKeyPlaceholder: 'Sub2API Admin API Key',
            sub2apiAdminKeyHelp: '该 Key 仅通过 x-api-key 发送到 Sub2API 管理接口。',
            newApiAdminKeyPlaceholder: 'New-API 访问令牌（nap_ 开头）或系统访问令牌',
            newApiAdminKeyHelp: '推荐使用新版 New-API 的个人访问令牌（PAT）：用 Root 账号创建，勾选 profile:read、user:read、log:read、group:read、option:read、option:write、channel:read、channel:write 权限（需要由 TransitHub 自动创建或删除渠道时再加 channel:sensitive_write），并设置较长有效期。旧版系统访问令牌需同时填写用户 ID。',
            userIdPlaceholder: '用户 ID（可选，旧版系统访问令牌需要）',
            newApiPasswordHelp: '新版 New-API 的密码登录最长保持 30 天，到期需重新登录；开启两步验证的账号无法自动登录，请改用访问令牌。'
          },
          submit: '登录并校验',
          submitting: '校验中...'
        },
        errors: {
          request: 'admin 登录请求失败，请稍后重试。',
          missingCredentials: '请填写站点地址和所选登录方式的必填项。',
          invalidUrl: '站点地址无效，请填写正确的域名或 IP 后重试。',
          adminOnly: '该账户不是 admin 或鉴权失败，请确认凭证后重试。',
          network: '网络或跨域请求失败，请检查站点地址。',
          platformUnsupported: '不支持的平台类型，请选择 Sub2API 或 New-API。',
          unknown: 'admin 登录时发生未知错误。',
          reloginRequired: '管理员身份校验失败，请重新登录。',
          verificationRequired: '该账号开启了两步验证，无法自动登录，请改用访问令牌登录。',
          tokenRejected: '访问令牌校验失败：旧版 New-API 的系统访问令牌需要同时填写用户 ID；新版请确认令牌有效、未过期且勾选了所需权限。'
        }
      }
    },
      upstream: {
        searchPlaceholder: '搜索站点名称...',
        addSite: '新增站点',
        summary: '已连接 {connected} / {total} 个上游站点',
        refresh: {
          action: '刷新数据',
          refreshing: '刷新中...',
          countdown: '{seconds} 秒后刷新',
          disabled: '未开启自动刷新'
        },
      modal: {
        title: '新增上游站点',
        editTitle: '修改上游站点',
        cancel: '取消',
        submit: '确认新增',
        updateSubmit: '保存修改',
        submitting: '连接中...',
        form: {
          siteName: '站点名称',
          siteNamePlaceholder: '输入站点名称',
          siteUrl: '站点 URL',
          siteUrlPlaceholder: '输入完整的站点地址，如 https://api.example.com',
          platform: '选择平台',
          platforms: {
            auto: '自动识别',
            sub2api: 'Sub2API',
            newapi: 'New-API'
          },
          authMode: '认证方式',
          authModes: {
            password: '账号密码登录',
            passwordHelp: '使用站点账号密码登录，并自动保存会话。',
            token: 'Access Token / Refresh Token',
            tokenHelp: '适用于 Cloudflare 或二次验证导致账号密码无法直连的 Sub2API 站点。',
            userKey: '访问令牌',
            userKeyHelp: '使用 New-API 个人访问令牌（PAT）或旧版系统访问令牌，适合开启了两步验证或需要长期保持登录的账号。',
            passwordHelpNewApi: '使用站点账号密码登录。新版 New-API 的登录最长保持 30 天，开启两步验证的账号请改用访问令牌。'
          },
          account: '登录账号',
          accountPlaceholder: '输入账号',
          password: '登录密码',
          passwordPlaceholder: '输入密码',
          passwordEditPlaceholder: '不修改密码请留空',
          passwordEditHelp: '留空时不会重新登录，也不会修改已保存的登录会话；填写新密码后才会重新登录并更新会话。',
          accessToken: 'Access Token',
          accessTokenPlaceholder: '粘贴 access_token，可留空并仅提供 refresh_token',
          refreshToken: 'Refresh Token',
          refreshTokenPlaceholder: '粘贴 refresh_token，系统会先刷新一次获取最新过期时间',
          tokenType: 'Token Type',
          tokenTypePlaceholder: '默认 Bearer',
          tokenHelp: '如果提供 refresh_token，系统会优先调用刷新接口换取新的 access_token 和过期时间。',
          userId: '用户 ID（可选）',
          userIdPlaceholder: '旧版系统访问令牌需要填写所属用户 ID，新版访问令牌可留空',
          userKey: '访问令牌',
          userKeyPlaceholder: '粘贴 New-API 访问令牌（nap_ 开头）或系统访问令牌',
          userKeyHelp: '推荐使用新版个人访问令牌（PAT），勾选 profile:read、usage:read、api_key:read、api_key:reveal 权限（需要由 TransitHub 自动创建上游 Key 时再加 api_key:write），并设置较长有效期。',
          rechargeRate: '充值倍率',
          rechargeRatePlaceholder: '每 $1 上游额度的实际进货价（元），如 1 或 0.5',
          rechargeRateHelp: '必填。填你买 1 美元上游额度实际花了多少元：¥1 买 $1 填 1，打 5 折填 0.5，按美元汇率付费才填汇率（约 7.2）。今日成本 = 上游消费（美元）× 此倍率。',
          remark: '备注',
          remarkPlaceholder: '输入备注信息（可选）'
        }
      },
      currency: {
        usdValue: '{amount} USD',
        cnyValue: '{amount} CNY'
      },
      fields: {
        siteName: '站名',
        siteUrl: '站点 URL',
        platform: '平台',
        balance: '余额',
        todayConsume: '今日消费',
        historyRecharge: '历史充值',
        groupName: '分组名称',
        groupMultiplier: '分组倍率',
        availableGroups: '可用分组',
        viewAvailableGroups: '查看可用分组',
        closeGroupsModal: '关闭',
        dedicatedMultiplierBadge: '专属倍率',
        dedicatedMultiplierTooltip: '该用户命中了 sub2api 专属倍率，业务计算使用右侧倍率。',
        unknownPlatform: '未知类型',
        lastUpdated: '更新时间',
        notSynced: '暂未同步'
      },
      status: {
        connecting: '连接中',
        syncing: '同步中',
        connected: '已连接',
        error: '异常'
      },
      empty: {
        title: '未找到上游站点',
        description: '请调整搜索条件，或新增一个上游站点。'
      },
      delete: {
        action: '删除站点',
        title: '确认删除上游站点？',
        description: '你将删除“{name}”，删除后需要重新添加和登录才能恢复。',
        cancel: '取消',
        confirm: '确认删除'
      },
      action: {
        sync: '刷新',
        syncing: '刷新中',
        edit: '修改站点',
        settings: '站点设置',
        actions: '操作'
      },
      siteSettings: {
        title: '站点预警设置',
        balanceThreshold: '自定义余额预警阈值',
        balanceThresholdHelp: '开启后使用站点专属阈值，关闭则使用全局默认值。',
        balanceThresholdPlaceholder: '输入阈值金额',
        save: '保存',
        saveSuccess: '已保存',
        saving: '保存中...',
        cancel: '取消'
      },
      viewMode: {
        list: '列表模式',
        card: '卡片模式'
      },
      syncStream: {
        syncing: '正在同步...',
        done: '同步完成',
        error: '同步失败',
      },
      errors: {
        invalidUrl: '站点 URL 无效，请检查后重试。',
        network: '无法连接上游站点（网络、代理或 TLS 错误）。请检查站点地址，或在「系统设置 - 网络代理」中调整代理。',
        auth: '登录失败，请检查账号或密码。',
        request: '上游接口请求失败，请稍后重试。',
        invalidResponse: '上游返回内容无法解析。',
        tokenMissing: '登录成功但未返回访问令牌。',
        detect: '无法自动识别平台，请手动选择平台后重试。',
        verificationRequired: '该账号开启了两步验证，无法自动登录。请改用访问令牌方式添加站点。',
        newApiTokenRejected: '访问令牌校验失败：旧版 New-API 的系统访问令牌需要同时填写用户 ID；新版请确认令牌有效、未过期且勾选了所需权限。',
        unknown: '连接上游站点时发生未知错误。'
      }
    },
    groupRates: {
      badge: '倍率同步记录',
      title: '分组倍率',
      subtitle: '查看各上游站点分组倍率及最近变动，并追踪历史倍率记录。',
      common: {
        placeholder: '—',
        allTypes: '全部类型',
        allPlatforms: '全部平台',
        unknown: '未知'
      },
      platforms: {
        newapi: 'New-API',
        sub2api: 'Sub2API'
      },
      summary: {
        totalLabel: '分组倍率总数',
        updatedLabel: '已同步记录'
      },
      table: {
        title: '倍率列表',
        description: '列表顺序与后端返回保持一致。'
      },
      fields: {
        siteName: '站点名称',
        groupName: '分组名称',
        type: '分组类型',
        platform: '站点平台',
        currentMultiplier: '当前倍率',
        effectiveMultiplier: '换算后成本倍率',
        multiplierFormula: '上游 {upstream} × 充值系数 {recharge}',
        delta: '涨跌幅',
        updatedAt: '更新时间'
      },
      actions: {
        refresh: '刷新数据',
        viewHistory: '查看历史',
        viewHistoryForRate: '查看 {site} · {group} 的涨跌幅历史，当前涨跌幅 {delta}',
        closeHistory: '关闭历史'
      },
      filters: {
        searchLabel: '搜索',
        searchPlaceholder: '搜索站点或分组...',
        typeLabel: '分组类型',
        platformLabel: '站点平台'
      },
      sort: {
        label: '排序',
        multiplierAsc: '倍率从低到高',
        multiplierDesc: '倍率从高到低',
        siteNameAsc: '站点名称 A-Z',
        groupNameAsc: '分组名称 A-Z'
      },
      tabs: {
        all: '全部',
        deleted: '已删除'
      },
      pagination: {
        previous: '上一页',
        next: '下一页',
        currentPage: '第 {page} / {totalPages} 页',
        total: '共 {total} 条',
        pageSize: '每页 {pageSize} 条'
      },
      status: {
        loading: '正在加载分组倍率...',
        deleted: '已删除'
      },
      empty: {
        title: '暂无分组倍率',
        description: '同步上游站点后，这里会显示分组倍率数据。',
        filteredTitle: '当前条件下没有记录',
        filteredDescription: '切换状态或调整搜索、类型和平台筛选。'
      },
      history: {
        title: '倍率历史',
        titleWithGroup: '{site} · {group} 倍率历史',
        subtitle: '平台：{platform}',
        loading: '正在加载历史记录...',
        emptyTitle: '暂无历史记录',
        emptyDescription: '该站点分组暂未返回倍率历史。',
        multiplier: '倍率',
        delta: '涨跌幅',
        createdAt: '记录时间'
      },
      format: {
        multiplier: '{value}x',
        deltaMultiplier: '{value}x'
      },
      errors: {
        network: '网络或 CORS 请求失败，请检查接口地址与跨域配置。',
        request: '分组倍率接口请求失败，请稍后重试。',
        unknown: '加载分组倍率时发生未知错误。'
      }
    },
    massEmail: {
      common: {
        placeholder: '-'
      },
      filters: {
        search: '搜索用户',
        searchPlaceholder: '输入邮箱关键词',
        noSearch: '未设置搜索词',
        status: '用户状态',
        role: '角色',
        allStatuses: '全部状态',
        allRoles: '全部角色'
      },
      template: {
        label: '邮件模板',
        placeholder: '选择模板',
        noSubject: '尚未选择主题'
      },
      selection: {
        title: '收件人选择',
        count: '跨页已选择 {count} 个',
        selectPage: '选择当前页全部用户',
        selectUser: '选择 {email}'
      },
      users: {
        title: '收件人'
      },
      fields: {
        email: '邮箱',
        role: '角色',
        status: '状态',
        createdAt: '创建时间',
        actions: '操作'
      },
      roles: {
        user: '普通用户',
        admin: '管理员'
      },
      userStatus: {
        active: '正常',
        disabled: '已禁用',
        inactive: '未激活',
        banned: '已封禁'
      },
      actions: {
        search: '搜索',
        clearSearch: '清空搜索',
        refresh: '刷新',
        clearSelection: '清空',
        sendSelected: '发送所选',
        sendPage: '发送当前页',
        sendFilter: '发送当前筛选',
        sendRow: '发送',
        cancelBatch: '取消',
        closeConfirm: '关闭确认框',
        openBatches: '批次',
        openBatchDetail: '详情',
        previewTemplate: '预览'
      },
      status: {
        loadingUsers: '正在加载收件人...',
        loadingItems: '正在加载发送结果...'
      },
      batchStatus: {
        queued: '排队中',
        running: '发送中',
        completed: '已完成',
        completed_with_errors: '已完成但有错误',
        failed: '失败',
        cancelled: '已取消',
        cancelling: '取消中'
      },
      itemStatus: {
        pending: '待发送',
        sending: '发送中',
        sent: '已发送',
        failed: '失败',
        uncertain: '结果不确定',
        cancelled: '已取消'
      },
      empty: {
        usersTitle: '当前筛选下没有收件人',
        usersDescription: '调整搜索关键词、状态或角色筛选后重试。',
        batches: '暂无群发邮件批次。',
        detail: '选择一个批次查看收件人发送结果。'
      },
      pagination: {
        total: '共 {total} 个',
        pageSize: '每页 {pageSize} 条',
        currentPage: '第 {page} / {totalPages} 页',
        previous: '上一页',
        next: '下一页'
      },
      batches: {
        title: '批次',
        active: '{count} 个进行中',
        progress: '已处理 {done}/{total}，{percent}%',
        close: '关闭批次列表'
      },
      detail: {
        title: '批次详情',
        recipients: '共 {total} 个收件人结果',
        close: '关闭批次详情'
      },
      preview: {
        title: '模板预览',
        close: '关闭预览',
        iframeTitle: '邮件模板预览'
      },
      summary: {
        sent: '已发送',
        failed: '失败',
        uncertain: '不确定',
        cancelled: '已取消'
      },
      confirm: {
        selectedTitle: '向所选收件人发送邮件？',
        selectedDescription: '将为已选择的 {count} 个收件人创建群发邮件批次。',
        allTitle: '向当前筛选下全部收件人发送邮件？',
        allDescription: '将为当前筛选匹配的 {count} 个收件人创建群发邮件批次。',
        recipients: '收件人：{count} 个',
        template: '模板：{name}',
        filters: '筛选：{status}，{role}，搜索：{search}',
        cancel: '取消',
        submit: '创建批次'
      },
      success: {
        created: '群发邮件批次已创建。',
        cancelled: '已请求取消批次。'
      },
      errors: {
        network: '网络或 CORS 请求失败，请检查接口地址与跨域配置。',
        request: '群发邮件请求失败，请稍后重试。',
        templates: '加载邮件模板失败。',
        unknown: '加载群发邮件数据时发生未知错误。',
        invalidRequest: '群发邮件请求无效。',
        invalidSelection: '请至少选择一个有效收件人。',
        templateNotFound: '所选邮件模板不存在。',
        smtpNotReady: 'SMTP 设置尚未就绪。',
        upstreamAuth: '上游管理员鉴权失败。',
        upstreamRequest: '上游请求失败。',
        notFound: '群发邮件批次不存在。',
        invalidState: '当前批次状态无法执行该操作。',
        persistence: '群发邮件数据保存失败。',
        sendFailed: '邮件发送失败。',
        activeBatchExists: '当前工作区已有一个进行中的群发批次，请取消或等待完成后再创建。',
        recipientLimitReached: '本次选择超过 10,000 个收件人上限，请缩小筛选范围后重试。',
        itemGeneric: '该收件人发送失败，请稍后查看批次获取最新详情。',
        segmentUnavailable: '按活跃分群群发不可用。',
        segmentNotSynced: '还没有同步过用户数据：登录自己站点后会在后台自动同步，也可以在「用户分析」页面马上同步。',
        invalidPublicUrl: '退订链接地址无效，请填写 http(s) 开头的完整地址。',
        invalidSuppression: '请输入有效的邮箱地址（单次最多 1000 个）。',
        suppressed: '收件人已退订，未发送。'
      }
    },
    userActivity: {
      subtitle: '同步当前工作区站点（sub2api / new-api）的用户与每日用量到本地，分析活跃度、未活跃时长以及日 / 周 / 月环比和同比，并可按筛选结果群发邮件。',
      common: {
        placeholder: '—'
      },
      platforms: {
        sub2api: 'sub2api',
        newapi: 'new-api'
      },
      sync: {
        title: '数据同步',
        never: '尚未同步',
        lastSynced: '{time}同步',
        coverage: '用量覆盖 {range}',
        users: '{count} 个用户',
        running: '正在同步用户与用量…',
        runningBackfill: '正在回补更早的历史用量…',
        failed: '最近一次同步失败：{reason}'
      },
      time: {
        justNow: '刚刚',
        minutesAgo: '{count} 分钟前',
        hoursAgo: '{count} 小时前',
        daysAgo: '{count} 天前'
      },
      actions: {
        refresh: '刷新',
        syncNow: '立即同步',
        startSync: '开始同步',
        goDashboard: '前往仪表盘登录',
        sendEmail: '按筛选群发邮件',
        suppressions: '退订名单（{count}）',
        search: '搜索',
        clearSearch: '清除搜索',
        close: '关闭',
        cancel: '取消'
      },
      status: {
        loading: '加载中…'
      },
      empty: {
        title: '还没有用户数据',
        description: '登录自己站点后，系统会在一小时内自动在后台开始同步：用当前工作区的管理员会话读取站点用户列表和每日用量（首次拉取最近 62 天，之后每小时自动增量同步，并逐步回补更早的历史），数据只保存在本地用于分析。也可以点下面的按钮马上开始。',
        runningTitle: '正在首次同步',
        runningDescription: '正在读取站点用户和最近 62 天的用量，通常需要 1～3 分钟；完成后页面会自动刷新。'
      },
      money: {
        title: '资金概览',
        methodRecords: '按站点充值记录统计：在线充值、兑换码、管理员调整、邀请返利',
        methodSnapshot: '按两次同步之间用户「余额 + 累计消费」的变化统计（自 {time} 起），包含在线充值、兑换码、管理员调整、签到 / 邀请奖励等，无法区分渠道',
        todayRecharge: '今日入账',
        deduction: '扣减 {value}',
        yesterdayFull: '昨日全天 {value}',
        events: '{count} 笔',
        noRechargeToday: '今天还没有入账',
        noRechargeData: '还没有入账数据，同步后显示',
        notUpToDate: '最近一次同步没能读取入账记录，数据可能不是最新的',
        todayCost: '今日消耗',
        totalBalance: '全站用户余额',
        balanceChange: '较昨日末 {value}',
        usersWithBalance: '有余额 {count} 人',
        lifetimeRecharged: '累计充值 {value}（站点字段，不含管理员调整）',
        rechargeUsers: '今日入账人数',
        lowBalance: '活跃但余额不足 1 美元',
        channels: {
          payment: '在线充值',
          redeem: '兑换码',
          admin_add: '管理员加款',
          admin_deduct: '管理员扣款',
          affiliate: '邀请返利转入',
          credit: '余额增加',
          debit: '余额减少'
        }
      },
      warnings: {
        rechargeFetch: '同步时没能读取站点的入账记录（可能是管理员密钥没有兑换码记录的查看权限或网络问题），活跃与用量数据不受影响，下次同步会重试。',
        rechargeUnsupported: '站点版本较旧，不支持按使用时间读取兑换码记录，暂时无法统计入账。升级 sub2api 后即可使用。',
        rechargeTruncated: '需要读取的入账记录过多，本次没有写入入账数据，下次同步会继续尝试。'
      },
      kpi: {
        totalUsers: '用户总数',
        totalUsersHint: '有邮箱 {withEmail} · 已禁用 {disabled}',
        activeToday: '今日活跃用户',
        todayHint: '请求 {requests} · 消费 {cost}',
        active7d: '近 7 天活跃用户',
        vsPrevious7d: '对比前 7 天',
        active30d: '近 30 天活跃用户',
        vsPrevious30d: '对比前 30 天',
        noUsage: '从未使用',
        noUsageHint: '注册后没有任何用量，点击查看'
      },
      inactive: {
        title: '未活跃分布',
        hint: '不含已禁用用户（共 {count} 人），点击查看名单',
        bucket: '超过 {days} 天未使用'
      },
      periods: {
        day: {
          title: '日环比',
          description: '昨天 vs 前天'
        },
        week: {
          title: '周环比',
          description: '近 7 天 vs 前 7 天'
        },
        month: {
          title: '月环比',
          description: '近 30 天 vs 前 30 天'
        },
        metric: '指标',
        current: '本期',
        sequential: '环比',
        yearOverYear: '同比',
        previousMissing: '本地数据还没有覆盖上一周期，同步回补后显示',
        lastYearMissing: '本地还没有去年同期的数据（sub2api 只保留 90 天用量，需要本地持续同步满一年）'
      },
      metrics: {
        activeUsers: '活跃用户',
        requests: '请求数',
        tokens: 'Token',
        cost: '消费（美元）',
        newUsers: '新注册',
        recharge: '入账（美元）'
      },
      retention: {
        retained: '持续活跃 {count}',
        churned: '流失 {count}',
        gained: '新增活跃 {count}'
      },
      delta: {
        fromZero: '新增'
      },
      trend: {
        range: '趋势时间范围',
        lastDays: '近 {days} 天',
        completeDays: '截至昨天（完整自然日）',
        showTable: '表格',
        showChart: '图表',
        day: '日期',
        activeUsersTitle: '每日活跃用户',
        costTitle: '每日消费（美元）',
        latest: '{day}：{value}',
        usersValue: '{count} 人',
        activeUsersAria: '近 {days} 天每日活跃用户趋势',
        costAria: '近 {days} 天每日消费趋势',
        rechargeTitle: '每日入账（美元）',
        rechargeAria: '近 {days} 天每日入账趋势',
        noData: '无数据（尚未覆盖）'
      },
      users: {
        title: '用户列表',
        subtitle: '「近 N 天」均截至昨天；点击用户查看每日用量与环比。',
        matched: '匹配 {count} 人',
        empty: '没有符合条件的用户'
      },
      filters: {
        segment: '用户分群',
        inactiveDays: '未使用天数',
        status: '状态',
        role: '角色',
        search: '搜索',
        searchPlaceholder: '邮箱、用户名或用户 ID',
        allStatuses: '全部状态',
        allRoles: '全部角色',
        searchSummary: '搜索「{search}」'
      },
      segmentGroups: {
        basic: '全部',
        active: '活跃',
        inactive: '未活跃',
        change: '用量变化',
        money: '资金'
      },
      segments: {
        all: '全部用户',
        active_1d: '24 小时内使用过',
        active_7d: '7 天内使用过',
        active_30d: '30 天内使用过',
        inactive_2d: '超过 2 天未使用',
        inactive_7d: '超过 7 天未使用',
        inactive_14d: '超过 14 天未使用',
        inactive_30d: '超过 30 天未使用',
        inactive_60d: '超过 60 天未使用',
        inactive_90d: '超过 90 天未使用',
        inactive: '自定义未使用天数',
        inactiveCustom: '超过 {days} 天未使用',
        no_usage: '从未使用',
        churned_7d: '近 7 天流失（前 7 天用过）',
        declining: '用量下降（周消费减半）',
        growing: '用量增长（周消费 ≥1.5 倍）',
        recharged_today: '今日有入账',
        low_balance: '活跃但余额不足 1 美元'
      },
      userStatus: {
        active: '正常',
        disabled: '已禁用'
      },
      roles: {
        user: '普通用户',
        admin: '管理员',
        root: '超级管理员'
      },
      fields: {
        user: '用户',
        status: '状态',
        lastUsed: '最近使用',
        costToday: '今日消费',
        cost7d: '近 7 天消费',
        change7d: '周环比',
        cost30d: '近 30 天消费',
        activeDays30d: '30 天活跃天数',
        balance: '余额',
        totalRecharged: '累计充值',
        totalUsed: '累计消费',
        createdAt: '注册时间',
        today: '今日消费',
        requestsCount: '{count} 次请求',
        daysValue: '{count} 天',
        previousValue: '前 7 天 {value}',
        rechargeToday: '今日入账',
        recharge30d: '近 30 天入账'
      },
      lastUsed: {
        today: '今天',
        daysAgo: '{count} 天前',
        longAgo: '很久未使用',
        never: '从未使用'
      },
      detail: {
        identity: '用户 ID {id} · 用户名 {username}',
        trendTitle: '近 90 天每日用量',
        requestsChartAria: '该用户近 90 天每日请求数趋势',
        costChartAria: '该用户近 90 天每日消费趋势',
        rechargeChartAria: '该用户近 90 天每日入账趋势'
      },
      email: {
        title: '按筛选条件群发邮件',
        loading: '正在统计收件人…',
        matched: '匹配用户',
        withEmail: '有邮箱',
        suppressed: '已退订',
        sendable: '预计发送',
        previewHint: '收件人来自本地同步的用户快照；无效或重复的邮箱会在创建批次时跳过，发送进度可在「群发邮件」页面查看。',
        template: '邮件模板',
        templatePlaceholder: '请选择模板',
        noTemplates: '还没有邮件模板，请先在「系统设置 - 邮件模板」中创建。',
        subject: '主题：',
        unsubscribe: '附带退订链接',
        unsubscribeHint: '每封邮件末尾会自动附上该收件人的退订链接；也可以在模板中写入 {placeholder} 指定位置。',
        unsubscribePlaceholderFound: '模板中的 {placeholder} 会替换为每个收件人自己的退订链接。',
        publicBaseUrl: 'TransitHub 接口公网地址',
        publicBaseUrlHint: '收件人点击退订链接时访问该地址下的 /email-unsubscribe，请确认外网可以访问。',
        privateUrlWarning: '这看起来是本机或内网地址，收件人在外网可能打不开退订链接。',
        submit: '创建群发批次（{count} 人）',
        created: '已创建群发批次：{count} 个收件人，跳过 {skipped} 个（无效、重复或已退订）。',
        viewProgress: '查看发送进度'
      },
      suppressions: {
        title: '退订名单',
        description: '名单中的邮箱不会再收到群发邮件（对所有群发方式生效）。收件人点击邮件中的退订链接后会自动加入。',
        addLabel: '手动添加邮箱',
        addPlaceholder: '每行一个，或用逗号分隔',
        add: '加入退订名单',
        searchPlaceholder: '搜索邮箱',
        email: '邮箱',
        reason: '来源',
        createdAt: '加入时间',
        empty: '退订名单为空',
        remove: '移出名单',
        removeAria: '将 {email} 移出退订名单',
        removeConfirm: '确定将 {email} 移出退订名单？移出后该邮箱会重新收到群发邮件。',
        added: '已加入 {added} 个邮箱，忽略无效地址 {invalid} 个。',
        removed: '已将 {email} 移出退订名单。',
        reasons: {
          unsubscribe: '收件人退订',
          manual: '手动添加'
        }
      },
      pagination: {
        total: '共 {total} 条',
        previous: '上一页',
        next: '下一页'
      },
      errors: {
        network: '网络请求失败，请检查 TransitHub 服务是否可用。',
        request: '用户分析数据请求失败，请稍后重试。',
        authRequired: '需要当前工作区站点的管理员会话：请先在仪表盘登录 sub2api / new-api 管理员账号。',
        platformUnsupported: '当前工作区的站点类型不支持用户分析（仅支持 sub2api 和 new-api）。',
        syncInProgress: '同步正在进行中，请稍后再试。',
        usersFetch: '读取站点用户列表失败，请确认管理员会话有效。',
        usageFetch: '读取站点用量数据失败。sub2api 需要管理员会话；new-api 需要管理员权限（访问令牌需包含 log:read），且站点开启了数据看板统计。',
        interrupted: '上次同步被服务重启或超时打断，会在后台自动继续，无需操作。',
        invalidFilter: '筛选条件无效。',
        notFound: '用户不存在或已从站点删除。',
        tooManyUsers: '站点用户超过 50,000 人，暂不支持同步。',
        tooManyRecipients: '筛选结果超过 10,000 个收件人，请缩小筛选范围。',
        notSynced: '还没有同步过用户数据，请先同步。'
      }
    },
    sales: {
      title: '销售分析',
      subtitle: '入账结构、付费用户、转化与复购、收入集中度和入账排行。数据来自「用户分析」同步到本地的数据，只读。',
      common: {
        placeholder: '—'
      },
      source: {
        lastSynced: '数据来自用户分析同步 · 最近同步 {time}',
        neverSynced: '尚未同步用户数据',
        basis: '金额按 ¥{price}/$1 换算（与仪表盘营收口径一致）'
      },
      range: {
        label: '统计区间',
        lastDays: '近 {days} 天',
        from: '开始日期',
        to: '结束日期',
        apply: '应用',
        current: '统计区间：{range}（截至昨日的完整自然日，或自定义区间）',
        previous: '对比上一周期：{range}'
      },
      actions: {
        refresh: '刷新',
        changePrice: '修改售价',
        goUserActivity: '去用户分析同步数据',
        showTable: '表格',
        showChart: '图表',
        viewDetail: '查看 {name} 的详情'
      },
      empty: {
        title: '还没有可分析的数据',
        description: '销售分析使用「用户分析」同步到本地的用户、入账和用量数据。登录自己站点后会在后台自动同步，也可以去用户分析页面马上同步。'
      },
      coverage: {
        partial: '入账数据从 {from} 开始覆盖，所选区间不完整，汇总只包含已覆盖的日期。',
        snapshot: 'new-api 站点的入账按两次同步之间的余额变化统计（从 {from} 开始），无法区分充值来源，也无法识别首次入账用户。',
        usagePartial: '用量数据只覆盖 {range}，所选区间不完整',
        previousMissing: '上一周期没有完整的入账数据，无法计算环比'
      },
      kpi: {
        credits: '入账金额',
        creditsHint: '{count} 笔 · 另有扣减 {deductions}',
        payers: '入账用户',
        payersHint: '首次入账 {newPayers} 人 · 复购 {repeat} 人',
        arppu: '人均入账',
        arppuHint: '入账金额 ÷ 入账用户',
        consumption: '用户消耗',
        consumptionHint: '入账 ÷ 消耗 = {ratio}',
        consumptionNone: '所选区间没有消耗',
        balanceChange: '资金沉淀',
        balanceChangeHint: '期末用户余额 {value}',
        balanceUnknown: '缺少期初或期末的余额快照'
      },
      trend: {
        title: '每日入账与消耗',
        subtitle: '柱：按渠道堆叠的入账；线：用户消耗',
        total: '入账合计',
        deductions: '扣减',
        consumption: '用户消耗',
        day: '日期',
        noData: '无数据',
        aria: '{range} 每日入账（按渠道）与用户消耗'
      },
      channels: {
        title: '入账渠道',
        subtitle: '扣减（管理员扣款、余额减少）单独列出，不计入入账合计',
        channel: '渠道',
        amount: '金额',
        share: '占入账',
        events: '笔数',
        users: '人数',
        perEvent: '笔均',
        notCounted: '不计入',
        empty: '所选区间没有入账记录。',
        recordsNote: '在线充值来自站点支付订单；兑换码可能是售卡也可能是赠送；邀请返利转入不是现金收入。'
      },
      conversion: {
        title: '转化、复购与集中度',
        registered: '新用户转化',
        registeredHint: '区间内注册 {registered} 人，已入账 {paid} 人',
        repeat: '复购率',
        repeatHint: '{payers} 个入账用户中 {repeat} 人在 2 天及以上有入账',
        concentration: '前 10 名占比',
        concentrationHint: '第 1 名占 {top1}',
        distribution: '人均入账分布',
        bucket: '{min} ~ {max}',
        bucketOpen: '{min} 以上',
        bucketUsers: '{count} 人'
      },
      payers: {
        title: '入账排行',
        subtitle: '按区间内入账金额排序，前 20 名；点击用户查看详情',
        user: '用户',
        credits: '入账',
        events: '笔数 / 天数',
        eventsValue: '{events} 笔 / {days} 天',
        channels: '渠道',
        consumption: '区间消耗',
        balance: '当前余额',
        lastCredit: '最近入账',
        empty: '所选区间没有入账用户。'
      },
      footnote: '入账包含在线充值、兑换码、管理员加款和邀请返利转入（new-api 为余额增加），按「我的站点售价」换算为人民币，不等于实收现金；资金沉淀是期末与期初的用户余额之差。',
      errors: {
        network: '网络请求失败，请检查网络后重试。',
        request: '销售分析数据加载失败，请稍后重试。',
        invalidRange: '统计区间无效：开始日期不能晚于结束日期，结束日期不能晚于今天，跨度不超过 366 天。'
      }
    },
    tickets: {
      title: '工单',
      subtitle: 'TransitLink 用户提交的问题。新工单和用户追问会推送到「系统设置 → 策略」里选的工单通知机器人。',
      refresh: '刷新',
      loading: '正在加载…',
      disabledHint: '还没有开启工单：在「客户端」页面打开工单后，用户就能在 TransitLink 里给你提交问题。',
      openClientPage: '去「客户端」页面',
      tabs: {
        all: '全部'
      },
      status: {
        open: '待回复',
        pending: '用户追问',
        replied: '已回复',
        closed: '已关闭'
      },
      empty: '还没有工单。',
      emptyFiltered: '没有这个状态的工单。',
      unknownUser: '未知用户',
      page: '第 {page} / {pages} 页',
      previous: '上一页',
      next: '下一页',
      badge: '{count} 个工单待处理',
      detail: {
        loading: '正在加载…',
        status: '状态',
        close: '关闭',
        user: '用户',
        userWithId: '{name}（ID {id}）',
        email: '邮箱',
        contact: '联系方式',
        client: '客户端',
        you: '客服（你）',
        closedNote: '工单已关闭；回复后会重新打开。',
        reply: '回复',
        replyPlaceholder: '输入回复，Ctrl/⌘ + Enter 发送',
        send: '发送',
        attachmentLoadFailed: '图片加载失败',
        closePreview: '关闭预览'
      },
      errors: {
        network: '网络错误，请检查连接。',
        request: '请求失败，请稍后再试。'
      }
    },
    client: {
      title: '客户端',
      subtitle: '给用户发一个专属链接：用 TransitLink 打开就能接入你的站点，界面换成你的品牌，一键配置时用你推荐的分组和模型。',
      save: '保存',
      saving: '保存中…',
      saved: '已保存',
      cancel: '取消',
      loading: '正在加载…',
      link: {
        title: '专属链接',
        description: '用户点这个链接会打开 TransitLink，自动接入你的站点并换上你的品牌。',
        enabled: '开启',
        enabledHelp: '关掉后链接和配置接口都不能用；已经接入的用户照常使用，只是不再更新品牌和公告。',
        hubUrl: 'TransitHub 对外地址',
        hubUrlHelp: '用户的电脑要能访问这个地址。只需要对外开放 /api/client/ 开头的接口，管理后台不用开放。',
        siteUrl: '用户访问的站点地址',
        siteUrlHelp: '默认用工作区的站点地址；用户访问的地址不一样时（例如单独的 API 域名）填这里。',
        minVersion: 'TransitLink 最低版本',
        minVersionPlaceholder: '例如 0.2.0，留空不限制',
        minVersionHelp: '用户的 TransitLink 低于这个版本时，会提示他们更新到最新版。',
        versions: '查看 TransitLink 的版本',
        link: '专属链接',
        unsaved: '保存一次就会生成链接。',
        needHubUrl: '填好 TransitHub 对外地址后就会生成链接。',
        otherDomain: 'TransitHub 对外地址和站点地址不在同一个域名下：用户第一次打开专属链接时，TransitLink 会提醒他确认链接是站长发的。把 TransitHub 放在站点的同一个域名下（例如 hub.你的域名），用户就不会看到这个提醒。',
        disabledNote: '还没开启，链接暂时打不开。',
        copy: '复制',
        copied: '已复制',
        reset: '重置链接',
        resetConfirm: '重置后旧链接马上失效，已经接入的用户下次读取配置也会失败，确定吗？',
        configUrl: '配置地址',
        openConfig: '在浏览器里查看'
      },
      brand: {
        title: '品牌',
        description: '用户接入后，TransitLink 显示你的名称、logo 和主题色。',
        name: '名称',
        namePlaceholder: '留空时用站点自己的名称',
        color: '主题色',
        logo: 'Logo',
        upload: '上传图片',
        remove: '移除',
        logoUrl: '或者填图片地址（https）',
        logoHelp: 'PNG、JPG、WebP、GIF，不超过 256 KB，建议用正方形。',
        uploaded: '已上传图片，保存后生效。',
        welcome: '欢迎语',
        welcomePlaceholder: '例如：欢迎使用，遇到问题随时找客服',
        preview: '预览',
        previewName: '站点名称',
        previewHome: '首页',
        previewTools: '工具'
      },
      recommend: {
        title: '推荐配置',
        description: '用户一键配置时默认用这里的分组和模型，用户也可以自己改。',
        claudeCode: 'Claude Code',
        codex: 'Codex',
        group: '分组',
        model: '模型',
        groupAuto: '不指定（客户端按分组名自动推荐）',
        groupManual: '分组名（new-api）或分组 ID（sub2api）',
        missingGroup: '{group}（站点里已经没有这个分组）',
        claudeModelPlaceholder: '不指定（用 Claude Code 的默认模型）',
        codexModelPlaceholder: '不指定（自动选最新的 GPT 模型）',
        groupsLoading: '正在读取站点分组…',
        groupsFailed: '读取站点分组失败：{error}。可以直接填分组。'
      },
      notice: {
        title: '公告与客服',
        description: '公告和客服方式显示在 TransitLink 的首页。',
        notice: '公告',
        noticePlaceholder: '例如：本周六 2:00–4:00 维护',
        support: '客服方式',
        supportEmpty: '还没有添加客服方式。',
        add: '添加',
        remove: '删除',
        type: '类型',
        label: '名称，例如：Telegram 群',
        value: '链接、号码或邮箱',
        types: {
          telegram: 'Telegram',
          qq: 'QQ',
          wechat: '微信',
          discord: 'Discord',
          email: '邮箱',
          url: '网页',
          other: '其他'
        }
      },
      tickets: {
        title: '工单',
        description: '开启后，用户可以在 TransitLink 里给你提交问题，你在「工单」页面回复。',
        enabled: '开启',
        categories: '问题分类',
        categoriesHelp: '每行一个，用户提交时从中选择。最多 20 个，每个不超过 40 个字。',
        notifyHelp: '新工单和用户追问可以推送到通知机器人：在「系统设置 → 策略」里选择。',
        openTickets: '查看工单'
      },
      errors: {
        invalid: '有内容格式不对，请检查后再保存。',
        logoTooLarge: 'Logo 太大了，请换一张不超过 256 KB 的图片。',
        logoType: 'Logo 只支持 PNG、JPG、WebP、GIF。',
        request: '请求失败，请稍后再试。',
        network: '网络错误，请检查连接。'
      }
    },
    update: {
      title: '检查更新',
      checking: '正在检查…',
      current: '当前版本',
      latest: '最新版本',
      publishedAt: '发布于 {date}',
      upToDate: '已经是最新版本。',
      available: '有新版本 {version}',
      newVersion: '有新版本 {version}，点击查看',
      badge: '有新版本',
      notes: '更新说明',
      howTo: '更新方法（Docker 部署）',
      step1: '把 docker-compose.prod.yml 里的镜像版本改成 {version}（用 latest 的跳过这一步）。',
      step2: '在部署目录运行下面的命令，拉取新镜像并重启：',
      copy: '复制命令',
      viewRelease: '在 GitHub 上查看这个版本',
      recheck: '重新检查',
      close: '关闭',
      online: {
        title: '在线更新',
        description: '服务器会从 GitHub 下载 {version}，校验无误后安装并自动重启。通常一两分钟，重启时页面会暂时打不开，完成后自动刷新。',
        start: '更新到 {version}',
        confirm: '确定现在更新到 {version} 吗？重启时正在进行的后台采集会中断，重启后可以重新开始。',
        confirmButton: '确定更新',
        cancel: '取消'
      },
      progress: {
        downloading: '正在下载 {version}…',
        downloadLabel: '下载进度',
        downloaded: '已下载 {done} / {total} MB（{percent}%）',
        installing: '下载完成，正在校验并安装…',
        restarting: '正在重启到 {version}…',
        reloading: '已更新到 {version}，正在刷新页面…',
        keepOpen: '请不要关闭这个页面，更新完成后会自动刷新。',
        stalled: '等了 3 分钟还没连上服务。请稍后刷新页面；如果一直打不开，请查看服务器日志（docker compose logs app）。',
        reverted: '新版本没能启动，服务已自动退回 {version}。请查看服务器日志（docker compose logs app）找原因，或者用下面的 Docker 方法更新。',
        reload: '刷新页面'
      },
      deployment: {
        notPersistent: '数据目录没有挂载持久卷：在线更新装的新版本和凭据加密的密钥都保存在这里，重新创建容器后会丢失。请在 docker-compose.prod.yml 的 app 服务里加上 ../data/app:/app/data，然后重新创建容器。',
        notEncrypted: '保存的登录凭据（站点会话、机器人密钥、代理密码）目前没有加密。挂载上面的数据目录，或者设置环境变量 CREDENTIAL_ENCRYPTION_KEY 后重启，就会自动加密。'
      },
      reasons: {
        notAdmin: '只有系统管理员可以在线更新。',
        unsupported: '这台服务器不支持在线更新（需要在 Linux 服务器上运行，并且数据目录可写）。',
        noPackage: '这个版本没有在线更新包。'
      },
      errors: {
        checkFailed: '检查更新失败：连不上 GitHub，或者请求太频繁，请稍后再试。',
        forbidden: '只有系统管理员可以在线更新。',
        inProgress: '已经在更新了，请稍候。',
        unavailable: '现在没有可以安装的新版本，请重新检查。',
        failed: '更新失败，当前版本不受影响。请稍后重试；一直失败时请查看服务器日志（docker compose logs app）。'
      }
    },
    collections: {
      running: '后台采集中',
      runningProfit: '利润分析正在后台采集',
      runningUsers: '用户分析正在后台同步',
      separator: '；',
      view: '查看',
      goDashboard: '去仪表盘登录',
      dismiss: '关闭提醒',
      unknownError: '采集出错，请稍后再看。',
      profit: {
        done: '利润分析采集完成',
        firstDone: '利润分析首次采集完成',
        resumed: '利润分析已在后台继续并采集完成',
        doneDetail: '数据已更新到 {date}',
        failed: '利润分析采集失败'
      },
      users: {
        done: '用户数据同步完成',
        firstDone: '用户分析首次同步完成',
        resumed: '用户数据已在后台继续并同步完成',
        doneDetail: '共 {count} 个用户，销售分析也已更新',
        failed: '用户数据同步失败',
        viewSales: '看销售分析'
      }
    },
    profit: {
      title: '利润分析',
      subtitle: '按上游和分组看毛利：营收来自自己站点每条路由（sub2api 上游账号 / new-api 渠道）的分组统计，成本来自已登记上游站点的消耗，按上游域名自动对应，不需要手动绑定。只读。',
      common: {
        placeholder: '—'
      },
      sync: {
        never: '尚未采集',
        lastSynced: '{time}采集',
        coverage: '数据覆盖 {range}',
        routes: '{count} 条路由',
        running: '正在采集…',
        failed: '最近一次采集失败：{reason}'
      },
      sources: {
        sub2api_groups: '营收按账号和分组统计',
        newapi_flow: '营收来自数据看板（按渠道和分组）',
        newapi_log_stat: '营收按渠道逐个查询日志'
      },
      source: {
        basis: '营收按 ¥{price}/$1 换算（与仪表盘一致）'
      },
      range: {
        label: '统计区间',
        lastDays: '近 {days} 天',
        from: '开始日期',
        to: '结束日期',
        apply: '应用',
        current: '统计区间：{range}（截至昨天的完整自然日）',
        previous: '对比上一周期：{range}'
      },
      actions: {
        refresh: '刷新',
        sync: '立即采集',
        startSync: '开始首次采集',
        changePrice: '修改售价',
        goDashboard: '去仪表盘登录',
        goUpstream: '去登记上游',
        showTable: '表格',
        showChart: '图表',
        expand: '展开 {name} 的站点和路由',
        collapse: '收起 {name}'
      },
      empty: {
        title: '还没有利润数据',
        description: '利润分析会读取自己站点每条路由按分组的营收，以及已登记上游站点每天的消耗，按上游域名自动对应。登录自己站点后，系统会在一小时内自动在后台开始首次采集（回补最近 30 天，可能需要几分钟），之后每小时自动补齐；也可以点下面的按钮马上开始。',
        runningTitle: '正在进行首次采集',
        runningDescription: '正在回补最近 30 天的营收和上游成本，可能需要几分钟，完成后页面会自动刷新。',
        requirements: '需要先在仪表盘登录自己站点的管理员账号，并在「上游管理」登记上游站点、设置充值倍率。'
      },
      coverage: {
        partial: '所选区间还没有全部采集或定稿：已采集 {range}，定稿到 {complete}。没有采集的日期不计入合计。',
        previousMissing: '上一周期没有完整的数据，无法计算环比'
      },
      kpi: {
        revenue: '营收',
        revenueHint: '自己站点用户的消耗，按售价换算',
        cost: '上游成本',
        costHint: '已登记上游站点的消耗 × 充值倍率',
        costFailedHint: '有 {count} 个站点日的成本查询失败，成本可能偏低',
        profit: '毛利',
        profitHint: '毛利率 {margin}',
        previousMargin: '（上期 {margin}）',
        profitDelta: '较上期 {value}',
        attributed: '归因覆盖',
        attributedHint: '未归因营收 {value}（成本未知）',
        unmatched: '未匹配成本',
        unmatchedHint: '有成本，但没有路由指向的上游'
      },
      insights: {
        title: '需要关注',
        severity: {
          critical: '严重',
          warning: '注意',
          info: '提示'
        },
        negativeMargin: '上游 {subject} 在亏损：成本比营收多 {value}。',
        unmatchedCost: '上游 {subject} 有成本 {value}，但没有路由指向它：可能路由地址和登记地址不是同一个域名，或者 key 在本站以外被使用。',
        unregistered: '有 {value} 营收来自没有登记的上游 {subject}，成本没有计入。登记后就能算出毛利。',
        lowRunway: '{subject} 的余额约可用 {days}，建议充值 {topUp}。',
        costSpike: '上游 {subject} 昨天的成本占营收 {ratio}，明显高于前 7 天的中位数 {baseline}：可能上游涨价，或 key 在本站以外被使用。',
        noRate: '{subject} 没有设置充值倍率，成本按 0 计算。',
        costFailed: '{subject} 有 {count} 天成本查询失败，这些天的成本没有计入。',
        priceIncrease: '{subject} 倍率上调 {change}，按近 7 天消耗估算每天多 {value}。',
        priceIncreaseUnknown: '{subject} 倍率上调 {change}，暂时无法估算对成本的影响。'
      },
      trend: {
        title: '每日营收、成本与毛利',
        subtitle: '线：营收和上游成本；柱：毛利（可能为负）',
        revenue: '营收',
        cost: '上游成本',
        profit: '毛利',
        margin: '毛利率',
        day: '日期',
        noData: '未采集',
        aria: '{range} 每日营收、上游成本与毛利'
      },
      upstreams: {
        title: '按上游',
        subtitle: '同一个域名下的站点和路由归为一个上游（api.example.com 和 example.com 视为同一个）；点击展开查看站点和路由',
        upstream: '上游',
        revenue: '营收',
        cost: '成本',
        profit: '毛利',
        margin: '毛利率',
        balance: '余额',
        runway: '续航',
        topUp: '建议充值',
        topUpHint: '按近 7 天日均成本，备足 7 天还差的金额',
        routes: '{count} 条路由',
        noRoutes: '没有路由',
        noRoutesHint: '没有路由指向这个上游，它的成本计入「未匹配成本」。',
        runwayDays: '{days} 天',
        runwayLong: '很长',
        sites: '上游站点',
        site: '站点',
        rechargeRate: '充值倍率',
        rangeCost: '区间成本',
        avgDailyCost: '近 7 天日均成本',
        deleted: '已删除',
        noRate: '未设置充值倍率',
        failedDays: '{count} 天查询失败',
        routeList: '路由',
        route: '路由',
        groups: '分组',
        routeRevenue: '区间营收',
        routeDisabled: '已停用',
        routeError: '异常',
        routeDeleted: '已删除',
        empty: '还没有登记上游站点，或所选区间没有成本和营收。'
      },
      groups: {
        title: '按分组',
        subtitle: '分组成本按该分组在每个上游的营收占比分摊；毛利和毛利率只算已归因的营收',
        group: '分组',
        revenue: '营收',
        cost: '分摊成本',
        profit: '毛利',
        margin: '毛利率',
        unattributed: '未归因营收',
        none: '（无分组）',
        empty: '所选区间没有营收。'
      },
      unattributed: {
        title: '未归因营收',
        subtitle: '这些营收找不到对应的已登记上游，成本未知，不计入毛利率',
        category: '类别',
        address: '上游地址',
        routes: '路由',
        revenue: '营收',
        categories: {
          official: '官方账号 / 没有填地址',
          unregistered: '没有登记的上游',
          unknown: '未知路由'
        },
        unknownRoute: '已删除或站点没有记录的路由',
        empty: '所选区间的营收都已归因。'
      },
      priceChanges: {
        title: '上游调价',
        subtitle: '上游站点同步时检测到的分组倍率变化（最近 30 天），按该分组近 7 天的消耗估算对每天成本的影响；只提示，不会改价',
        detectedAt: '检测时间',
        site: '上游站点',
        group: '分组',
        multiplier: '倍率',
        avgDailyCost: '近 7 天日均成本',
        impact: '每天成本影响',
        noUsage: '近 7 天未使用',
        noRate: '未设置充值倍率',
        failed: '无法估算',
        empty: '最近 30 天没有检测到上游分组倍率变化。'
      },
      footnote: '营收是自己站点用户的消耗（站点额度）按「我的站点售价」换算的人民币；成本是已登记上游站点的消耗 × 充值倍率。路由地址和上游站点按域名对应，分组成本按营收占比分摊。数据是完整自然日（北京时间），每小时 10 分自动采集；续航和建议充值按近 7 天日均成本计算。',
      warnings: {
        costFailed: '部分上游站点的成本查询失败，相关日期的成本可能偏低；最近两天会自动重试。',
        flowFallback: '站点没有开启数据看板（或版本较旧），已改为逐个渠道查询营收，采集会慢一些。',
        tooManyRoutes: '路由超过 500 条，只统计了前 500 条。'
      },
      errors: {
        authRequired: '自己站点的管理员登录已失效，请先在仪表盘重新登录。',
        platformUnsupported: '当前站点类型不支持利润分析（只支持 sub2api 和 new-api）。',
        syncInProgress: '正在采集中，请稍后再试。',
        routesFetch: '读取站点的账号 / 渠道列表失败，请稍后重试。',
        revenueFetch: '读取站点的营收统计失败，请稍后重试。',
        network: '网络请求失败，请检查网络后重试。',
        interrupted: '上次采集被服务重启打断，会在服务启动约 2 分钟后在后台自动继续，无需操作。',
        invalidRange: '统计区间无效：开始日期不能晚于结束日期，结束日期不能晚于昨天，跨度不超过 366 天。',
        request: '利润分析数据加载失败，请稍后重试。'
      }
    },
    settings: {
      ticketNotify: {
        title: '工单通知',
        help: '用户在 TransitLink 里新建工单或追问时，推送给选中的机器人；不选就不推送。'
      },
      dailyReport: {
        title: '经营日报',
        help: '每天在设定的时间，把昨天的营收、成本、毛利、入账、活跃用户和上游余额汇总发给机器人。数据来自仪表盘与用户分析的本地数据，不会访问站点。',
        time: '发送时间（北京时间）',
        timeHelp: '服务在设定时间之后才启动时，当天仍会补发一次',
        preview: '预览昨日日报',
        sendTest: '立即测试发送',
        sent: '已发送给 {count} 个机器人',
        previewTitle: '日报内容预览',
        saveHint: '开关、时间和机器人需要点击右上角「保存」后生效；预览和测试发送不需要先保存。',
        errors: {
          noBots: '请先选择至少一个接收机器人。',
          request: '日报生成或发送失败，请稍后重试。'
        }
      },
      title: '系统设置',
      subtitle: '管理系统运行参数、通知渠道及自动化策略。',
      save: '保存配置',
      saving: '保存中...',
      saveSuccess: '已保存',
      strategyDescription: '配置数据刷新频率、预警阈值和自动化策略。',
      requiresRefresh: '建议先开启数据刷新频率，以便系统自动检测变化并触发预警。',
      balanceWarningAmount: '触发金额（CNY）',
      notifyBots: '发送通知到机器人',
      customTemplate: '自定义通知文案',
      templateEditor: {
        formatLabel: '模板格式',
        formatHelp: '实时预览会代入示例数据；发送时系统会按各通知渠道支持的富文本格式自动适配。',
        editor: '模板内容',
        preview: '实时预览',
        previewTitle: '通知模板实时预览',
        formats: {
          text: '纯文本',
          markdown: 'Markdown',
          html: 'HTML'
        },
        samples: {
          siteName: '示例上游站点',
          balance: '8.50',
          threshold: '10.00',
          groupName: '默认分组',
          oldRate: '1.0000',
          newRate: '1.1200',
          changeDirection: '上升',
          costImpact: '按该分组近 7 天日均成本 ¥20.00 估算，每天约多 ¥4.00'
        }
      },
      balanceTemplateVars: '(支持变量: {siteName}, {balance}, {threshold})',
      multiplierTemplateVars: '(支持变量: {siteName}, {groupName}, {oldRate}, {newRate}, {changeDirection}, {costImpact})',
      unnamedBot: '未命名机器人',
      noBotsConfigured: '请先在"通知与渠道"中配置机器人',
      mustSelectBot: '必须选择至少一个通知机器人',
      varSiteName: '站点名称',
      varBalance: '当前余额（CNY）',
      varThreshold: '阈值金额（CNY）',
      varGroupName: '分组名称',
      varOldRate: '原倍率',
      varNewRate: '新倍率',
      varChangeDirection: '变更方向',
      varCostImpact: '成本影响',
      botNameLabel: '机器人名称标识',
      botNameDingtalkPlaceholder: '例如：钉钉主群',
      botNameWecomPlaceholder: '例如：企业微信主群',
      botNameQQPlaceholder: '例如：QQ 单聊通知',
      botNameFeishuPlaceholder: '例如：飞书主群',
      botNameTelegramPlaceholder: '例如：TG主群',
      addDingtalkBot: '添加钉钉机器人',
      addWecomBot: '添加企业微信机器人',
      addQQBot: '添加 QQ 机器人',
      addFeishuBot: '添加飞书机器人',
      addTelegramBot: '添加 TG 机器人',
      emptyDingtalk: '暂无钉钉机器人配置',
      emptyWecom: '暂无企业微信机器人配置',
      emptyQQ: '暂无 QQ 机器人配置',
      emptyFeishu: '暂无飞书机器人配置',
      emptyTelegram: '暂无 Telegram 机器人配置',
      tabs: {
        strategy: '自动化与策略',
        channels: '通知与渠道',
        templates: '消息模板',
        email: '邮件设置',
        network: '网络代理'
      },
      proxy: {
        title: '网络代理',
        subtitle: 'TransitHub 访问上游站点（包括你自己的 sub2api / new-api 站点）时使用的代理。修改后立即生效，无需重启。通知机器人不受影响，Telegram 有单独的代理设置。',
        loading: '正在读取代理设置…',
        mode: {
          environment: '当前生效：沿用启动时的环境变量代理 {proxy}',
          environmentDirect: '当前生效：沿用启动时的环境变量（未设置代理，直连）',
          proxy: '当前生效：使用代理 {proxy}',
          direct: '当前生效：不使用代理（直连）'
        },
        unconfiguredHint: '还没有在这里保存过代理设置，下方已按启动环境变量预填。保存后以这里的设置为准。',
        readOnly: '代理设置对整个 TransitHub 生效，只有系统管理员（ADMIN_EMAIL 对应的账号）可以修改。',
        enable: '启用代理',
        enableHelp: '关闭后所有上游请求直连。',
        url: '代理地址',
        urlPlaceholder: '例如 http://127.0.0.1:7897，支持 http、https、socks5',
        urlHelp: "需要账号密码时写成 http://用户名:密码{'@'}地址:端口。已保存的密码显示为 ***，不修改会保留原密码。",
        urlRequired: '启用代理时必须填写代理地址。',
        bypass: '不走代理的地址',
        bypassPlaceholder: '例如 milus.one, *.example.com, 10.0.0.0/8',
        bypassHelp: '用逗号或换行分隔。填写域名会同时匹配它的子域名，也支持 IP 和网段；localhost 与 127.0.0.1 始终直连。',
        save: '保存代理设置',
        saved: '已保存，并已立即生效',
        test: {
          title: '测试连接',
          help: '用上面尚未保存的设置访问一个地址，确认能否连通，例如填写你的上游站点地址。',
          target: '测试地址',
          targetPlaceholder: 'https://你的上游站点地址',
          run: '测试连接',
          running: '测试中…',
          success: '连接成功（{route}，HTTP {status}，{ms} ms）',
          failure: '连接失败（{route}，{ms} ms）：{reason}',
          routes: {
            proxy: '经代理',
            direct: '直连'
          },
          failures: {
            dns: '域名无法解析，请检查地址是否正确',
            proxy: '代理无法连接到目标站点，或代理本身不可用。请检查代理是否在运行、代理规则或节点，或把该地址加入「不走代理的地址」',
            tls: 'TLS 握手或证书校验失败',
            timeout: '连接超时',
            refused: '连接被拒绝，请检查地址和端口',
            network: '网络连接失败'
          }
        },
        errors: {
          forbidden: '只有系统管理员（ADMIN_EMAIL 对应的账号）可以修改代理设置。',
          invalidUrl: '代理地址无效：请填写 http、https 或 socks5 地址，例如 http://127.0.0.1:7897。',
          invalidTarget: '测试地址无效，请填写 http 或 https 地址。',
          request: '代理设置请求失败，请稍后重试。',
          unavailable: '代理设置暂不可用。'
        }
      },
      sections: {
        basic: {
          title: '基础设置',
          description: '配置系统的基础运行参数。',
          refreshInterval: '数据刷新频率',
          refreshIntervalHelp: '设置系统在后台自动拉取并刷新上游站点数据的时间间隔，最低 60 秒。',
          seconds: '秒'
        },
        thresholds: {
          title: '站点预警阈值',
          description: '配置针对所有上游站点的默认报警触发条件。',
          balanceWarning: '余额预警',
          balanceWarningHelp: '当某个上游站点的余额（按充值倍率折合人民币）低于设定金额时，通过机器人发送预警通知。',
          multiplierChangeWarning: '倍率变更预警',
          multiplierChangeWarningHelp: '上游站点任一分组的倍率发生变动时，通过机器人发送通知。'
        },
        channels: {
          title: '通知渠道配置',
          description: '配置接收系统报警的第三方渠道参数（如钉钉、企业微信、QQ、TG、飞书）。',
          dingtalk: '钉钉机器人',
          dingtalkHelp: '配置钉钉群机器人的 Webhook 和加签密钥。',
          wecom: '企业微信机器人',
          wecomHelp: '配置企业微信群机器人的 Webhook，无需加签密钥。',
          qq: 'QQ 机器人',
          qqHelp: '配置 QQ 官方机器人的应用凭据和接收通知的用户 OpenID。',
          feishu: '飞书机器人',
          feishuHelp: '配置飞书群机器人的 Webhook 和加签密钥。',
          telegram: 'Telegram 机器人',
          telegramHelp: '配置 Telegram Bot Token 和接收消息的 Chat ID。',
          webhookUrl: 'Webhook 地址',
          secret: '加签密钥 (Secret)',
          appId: 'AppID',
          appIdPlaceholder: '请输入机器人 AppID',
          appSecret: 'AppSecret',
          appSecretPlaceholder: '请输入机器人 AppSecret',
          userOpenId: '用户 OpenID',
          userOpenIdPlaceholder: '请输入接收通知用户的 openid',
          userOpenIdHelp: '在消息列表沙箱中添加该用户，用户扫码并向机器人发送消息后，从 C2C_MESSAGE_CREATE 事件中获取。',
          botToken: 'Bot Token',
          chatId: 'Chat ID',
          proxyUrl: '代理地址（可选）',
          proxyUrlPlaceholder: '例如 http://127.0.0.1:7890',
          proxyUrlHelp: '服务器无法直连 Telegram 时填写代理地址；留空则直连。',
          loading: '正在加载通知渠道配置...',
          testConnection: '测试连通性',
          testConnectionSuccess: '发送成功'
        },
        templates: {
          balanceDefaultTemplate: '🔴 **余额预警**\n\n🏷️ **站点：** {siteName}\n💰 **当前余额：** ¥{balance}\n⚠️ **预警阈值：** ¥{threshold}\n\n请及时检查并充值，避免服务中断。',
          multiplierDefaultTemplate: '🟠 **倍率变更预警**\n\n🏷️ **站点：** {siteName}\n📦 **分组：** {groupName}\n📊 **倍率：** {oldRate}x → **{newRate}x**（{changeDirection}）\n💸 **成本影响：** {costImpact}\n\n🔎 请确认成本变化，并检查下游定价策略。',
          balanceTemplatePlaceholder: '例如：🔴 {siteName} 当前余额 ¥{balance}，已低于 ¥{threshold}。',
          multiplierTemplatePlaceholder: '例如：🟠 {siteName} / {groupName}：{oldRate}x → {newRate}x。'
        }
      },
      errors: {
        network: '网络或 CORS 请求失败，请检查接口地址与跨域配置。',
        request: '通知渠道测试请求失败，请稍后重试。',
        unknown: '测试通知渠道时发生未知错误。',
        invalidChannel: '通知渠道类型无效。',
        missingWebhook: '请先填写机器人 Webhook 地址。',
        missingQQConfig: '请先填写 QQ 机器人的 AppID、AppSecret 和用户 OpenID。',
        missingTelegramConfig: '请先填写 Telegram Bot Token 和 Chat ID。',
        sendFailed: '测试消息发送失败，请检查机器人配置和网络连通性。'
      },
      smtp: {
        title: 'SMTP 邮件设置',
        description: '配置用于发送系统邮件的 SMTP 服务器。',
        host: 'SMTP 主机',
        port: '端口',
        tlsMode: 'TLS 模式',
        tlsStarttls: 'STARTTLS (587)',
        tlsImplicit: '隐式 TLS (465)',
        username: '用户名',
        password: '密码',
        passwordConfigured: '已保存密码',
        passwordNotConfigured: '未保存密码',
        passwordKeepPlaceholder: '留空以保留已保存密码',
        passwordNewPlaceholder: '输入 SMTP 密码',
        fromEmail: '发件邮箱',
        fromName: '发件名称',
        testRecipient: '测试收件人',
        saveSuccess: 'SMTP 设置已保存',
        testEmail: '发送测试邮件',
        testEmailSuccess: '测试邮件已发送',
        dirtyBeforeTest: '请先保存当前 SMTP 设置再发送测试邮件',
        errors: {
          validation: '请检查 SMTP 设置。',
          missingConfig: '请先保存 SMTP 设置。',
          invalidTlsMode: 'TLS 模式无效。',
          invalidEmail: '邮箱地址无效。',
          invalidPort: '端口必须是 1-65535 之间的整数。',
          encryptionKeyUnavailable: '服务器未配置 SMTP 加密密钥。',
          decryptFailed: '无法读取已保存的 SMTP 密码。',
          sendFailed: '测试邮件发送失败。',
          persistence: 'SMTP 设置保存失败。'
        }
      },
      emailTemplates: {
        title: '邮件模板',
        description: '创建并维护可复用的 HTML 邮件，每个模板都可以独立测试发送。',
        library: '模板库',
        editor: '模板编辑器',
        add: '新建模板',
        builtIn: '内置',
        loading: '正在加载邮件模板...',
        empty: '暂无可用模板',
        name: '模板名称',
        subject: '邮件主题',
        htmlBody: 'HTML 正文',
        preview: '邮件预览',
        code: '查看源码',
        previewTitle: '邮件模板安全预览',
        save: '保存模板',
        delete: '删除',
        test: '发送测试邮件',
        testRecipient: '测试收件人',
        testRecipientPlaceholder: "name{'@'}example.com",
        unsaved: '有未保存的修改',
        dirtyBeforeTest: '请先保存模板，再发送测试邮件。',
        discardConfirm: '当前模板有未保存的修改，确定要放弃吗？',
        deleteConfirm: '确定删除模板“{name}”吗？此操作无法撤销。',
        newTemplateName: '自定义模板',
        newTemplateSubject: '请输入邮件主题',
        newTemplateHtml: '<div style="font-family:Arial,sans-serif;padding:32px"><h1>在这里填写标题</h1><p>在这里编写邮件内容。</p></div>',
        createSuccess: '模板已创建',
        saveSuccess: '模板已保存',
        deleteSuccess: '模板已删除',
        testEmailSuccess: '测试邮件已发送',
        errors: {
          validation: '请填写模板名称、单行主题和不超过 100KB 的 HTML 正文。',
          invalidEmail: '测试收件人邮箱无效。',
          notFound: '邮件模板不存在或已被删除。',
          builtInProtected: '内置模板不能删除，但可以自由编辑。',
          limitReached: '最多可创建 50 个自定义模板。',
          persistence: '邮件模板保存失败，请稍后重试。'
        }
      }
    },
    system: {
      version: '版本 {version}',
      openGithubRepository: '在 GitHub 上查看源码',
      errors: {
        network: '系统信息请求失败，请检查网络连接。',
        request: '系统请求失败，请稍后重试。'
      }
    }
  },
  tickets: {
    errors: {
      invalid: '内容不合法，请检查后再试。',
      notFound: '工单不存在或已删除。',
      noCurrentAccount: '请先选择一个工作区。',
      request: '请求失败，请稍后再试。'
    }
  }
}
