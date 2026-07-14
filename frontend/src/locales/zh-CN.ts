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
      openNavigation: '打开导航',
      closeNavigation: '关闭导航'
    },
    menu: {
      dashboard: '仪表盘',
      leaderboard: '排行榜',
      lottery: '抽奖活动',
      upstream: '上游管理',
      groupManagement: '分组管理',
      groupRates: '分组倍率',
      groupAssociations: '分组关联',
      connectionHealth: '分组健康',
      groupRateCampaigns: '活动调价',
      settings: '系统设置',
      tickets: '工单',
      massEmail: '群发邮件',
      signOut: '退出登录'
    },
    leaderboard: {
      eyebrow: '用量排行',
      title: 'Token 排行榜',
      subtitle: '按实际 Token 使用量查看当前工作区的活跃用户。',
      refresh: '刷新排行榜',
      errorTitle: '排行榜加载失败',
      emptyTitle: '当前周期暂无用量',
      emptyDescription: '切换统计周期或稍后刷新后再查看。',
      anonymous: '用户 {id}',
      podiumLabel: '前三名用户',
      updatedAt: '更新于 {time}',
      period: { label: '统计周期', today: '今日', '7d': '7 天', '30d': '30 天' },
      metrics: { tokens: 'Token', requests: '请求数', cost: '实际消费' },
      table: { title: '完整排名', caption: '共 {count} 位用户', rank: '排名', user: '用户' },
      embed: {
        action: '嵌入设置',
        title: '排行榜嵌入设置',
        subtitle: '系统会自动绑定当前工作区，并生成安全的 Sub2API 自定义菜单链接。',
        sourceOrigin: '当前绑定站点',
        sourceOriginHint: '此地址自动读取自当前工作区的 Sub2API 管理会话，无需手动填写。',
        url: 'iframe 链接',
        urlHint: '将此链接填写到 Sub2API 自定义菜单的 URL 中。',
        copy: '复制链接',
        copyFailed: '复制失败，请手动选择链接。',
        rotate: '重新生成令牌',
        confirmRotate: '重新生成后，旧的排行榜嵌入链接将失效。确定继续吗？',
        close: '关闭'
      },
      errors: {
        network: '网络请求失败，请检查连接后重试。',
        request: '请求参数无效，请刷新后重试。',
        invalidSourceOrigin: '来源地址必须与当前工作区连接的 Sub2API 站点完全一致。',
        upstreamUnsupported: '当前 Sub2API 版本不支持排行榜接口或 Token 排序，请先升级。',
        unknown: '排行榜请求失败，请稍后重试。'
      }
    },
    lottery: {
      eyebrow: '抽奖运营',
      title: '抽奖活动',
      subtitle: '创建并运营 Sub2API 抽奖活动、报名、开奖结果、奖励发放、审计记录和嵌入令牌设置。',
      common: {
        empty: '无',
        noDescription: '暂无描述。',
        yes: '是',
        no: '否'
      },
      filters: {
        status: '活动状态',
        all: '全部状态'
      },
      tabs: {
        overview: '概览',
        entries: '报名',
        rewards: '中奖与奖励',
        audit: '审计',
        embed: '嵌入'
      },
      list: {
        title: '活动列表',
        count: '{count} 项',
        empty: '当前筛选下没有抽奖活动。'
      },
      detail: {
        empty: '选择一个活动查看详情。'
      },
      sections: {
        schedule: '时间安排',
        integrity: '开奖完整性',
        prizes: '奖品'
      },
      metrics: {
        entries: '{count} 个报名',
        winners: '{count} 个中奖'
      },
      actions: {
        create: '创建活动',
        refresh: '刷新活动',
        edit: '编辑草稿',
        publish: '发布',
        close: '关闭报名',
        draw: '开奖',
        cancel: '取消活动',
        retry: '重试',
        completeManual: '标记已兑换',
        save: '保存活动',
        closeDialog: '关闭',
        confirm: {
          publish: '确定发布此抽奖活动？',
          close: '确定关闭此活动的报名？',
          draw: '确定为此活动开奖？',
          cancel: '确定取消此活动？',
          completeManual: '确认中奖者已经完成人工兑换并将奖励标记为已发放？'
        }
      },
      status: {
        draft: '草稿',
        scheduled: '待开放',
        open: '报名中',
        closed: '报名已关闭',
        drawing: '开奖中',
        drawn: '已开奖',
        fulfilling: '发奖中',
        completed: '已完成',
        partial: '部分完成',
        cancelled: '已取消'
      },
      drawMode: {
        manual: '手动开奖',
        scheduled: '定时开奖'
      },
      prizeType: {
        balance: '余额',
        subscription: '订阅'
      },
      entryStatus: {
        active: '有效',
        withdrawn: '已撤回'
      },
      rewardStatus: {
        pending: '待处理',
        processing: '处理中',
        fulfilled: '已发放',
        retryable_failed: '可重试失败',
        manual_attention: '需人工处理',
        failed: '失败'
      },
      fields: {
        actions: '操作',
        balanceAmount: '余额金额',
        createdAt: '创建时间',
        description: '描述',
        drawAt: '开奖时间',
        drawMode: '开奖方式',
        deliveryMode: '领取方式',
        entrySnapshotHash: '报名快照哈希',
        entryId: '报名 ID',
        error: '错误',
        groupId: '分组 ID',
        groupName: '分组名称',
        subscriptionGroup: '订阅分组',
        currentMultiplier: '当前倍率',
        maskedEmail: '脱敏邮箱',
        multiplier: '倍率',
        rewardMultiplier: '奖励倍率',
        name: '活动名称',
        prize: '奖品',
        prizeName: '奖品名称',
        prizeSlot: '奖品序号',
        prizeType: '奖品类型',
        publicWinners: '公开展示中奖者',
        quantity: '数量',
        voucherCodes: '兑换券（每行一份）',
        manualContact: '人工兑换联系方式',
        receiptHash: '报名凭证哈希',
        registrationEnd: '报名结束',
        registrationStart: '报名开始',
        revealedSeed: '公开种子',
        rewardStatus: '奖励状态',
        seedCommitment: '种子承诺',
        status: '状态',
        validityDays: '有效天数',
        winner: '中奖者'
      },
      entries: {
        empty: '暂无报名。'
      },
      delivery: {
        title: '余额奖品领取设置',
        subtitle: '每个奖品可自动发放、逐份分配兑换券，或由中奖者联系管理员兑换。',
        quantityHint: '该奖品共 {count} 份',
        mode: {
          sub2api_auto: '自动发放',
          voucher: '兑换券',
          manual: '人工兑换'
        },
        voucherPlaceholder: '每行填写一个唯一兑换券码',
        voucherCount: '已填写 {current} 份，需要 {required} 份；开奖后按顺序逐份发给中奖者。',
        manualPlaceholder: '填写邮箱、客服账号或其他兑换说明',
        manualHint: '仅中奖者本人和管理员可看到此联系方式。',
        autoHint: '中奖后由当前 Sub2API 工作区自动发放余额。'
      },
      rewards: {
        empty: '暂无奖励任务。',
        manualTitle: '待确认的人工兑换'
      },
      prizes: {
        subscriptionSummary: '{group}（ID {id}）· 奖励倍率 {multiplier} · {days} 天'
      },
      audit: {
        create: '已创建',
        update: '已更新',
        publish: '已发布',
        close: '已关闭',
        draw: '已开奖',
        empty: '暂无审计事件。'
      },
      embed: {
        title: '抽奖嵌入设置',
        subtitle: '系统会自动绑定当前工作区；公开页面启用后，可将此链接用于 Sub2API 自定义菜单。',
        sourceOrigin: '当前绑定站点',
        url: 'iframe 链接',
        copy: '复制链接',
        copyFailed: '复制失败，请手动选择链接。',
        rotate: '重新生成令牌',
        confirmRotate: '重新生成后，旧的抽奖嵌入链接将失效。确定继续吗？'
      },
      form: {
        createTitle: '创建抽奖活动',
        editTitle: '编辑抽奖活动',
        subtitle: '草稿发布前可编辑；公开开奖默认开启，余额奖品可配置自动发放、兑换券或人工兑换。',
        namePlaceholder: '七月用户抽奖',
        descriptionPlaceholder: '活动内部备注和公开描述。',
        addPrize: '添加奖品',
        removePrize: '移除',
        prizeNumber: '奖品 {number}',
        prizeNamePlaceholder: '奖品展示名称',
        subscriptionGroupPlaceholder: '选择当前工作区的订阅分组',
        subscriptionGroupsLoading: '正在读取当前工作区分组…',
        subscriptionGroupsEmpty: '当前工作区没有可用分组',
        subscriptionGroupOption: '{name}（ID {id}）· 当前倍率 {multiplier}',
        subscriptionGroupUnavailable: '{name}（ID {id}）· 已保存奖励倍率 {multiplier} · 当前不可用',
        refreshSubscriptionGroups: '重新读取'
      },
      errors: {
        network: '网络请求失败，请检查连接后重试。',
        request: '抽奖请求无效，请刷新后重试。',
        unknown: '抽奖请求失败，请稍后重试。',
        invalidSourceOrigin: '抽奖嵌入必须使用当前工作区的 Sub2API 站点。',
        notFound: '未找到此抽奖活动。',
        invalidState: '当前活动状态不允许执行此生命周期操作。',
        validation: '请检查必填字段、时间顺序和奖品配置。',
        voucherQuantity: '兑换券必须一行一份、互不重复，并与奖品数量完全一致。',
        manualContactRequired: '人工兑换必须填写中奖者可使用的联系方式。',
        manualRedemptionRequired: '请中奖者按照显示的联系方式完成人工兑换。',
        rewardSafeMessage: '奖励发放需要处理，请查看奖励状态，并在可重试时重试。',
        rewardUnsupported: '当前 Sub2API 站点不支持此奖励类型。',
        rewardAdminSession: '请重新连接当前工作区的 Sub2API 管理员账号。',
        subscriptionGroups: '无法读取当前工作区的订阅分组，请确认 Sub2API 管理员登录仍有效。'
      }
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
        vsPrev: '较前一日'
      },
      loading: '正在加载指标数据...',
      loadError: '加载仪表盘指标失败。',
      retry: '重试',
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
      upstreamKeyUsage: {
        title: '今日成本明细',
        subtitle: '共 {count} 个 key，合计 {total}',
        close: '关闭',
        empty: '暂无今日消费的 key。',
        loadError: '加载今日成本明细失败。',
        retry: '重试',
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
            token: 'RT + AT'
          },
          fields: {
            emailPlaceholder: '管理员邮箱',
            usernamePlaceholder: '管理员账号',
            passwordPlaceholder: '管理员密码',
            accessTokenPlaceholder: 'Access Token（可选，可留空）',
            refreshTokenPlaceholder: 'Refresh Token（必填）',
            tokenTypePlaceholder: 'Token Type（可选，默认 Bearer）',
            tokenHelp: '只需填写 Refresh Token 即可登录：系统会先用它刷新一次以获取最新过期时间，并在临期时自动刷新。'
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
          reloginRequired: '管理员身份校验失败，请重新登录。'
        }
      }
    },
    groupAssociations: {
      title: '分组关联',
      subtitle: '共 {count} 组 · {associated} 组已关联 · {unassociated} 组未关联',
      close: '关闭',
      empty: '暂无分组映射数据。',
      loadError: '加载分组列表失败。',
      runError: '执行自动调价失败，请重试。',
      unassociatedLabel: '未关联上游',
      unassociatedMultiplier: '暂无倍率',
      columns: {
        index: '序号',
        ownGroup: '我的分组',
        platform: '平台',
        groupType: '分组类型',
        status: '状态',
        ownMultiplier: '我的倍率',
        upstreamGroup: '对接分组',
        upstreamMultiplier: '对接倍率',
        autoPricing: '自动调价'
      },
      exclusiveLabels: {
        public: '公开',
        exclusive: '专属'
      },
      statusLabels: {
        active: '启用',
        inactive: '禁用'
      },
      autoPricingTip: '开启后，同步倍率时自动在上游倍率基础上加价，支持固定值或百分比两种策略。',
      autoPricingStatus: {
        notConfigured: '未配置',
        enabled: '已开启',
        savedDisabled: '已保存，未启用'
      },
      autoPricingActions: {
        configure: '配置',
        edit: '编辑',
        runNow: '立即执行',
        runNowFor: '立即执行 {group} 的自动调价'
      },
      lastRun: {
        never: '从未执行',
        summary: '上次：{status} · {trigger} · {time}',
        reason: '原因：{reason}',
        triggerManual: '手动',
        triggerAfterSync: '同步后',
        triggerUnknown: '未知触发',
        reasonUnknown: '暂无详情',
        status: {
          applied: '成功',
          skipped: '跳过',
          thresholdExceeded: '超过阈值',
          failed: '失败',
          unknown: '无记录'
        },
        reasons: {
          primary_upstream_not_affected: '主上游未受本次同步影响。',
          missing_reference_multiplier: '缺少参考倍率。',
          unknown_pricing_source: '无法识别定价来源。',
          status_persist_failed: '执行状态保存失败。',
          invalid_old_reference_multiplier: '原参考倍率无效。',
          threshold_exceeded: '变化超过配置阈值。',
          own_group_not_found_in_admin: '管理员站点中未找到我的分组。',
          target_unchanged: '目标倍率未变化。',
          remote_update_failed: '远端倍率更新失败。'
        }
      },
      autoPricingDrawer: {
        title: '自动调价配置',
        titleWithGroup: '{group} · 自动调价配置',
        enableLabel: '启用自动调价',
        sourceLabel: '定价来源',
        sourcePrimaryUpstream: '指定主上游',
        sourceLowestUpstream: '最低倍率上游',
        sourceHighestUpstream: '最高倍率上游',
        sourceAverageUpstream: '平均倍率',
        primaryUpstreamLabel: '主上游',
        primaryUpstreamPlaceholder: '请选择主上游',
        strategyLabel: '加价方式',
        strategyFixed: '固定加价',
        strategyPercentage: '百分比加价',
        fixedIncreaseLabel: '固定加价值',
        percentageIncreaseLabel: '百分比加价值',
        thresholdLabel: '跟随阈值',
        thresholdHelp: '上游变化不超过该百分比时才自动跟随',
        minMultiplierLabel: '最低倍率',
        maxMultiplierLabel: '最高倍率',
        estimatedMultiplier: '预估倍率',
        save: '保存配置',
        cancel: '取消',
        noUpstreams: '当前分组未关联任何上游，无法配置自动调价。',
        noMultiplierData: '暂无可用上游倍率数据，无法计算预估倍率。',
        tips: {
          minMultiplier: '自动计算出的倍率不会低于这个值。用于防止价格过低，保护最低利润。留空表示不限制最低倍率。',
          maxMultiplier: '自动计算出的倍率不会高于这个值。用于防止价格突然过高，影响用户使用。留空表示不限制最高倍率。',
          threshold: '上游倍率变化在该百分比以内时才自动跟随。超过阈值时应等待人工确认，避免上游价格异常波动导致你的分组价格被带偏。',
          minMultiplierAria: '查看最低倍率说明',
          maxMultiplierAria: '查看最高倍率说明',
          thresholdAria: '查看跟随阈值说明',
        },
        guidance: {
          title: '建议设置',
          minMultiplier: '最低倍率：你的成本价 + 最低利润',
          maxMultiplier: '最高倍率：你觉得用户还能接受的最高价',
          threshold: '跟随阈值：10%',
          exampleTitle: '计算示例',
          exampleOld: '上游原倍率：1.00',
          exampleNew: '上游新倍率：1.08',
          exampleThreshold: '跟随阈值：10%',
          exampleMarkup: '加价方式：上游 + 0.10',
          exampleMin: '最低倍率：1.00',
          exampleMax: '最高倍率：1.30',
          exampleResult: '变化幅度为 8%，未超过 10%，因此允许自动跟随；最终倍率为 1.18，并且处于 1.00 到 1.30 的限制范围内。',
        },
        notify: {
          sectionTitle: '自动调价成功通知',
          enableLabel: '调价成功后发送通知',
          enableHelp: '当自动调价实际更新了分组倍率后，通过机器人发送通知。',
          botSelectLabel: '通知机器人',
          botSelectPlaceholder: '选择要通知的机器人',
          noBots: '暂无可用机器人，请先在系统设置的通知与渠道中配置机器人。',
          templateLabel: '通知模板',
          templateHelp: '留空使用默认模板。支持以下变量：',
          templatePlaceholder: '留空则使用默认模板',
          defaultTemplate: '【自动调价】{ownGroup} 已自动从 {oldOwnMultiplier}x 调整为 {newOwnMultiplier}x。参考来源：{upstreamSiteName} / {upstreamGroupName}，参考倍率 {oldReference}x -> {newReference}x。',
          variablesTitle: '可用变量',
          varOwnGroup: '我的分组名',
          varUpstreamSiteName: '上游站点名',
          varUpstreamGroupName: '上游分组名/参考来源',
          varOldReference: '旧参考倍率',
          varNewReference: '新参考倍率',
          varOldOwnMultiplier: '调整前倍率',
          varNewOwnMultiplier: '调整后倍率',
          varStrategy: '加价策略',
          varFixedIncrease: '固定加价值',
          varPercentageIncrease: '百分比加价值',
          varThreshold: '跟随阈值',
          copied: '已复制',
        },
        errors: {
          primaryRequired: '指定主上游模式下必须选择主上游。',
          increaseNonNegative: '加价值不能为负数。',
          thresholdNonNegative: '阈值不能为负数。',
          multiplierNonNegative: '倍率不能为负数。',
          minGreaterThanMax: '最低倍率不能大于最高倍率。',
          invalidConfig: '自动调价配置无效，请检查后重试。',
          notifyBotsRequired: '开启通知时必须至少选择一个机器人。',
        }
      },
      save: '保存',
      saveSuccess: '已保存',
      saving: '保存中...',
      saveError: '保存失败，请重试。'
    },
    connectionHealth: {
      title: '分组健康',
      subtitle: '对当前 admin workspace 下分组内的账号/渠道做独立轻量探活，监控健康状态并支持自动降级/恢复。',
      adminSubtitle: '展示当前 admin workspace 下的全量分组，点击账号数查看分组下账号/渠道及独立探活状态。',
      refresh: '刷新',
      empty: '当前 admin workspace 下暂无可探活的账号/渠道。',
      adminEmpty: '当前 admin workspace 下暂无分组。',
      notConnected: '未对接',
      notProbed: '尚未探活',
      notConfigured: '未配置探活模型',
      groupTypes: {
        public: '公开',
        exclusive: '专属',
        subscription: '订阅'
      },
      groupStatusLabels: {
        active: '启用',
        inactive: '禁用'
      },
      adminColumns: {
        name: '名称',
        platform: '平台',
        type: '类型',
        multiplier: '倍率',
        accounts: '账号数',
        accountsUnit: '个',
        status: '分组状态',
        probeOverview: '探活概览',
        detail: '详情'
      },
      adminOverview: {
        probeable: '可探活 {probeable}/{total}',
        noneProbeable: '无可探活目标',
        noProbe: '{count} 个待探活'
      },
      probeUnavailableReasons: {
        credential_unavailable: '无法安全获取上游凭据，暂不可探活',
        secure_verification_required: '需要上游 root 安全验证后才能读取 channel key',
        base_url_unavailable: '缺少可用的 Base URL，暂不可探活',
        model_unavailable: '没有可用的探活模型（请在探活策略中配置）',
        export_unavailable: '上游账号导出接口不可用，无法获取凭据',
        credentials_redacted: '上游凭据已脱敏，无法用于探活'
      },
      accountsDialog: {
        multiplier: '倍率',
        unknownPlatform: '未知平台',
        unknownStatus: '未知状态',
        empty: '该分组下暂无账号/渠道。',
        noProbeData: '无探活数据',
        unprobeable: '不可探活',
        unassignedPolicy: '未分配策略',
        unassignedPolicyHint: '未分配策略，不会自动探活，仍可手动一次性探活。',
        assignedPolicyCount: '{name} 等 {count} 个',
        assignPolicy: '分配策略',
        columns: {
          name: '名称',
          platform: '平台',
          type: '类型',
          status: '状态',
          priority: '优先级',
          concurrency: '并发',
          weight: '权重',
          models: '模型',
          probeStatus: '探活状态',
          policyAssignment: '策略分配',
          actions: '操作'
        }
      },
      summary: {
        total: '探活目标数',
        unconfigured: '未配置探活'
      },
      stateLabels: {
        healthy: '健康',
        degraded: '降级',
        suspended: '已暂停',
        observing: '观察中',
        recovering: '恢复中',
        disabled: '已禁用'
      },
      providerLabels: {
        gemini: 'Gemini',
        anthropic: 'Anthropic',
        openai: 'OpenAI',
        custom: '自定义'
      },
      filters: {
        allGroups: '全部我的分组',
        allSites: '全部上游站点',
        allStates: '全部状态',
        allProviders: '全部模型类型',
        searchGroup: '搜索分组名称...',
        allPlatforms: '全部平台',
        allTypes: '全部类型'
      },
      columns: {
        model: '模型',
        state: '状态',
        weight: '权重',
        lastProbe: '最近探活',
        lastError: '最近错误'
      },
      actions: {
        probe: '手动探活',
        disable: '禁用',
        restore: '恢复',
        viewEvents: '查看事件'
      },
      errorKeys: {
        ok: '正常',
        network_fluctuation: '网络波动',
        rate_limited: '触发限流',
        server_error: '上游服务异常',
        auth: '鉴权失败',
        model_not_found: '模型不存在',
        invalid_response: '响应无法解析',
        unsupported: '暂不支持',
        manual_disable: '人工禁用',
        manual_restore: '人工恢复'
      },
      topActions: {
        runFlow: '运行流程',
        policies: '探活策略',
        events: '探活事件'
      },
      events: {
        title: '最近探活与远端动作',
        empty: '暂无事件记录。',
        emptyForConnection: '暂无该目标事件记录。',
        showAll: '查看全部'
      },
      eventsDialog: {
        subtitle: '查看该探活目标（账号/渠道）各模型的探活健康状态。',
        globalSubtitle: '最近的探活与远端动作事件。',
        viewingConnection: '正在查看该目标事件',
        card: {
          latencyLabel: '对话延迟',
          pingLabel: '节点 PING',
          availabilityLabel: '可用率',
          recentRecordsLabel: '近 60 次记录',
          past: 'PAST',
          now: 'NOW',
          noData: '-',
          nextProbeIn: '下次探活：{seconds}s 后',
          nextProbeDue: '下次探活：已到期，等待调度',
          nextProbeNoPolicy: '下次探活：未配置策略',
          nextProbeNeverProbed: '下次探活：尚未探活',
          nextProbeDisabled: '下次探活：已禁用，不自动探活',
          remoteActionLine: '远端动作：{label}'
        }
      },
      remoteActions: {
        unsupported: '不支持（未真正调用上游）',
        skippedIndependentProbe: '未开启自动远端动作，已跳过',
        sub2apiInactive: 'Sub2API 账号已切换为 inactive',
        sub2apiActive: 'Sub2API 账号已切换为 active',
        sub2apiInactiveFailed: 'Sub2API 账号切换 inactive 失败',
        sub2apiActiveFailed: 'Sub2API 账号切换 active 失败',
        newapiDisabled: 'NewAPI channel 已禁用',
        newapiWeight: 'NewAPI channel 权重已调整为 {weight}',
        other: '{action}'
      },
      policies: {
        title: '探活策略',
        subtitle: '配置模型探活目标、阈值和自动降级/恢复行为。',
        create: '新建策略',
        empty: '暂无探活策略，点击"新建策略"开始配置。',
        enabled: '已启用',
        disabled: '已停用',
        enable: '启用',
        disable: '停用',
        edit: '编辑',
        remoteActionOn: '远端动作已开启',
        allGroupsScope: '全部分组',
        modelTargetCount: '{count} 个模型目标'
      },
      policyDrawer: {
        createTitle: '新建探活策略',
        editTitle: '编辑探活策略',
        nameLabel: '策略名称',
        namePlaceholder: '输入策略名称',
        enabledLabel: '启用该策略',
        ownGroupLabel: '策略范围',
        ownGroupAllOption: '当前 workspace 全部分组',
        modelTargetsLabel: '模型探活目标',
        addModelTarget: '添加模型',
        modelNamePlaceholder: '模型名称，如 gpt-4o-mini',
        modelEnabledLabel: '启用',
        maxProbeTokensLabel: '最大 token',
        probePromptPlaceholder: '探活 prompt（留空使用默认值）',
        probeIntervalLabel: '探活间隔（秒）',
        dailyBudgetLabel: '每日探活预算',
        failureThresholdLabel: '失败阈值',
        successThresholdLabel: '恢复成功阈值',
        cooldownLabel: '冷却时间（秒）',
        observationLabel: '观察时间（秒）',
        recoveryStepLabel: '恢复步进百分比',
        autoDegradeLabel: '自动降级',
        autoDegradeHelp: '探活失败达到阈值时自动降低本地权重或暂停链路。',
        autoRemoteActionLabel: '自动远端动作',
        autoRemoteActionHelp: 'NewAPI：自动远端动作会修改 channel 权重/状态。Sub2API：自动远端动作会切换 admin 账号 active/inactive，priority 暂不自动调整。当前分组健康的独立探活路径下，Sub2API 已支持按策略自动切换账号状态；NewAPI 独立探活维度暂未实现远端动作，只会记录为不支持，不会真正调用上游。',
        providerLabel: '模型 Provider',
        providerPlaceholder: '请选择 Provider',
        providerMismatchWarning: '检测到该策略已有的模型探活目标使用了不同的 provider。请在上方选择一个 provider，保存后所有模型探活目标都会统一为你选择的这个 provider。',
        cancel: '取消',
        save: '保存策略',
        tooltips: {
          ownGroup: '用于描述这条策略面向的业务分组范围。当前分组健康的独立探活按显式分配关系启用策略，并使用该策略的启用模型目标组成模型池；如果目标自带模型列表，则取"目标模型 ∩ 策略模型池"，否则使用策略模型池。',
          modelTargets: '这里配置该策略要探活的模型列表，自动调度和手动探活都会按这些模型逐一执行探活请求。',
          provider: '一个探活策略只能选择一个 provider（openai / anthropic / gemini / custom），下方新增的所有模型探活目标都会自动使用这个 provider，避免同一策略内混用不同厂商的模型。',
          probeInterval: '自动调度会按"上次探活时间 + 该间隔"判断是否到期；连续探活失败时后端还会额外叠加 2/5/10 分钟的递增退避。',
          dailyBudget: '限制当前 workspace 每天最多执行多少次真实探活请求；预算耗尽后会跳过真实探活请求，避免消耗过高，不代表系统异常。',
          failureThreshold: '连续软失败达到该次数后会暂停/降级对应链路；某些硬失败（如鉴权失败）可能不经过降级直接暂停。',
          successThreshold: '观察期内连续探活成功达到该次数后，才会判定链路真正恢复并回到健康状态。',
          cooldown: '链路被暂停后，在这段冷却时间结束前，调度器不会对其发起自动探活。',
          observation: '人工恢复或自动恢复流程触发后会进入观察期，这段时间的连续探活结果用于确认链路是否真的已经稳定。',
          recoveryStep: '恢复过程中每次探活成功会按该百分比逐步提高本地权重，不是一次性恢复到 100%。',
          autoDegrade: '开启后，探活结果会推进链路的健康状态机并调整本地转发权重；关闭后只记录探活结果，不会自动改变状态或权重。',
          autoRemoteAction: '开启后，状态机触发降级/恢复时会执行受支持的上游动作：Sub2API 当前分组健康目标会切换 admin 账号 active/inactive，priority 不会自动调整；NewAPI 旧对接链路路径可调整 channel 权重/状态。NewAPI 当前目标维度暂未实现远端动作，会记录为不支持。关闭后只记录探活和状态结果，不执行远端禁用/恢复。'
        },
        runFlow: {
          buttonLabel: '运行流程',
          title: '探活运行流程说明',
          subtitle: '面向后台管理员的完整机制说明，帮助理解策略、调度、状态机和手动探活之间的关系。',
          close: '关闭运行流程说明',
          steps: {
            policyScope: {
              title: '1. 策略如何生效',
              description: '分组健康使用独立探活：探活目标是当前 admin workspace 下 admin 分组内的账号(Sub2API)/渠道(NewAPI)本身，不依赖 real_connections 对接链路。账号/渠道只有在被显式分配策略后才会自动探活；探活模型来自已分配策略的启用模型目标。如果该目标自带模型列表（如 NewAPI channel 的 models），则取"目标模型 ∩ 策略模型池"的交集，否则直接使用策略模型池。'
            },
            modelProvider: {
              title: '2. 模型目标如何生效',
              description: '每个探活策略只对应一个 provider（openai / anthropic / gemini / custom），策略下新增的所有模型探活目标都属于这一个 provider，不会出现同一策略内混用多个 provider 的模型。自动调度和手动探活都会按上一步得出的候选模型逐一发起探活请求。'
            },
            schedulerCadence: {
              title: '3. 自动调度规则',
              description: '后端有一个独立的调度器，大约每 30 秒扫描一次当前 workspace 下所有可探活目标的探活任务。调度的最小粒度是"一个探活目标（账号/渠道）+ 一个模型"，同一个目标下的多个候选模型会被拆成多个独立任务分别判断是否需要探活。'
            },
            dueCheck: {
              title: '4. 到期判断',
              description: '从未探活过的（目标，模型）组合会被尽快安排一次探活；已经探活过的组合，则按"上次探活时间 + 策略配置的探活间隔"计算下一次到期时间，到期后才会被重新排入探活队列。连续探活失败时，调度器还会引入 2 分钟 / 5 分钟 / 10 分钟的递增退避，避免对持续异常的目标频繁重试。'
            },
            budget: {
              title: '5. 预算规则',
              description: '每条策略都配置了"每日探活预算"，用于限制当前 workspace 每天最多执行多少次真实探活请求。预算耗尽后，调度器会跳过真实探活请求，也不会写入新的探活事件——即使某个模型已经到期，也可能持续显示"已到期，等待调度"而没有新事件产生，这是预算限制导致的正常现象，不代表系统故障。'
            },
            stateTransition: {
              title: '6. 状态变化',
              description: '探活成功会清零该模型的连续失败计数；连续软失败（例如网络波动、限流等可恢复错误）在达到失败阈值前会先进入降级状态、按恢复步进百分比逐步降低本地权重，达到失败阈值后会暂停该模型；部分硬失败（例如鉴权失败、模型不存在）可能会跳过降级直接暂停。'
            },
            cooldownObservation: {
              title: '7. 冷却和观察',
              description: '目标/模型被暂停后会进入策略配置的冷却时间，冷却结束前调度器不会对其发起自动探活。冷却结束、或管理员手动点击"恢复"之后会进入观察阶段：这段时间内的连续探活结果用于判断目标是否真的恢复稳定，只有连续成功次数达到"恢复成功阈值"才会真正回到健康状态。'
            },
            autoDegradeVsRemoteAction: {
              title: '8. 自动降级和自动远端动作的区别',
              description: '自动降级只影响系统内部的状态机和本地展示权重，不会调用任何上游平台接口，属于低风险开关。自动远端动作只有策略显式开启（自动远端动作=开）且状态机判定需要远端动作时才会真实调用上游：NewAPI 对接链路路径会修改 channel 权重/状态；当前分组健康独立探活路径下，Sub2API target 会切换 admin 账号 active/inactive（不调整 priority），NewAPI target 维度暂未实现远端动作，会记录为 unsupported，不会真正调用上游。策略未开启自动远端动作时，两条路径都只记录 skipped，绝不调用任何上游接口。'
            },
            manualProbe: {
              title: '9. 手动探活',
              description: '手动探活是一次性即时测试，与策略自动探活完全隔离：打开弹窗后，后端会用该 targetId 重新解析出的凭据临时请求上游 /v1/models 现查可用模型列表，前端不接触 base_url/key。用户选择模型后点击"开始测试"，结果只显示在弹窗内，不写入策略探活状态/事件，不消耗策略预算，不触发自动降级/恢复。自动策略探活只针对已经显式分配了策略的账号/channel；未分配策略的账号/渠道仍可以随时手动一次性探活，只是不会被后台调度器自动探活。'
            },
            nextProbeCopy: {
              title: '10. "下次探活"文案说明',
              description: '"下次探活：已到期，等待调度"表示按时间计算已经到了应该探活的时间点，但实际执行还需要等待后台调度器的下一轮扫描（约 30 秒一次）、当前并发探活的名额、当日探活预算是否充足，以及该目标是否仍处于失败退避或冷却期内——这几个条件同时满足后才会真正发起一次探活请求。'
            }
          }
        },
        errors: {
          nameRequired: '请输入策略名称。',
          modelTargetRequired: '至少需要一个已填写模型名称的探活目标。',
          providerRequired: '请选择该策略的 provider。'
        }
      },
      probeDialog: {
        title: '选择探活模型',
        cancel: '取消',
        confirm: '开始探活',
        emptyTitle: '当前目标没有可探活的模型。',
        emptyHint: '请先在探活策略中添加并启用模型目标。',
        fromPolicy: '来自策略「{name}」',
        maxTokens: '探活上限 {count} token',
        remoteActionOn: '远端动作已开启',
        noResults: '探活已执行，但未获取到任何结果，请稍后重试或检查上游站点连通性。'
      },
      manualProbeDialog: {
        title: '手动一次性探活',
        loadingModels: '正在从上游获取可用模型列表...',
        retryLoad: '重新加载',
        empty: '未获取到任何可用模型。',
        selectHint: '勾选要测试的模型，可多选。',
        startTest: '开始测试',
        testing: '测试中...',
        resultTitle: '测试结果',
        resultEmpty: '尚未开始测试，选择模型后点击"开始测试"。',
        latency: '{ms}ms',
        selectedCount: '已选 {count} 个模型',
        close: '关闭'
      },
      policyAssignment: {
        title: '分配探活策略',
        subtitle: '分配后台策略探活关系',
        save: '保存',
        cancel: '取消',
        empty: '当前 workspace 暂无探活策略，请先创建策略。'
      },
      errors: {
        request: '操作失败，请稍后重试。',
        network: '网络异常，请检查连接后重试。',
        notFound: '探活目标不存在或无权访问。',
        noMatchingModels: '所选模型未匹配当前探活策略。',
        accountsFetch: '该分组账号列表加载失败。',
        targetNotFound: '探活目标不存在或不属于当前工作区。',
        credentialUnavailable: '无法安全获取上游凭据，暂不可探活。',
        secureVerificationRequired: '需要上游 root 安全验证后才能读取 channel key。',
        baseUrlUnavailable: '缺少可用的 Base URL，暂不可探活。',
        modelUnavailable: '没有可用的探活模型，请先在探活策略中配置。',
        exportUnavailable: '上游账号导出接口不可用，无法获取凭据。',
        credentialsRedacted: '上游凭据已脱敏，无法用于探活。',
        modelListUnavailable: '无法获取上游模型列表，请稍后重试。',
        modelListInvalid: '上游模型列表响应格式无法识别。',
        manualModelsRequired: '请至少选择一个模型再开始测试。',
        policyNotFound: '所选策略不存在或不属于当前工作区。'
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
            tokenHelp: '适用于 Cloudflare 或二次验证导致账号密码无法直连的 Sub2API 站点。'
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
          rechargeRate: '充值倍率',
          rechargeRatePlaceholder: '输入 USD 转 CNY 的倍率，如 7',
          rechargeRateHelp: '必填。人民币金额 = USD 金额 × 此倍率。',
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
        isConnected: '是否对接',
        connected: '已对接',
        disconnected: '未对接',
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
        network: '网络或 CORS 请求失败，请检查站点地址与跨域配置。',
        auth: '登录失败，请检查账号或密码。',
        request: '上游接口请求失败，请稍后重试。',
        invalidResponse: '上游返回内容无法解析。',
        tokenMissing: '登录成功但未返回访问令牌。',
        detect: '无法自动识别平台，请手动选择平台后重试。',
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
        delta: '涨跌幅',
        updatedAt: '更新时间',
        actions: '操作'
      },
      actions: {
        refresh: '刷新数据',
        createCampaign: '创建活动',
        viewHistory: '查看历史',
        viewHistoryForRate: '查看 {site} · {group} 的涨跌幅历史，当前涨跌幅 {delta}',
        closeHistory: '关闭历史',
        editType: '修改',
        closeEdit: '关闭修改分组类型',
        connect: '点击对接',
        closeConnect: '关闭对接窗口',
        saveConnect: '确认对接',
        cancel: '取消',
        saveType: '保存类型'
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
        mapped: '已对接',
        unmapped: '未对接',
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
        mapped: '已对接',
        unmapped: '未对接',
        deleted: '已删除'
      },
      empty: {
        title: '暂无分组倍率',
        description: '同步上游站点后，这里会显示分组倍率数据。'
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
      edit: {
        title: '修改分组类型',
        titleWithGroup: '修改 {site} · {group} 的分组类型',
        description: '保存后会更新该站点分组的倍率类型，并刷新列表。',
        typeLabel: '分组类型',
        typePlaceholder: '请选择分组类型'
      },
      connect: {
        titleWithGroup: '对接 {site} · {group}',
        description: '选择我的站点分组后，会把当前上游分组加入该分组的对接关系。',
        ownGroupLabel: '我的站点分组',
        ownGroupPlaceholder: '请选择我的站点分组',
        upstreamGroupLabel: '对接分组',
        upstreamGroupPlaceholder: '请选择对接分组',
        upstreamSiteLabel: '上游站点',
        upstreamGroupNameLabel: '上游分组',
        upstreamMultiplierLabel: '上游倍率',
        upstreamPlatformLabel: '平台',
        modeData: '数据统计',
        modeReal: '真实对接',
        realDescription: '将自动在上游站点创建 API Key，并在 admin 站点创建对应的转发账号。',
        groupTypeLabel: '分组类型',
        groupTypePlaceholder: '请选择分组类型',
        groupTypeOpenai: 'OpenAI',
        groupTypeAnthropic: 'Anthropic',
        groupTypeGemini: 'Gemini',
        groupTypeAntigravity: 'Antigravity',
        channelTypeLabel: '渠道类型',
        channelTypePlaceholder: '请选择渠道类型',
        realNotSupported: '当前平台不支持真实对接',
        realConnecting: '正在创建对接...',
        realSuccess: '真实对接创建成功',
        realFailed: '真实对接创建失败',
        modeBind: '手动绑定',
        bindDescription: '选择已有的上游 Key 绑定到当前分组，不会创建新资源。',
        bindSelectKey: '选择上游 Key',
        bindKeysLoading: '正在加载 Key 列表...',
        bindKeysEmpty: '该站点暂无可用 Key',
        bindFailed: '手动绑定失败'
      },
      disconnect: {
        action: '取消对接',
        title: '取消对接',
        description: '确认取消 {site} · {group} 的真实对接？',
        unlinkOnly: '仅取消关联',
        unlinkOnlyHint: '仅删除本地绑定记录，保留上游 Key 和 Admin 账号',
        deleteAll: '删除账号和 Key',
        deleteAllHint: '同时删除上游 Key 和 Admin 站点的转发账号',
        confirm: '确定',
        disconnecting: '正在取消对接...',
        failed: '取消对接失败'
      },
      format: {
        multiplier: '{value}x',
        deltaMultiplier: '{value}x'
      },
      errors: {
        network: '网络或 CORS 请求失败，请检查接口地址与跨域配置。',
        request: '分组倍率接口请求失败，请稍后重试。',
        unknown: '加载分组倍率时发生未知错误。',
        refreshFailed: '变更已保存，但列表刷新失败。请重新刷新以更新视图。'
      }
    },
    groupRateCampaigns: {
      title: '活动调价',
      subtitle: '批量调整自有分组倍率，支持定时开始/结束并自动恢复原倍率。',
      common: {
        placeholder: '—'
      },
      actions: {
        create: '新建活动',
        refresh: '刷新',
        start: '立即开始',
        end: '结束活动',
        cancel: '取消活动',
        viewDetail: '查看详情',
        close: '关闭',
        preview: '预览影响',
        confirmCreate: '创建活动',
        cancelEdit: '取消'
      },
      tabs: {
        all: '全部'
      },
      status: {
        draft: '草稿',
        scheduled: '待开始',
        running: '进行中',
        ending: '结束中',
        ended: '已结束',
        partial: '部分成功',
        failed: '失败',
        cancelled: '已取消',
        loading: '正在加载活动...'
      },
      fields: {
        name: '活动名称',
        status: '状态',
        startAt: '开始时间',
        endAt: '结束时间',
        summary: '执行结果',
        createdBy: '创建人',
        actions: '操作'
      },
      empty: {
        title: '暂无活动',
        description: '点击"新建活动"创建第一个批量调价活动。'
      },
      pagination: {
        total: '共 {total} 个',
        pageSize: '每页 {pageSize} 条',
        currentPage: '第 {page} / {totalPages} 页',
        previous: '上一页',
        next: '下一页'
      },
      format: {
        summary: '{applied}/{total} 已生效'
      },
      errors: {
        network: '网络或 CORS 请求失败，请检查接口地址与跨域配置。',
        request: '活动调价接口请求失败，请稍后重试。',
        unknown: '加载活动调价时发生未知错误。',
        emptySelection: '请至少选择一个分组，且分组必须存在于自有分组中。',
        invalidName: '活动名称无效，请检查长度是否在 1-80 个字符之间。',
        invalidAdjustment: '活动倍率无效，请检查每个分组是否填写了有效的固定倍率。',
        invalidSchedule: '时间计划无效，请检查开始/结束时间设置。',
        noNotifyBots: '开启通知后请至少选择一个机器人。',
        notFound: '活动不存在。',
        invalidState: '当前活动状态不支持该操作。',
        duplicateGroup: '同一个分组不能重复选择。'
      },
      editor: {
        titleCreate: '新建活动调价',
        sectionInfo: '活动信息',
        nameLabel: '活动名称',
        namePlaceholder: '例如：双十一活动',
        descriptionLabel: '活动描述',
        descriptionPlaceholder: '选填，方便自己识别活动用途',
        sectionSelection: '选择分组',
        selectionHint: '每个分组单独设置活动倍率',
        groupsEmpty: '暂无可选分组',
        groupMultiplierPlaceholder: '活动倍率',
        sectionSchedule: '时间计划',
        startModeLabel: '开始方式',
        startNow: '立即开始',
        startScheduled: '定时开始',
        startDraft: '保存为草稿',
        startAtLabel: '开始时间',
        endModeLabel: '结束方式',
        endScheduled: '定时结束',
        endManual: '手动结束',
        endAtLabel: '结束时间',
        sectionNotify: '通知',
        notifyEnableLabel: '开启机器人通知',
        notifyBotSelectLabel: '选择机器人',
        notifyNoBots: '暂无可用机器人，请先在系统设置中配置。',
        notifyStartTemplateLabel: '开始通知文案',
        notifyEndTemplateLabel: '结束通知文案',
        notifyVariablesTitle: '可用变量，点击复制',
        notifyVarActivityName: '活动名称',
        notifyVarTotalCount: '目标分组总数',
        notifyVarAppliedCount: '已生效数量',
        notifyVarFailedCount: '失败数量',
        notifyVarStartTime: '开始时间',
        notifyVarEndTime: '结束时间',
        copyVarFailed: '复制失败，请手动复制变量。',
        previewTitle: '预览影响的分组',
        previewEmpty: '暂无预览结果，点击"预览影响"查看',
        previewGroupName: '分组名称',
        previewOriginal: '原倍率',
        previewCampaign: '活动倍率',
        previewTotal: '共 {total} 个分组受影响',
        errors: {
          nameRequired: '请输入活动名称',
          selectionEmpty: '请至少选择一个分组',
          groupMultiplierInvalid: '请为每个分组填写有效活动倍率',
          scheduleInvalid: '请检查开始/结束时间设置',
          notifyBotsRequired: '开启通知后请至少选择一个机器人'
        }
      },
      detail: {
        title: '活动详情',
        sectionConfig: '活动配置',
        sectionItems: '分组明细',
        itemGroupName: '分组名称',
        itemOriginal: '原倍率',
        itemCampaign: '活动倍率',
        itemRestored: '恢复倍率',
        itemApplyStatus: '开始状态',
        itemRestoreStatus: '恢复状态',
        noReason: '—',
        confirmEnd: '确定要手动结束该活动吗？将立即尝试恢复所有分组的原倍率。',
        confirmCancel: '确定要取消该活动吗？取消后不会执行任何调价操作。'
      }
    },
    mySites: {
      errors: {
        invalidAutoPricingConfig: '自动调价配置无效：主上游不在关联上游中，或最低倍率大于最高倍率。'
      }
    },
    tickets: {
      title: '工单',
      subtitle: '查看并回复通过 iframe 提交的用户工单。',
      common: {
        placeholder: '—'
      },
      actions: {
        refresh: '刷新',
        viewDetail: '查看详情',
        embedSettings: '嵌入设置'
      },
      tabs: {
        all: '全部'
      },
      status: {
        open: '待处理',
        pending: '待跟进',
        replied: '已回复',
        closed: '已关闭',
        loading: '正在加载工单...'
      },
      fields: {
        title: '标题',
        status: '状态',
        category: '分类',
        priority: '优先级',
        manualEmail: '联系邮箱',
        sub2apiUser: 'Sub2API 用户',
        sub2apiSrcHost: '来源域名',
        lastMessageAt: '最后回复时间',
        actions: '操作'
      },
      empty: {
        title: '暂无工单',
        description: '当前工作区还没有收到任何工单。'
      },
      pagination: {
        total: '共 {total} 个',
        pageSize: '每页 {pageSize} 条',
        currentPage: '第 {page} / {totalPages} 页',
        previous: '上一页',
        next: '下一页'
      },
      errors: {
        network: '网络或 CORS 请求失败，请检查接口地址与跨域配置。',
        request: '工单接口请求失败，请稍后重试。',
        unknown: '加载工单时发生未知错误。',
        notFound: '工单不存在。',
        invalidStatus: '无效的工单状态。',
        bodyRequired: '回复内容不能为空。',
        ticketClosed: '工单已关闭，无法继续回复。',
        noCurrentAccount: '请先选择一个工作区。',
        invalidTemplate: '不支持的嵌入页面模板。',
        invalidMaxImages: '每次工单最多上传图片数必须在 0-9 之间。',
        attachmentLoadFailed: '图片加载失败，请稍后重试。',
        invalidCategoryOptions: '分类选项无效，请检查是否为空、重复或超出数量/长度限制。',
        invalidPriorityOptions: '优先级选项无效，请检查是否为空、重复或超出数量/长度限制。'
      },
      detail: {
        title: '工单详情',
        sectionTicket: '工单信息',
        sectionMessages: '回复记录',
        sectionReply: '回复',
        category: '分类',
        priority: '优先级',
        manualEmail: '联系邮箱',
        lastMessageAt: '最后回复时间',
        sub2apiUserId: 'Sub2API 用户 ID',
        sub2apiEmail: 'Sub2API 邮箱',
        sub2apiRole: 'Sub2API 角色',
        sub2apiSrcHost: '来源域名',
        authorAdmin: '客服',
        authorCustomer: '用户',
        replyPlaceholder: '输入回复内容...',
        send: '发送回复',
        attachmentLoadFailed: '图片加载失败',
        previewImage: '放大预览',
        closePreview: '关闭预览'
      },
      embedConfig: {
        title: '嵌入设置',
        sections: {
          basic: '基础设置',
          category: '分类',
          priority: '优先级'
        },
        legacyNotice: '"启用嵌入工单"和"允许来源域名"配置已取消，嵌入地址始终可用，如需限制访问范围请联系管理员评估其它方案。',
        embedUrl: '嵌入地址',
        embedUrlHint: '将此地址配置到 Sub2API 自定义 iframe 中，Sub2API 会自动追加用户身份参数。',
        copy: '复制',
        copied: '已复制',
        copyFailed: '复制失败，请手动复制。',
        openPreview: '打开工单页面',
        openPreviewHint: '在新标签页预览嵌入页面。非 Sub2API iframe 环境打开时会缺少身份参数，属正常现象。',
        template: '页面模板',
        templates: {
          default: {
            name: '默认紧凑',
            description: '标准圆角卡片风格，适合默认使用。'
          },
          minimal: {
            name: '极简轻量',
            description: '更轻量的视觉密度，适合嵌入已有后台风格。'
          },
          support: {
            name: '客服面板',
            description: '更突出对话感，适合作为独立客服面板使用。'
          }
        },
        maxImages: '每次工单最多上传图片数',
        maxImagesHint: '0 表示关闭图片上传，最多允许 9 张。',
        categoryOptions: '分类选项',
        priorityOptions: '优先级选项',
        addOption: '添加选项',
        addOptionPlaceholder: '输入新选项后点击添加',
        removeOption: '删除该选项',
        restoreDefaults: '恢复默认值',
        optionsHint: '至少保留 1 项，单项最多 40 个字符。客户创建工单时必须从这里选择。',
        saveTemplate: '保存设置',
        saving: '保存中...',
        saveSuccess: '已保存',
        rotateToken: '轮换嵌入地址',
        confirmRotate: '确定要轮换嵌入地址吗？旧的嵌入地址将立即失效。'
      },
      sub2apiProfile: {
        title: 'Sub2API 用户资料',
        sectionIdentity: '身份信息',
        userId: '用户 ID',
        email: '邮箱',
        role: '角色',
        srcHost: '来源域名',
        username: '用户名',
        status: '账号状态',
        sectionBalance: '余额与充值',
        balance: '当前余额',
        totalRecharged: '总充值额度',
        registeredAt: '注册时间',
        frozenBalance: '冻结余额',
        concurrency: '并发数',
        rpmLimit: 'RPM 限制',
        lastUsedAt: '最后使用时间',
        unavailable: '暂不可用',
        sectionRechargeHistory: '充值记录',
        rechargeHistoryComingSoon: '暂未提供',
        historyEmpty: '暂无充值记录',
        empty: '暂无数据',
        remoteUnavailable: {
          noUserId: '该工单未记录 Sub2API 用户 ID，无法查询实时资料。',
          noAdminSession: '当前工作区尚未登录 Sub2API 管理员账号，以下仅展示工单快照。',
          userNotFound: '未能从 Sub2API 获取该用户的实时资料，以下仅展示工单快照。'
        }
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
        itemGeneric: '该收件人发送失败，请稍后查看批次获取最新详情。'
      }
    },
    settings: {
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
      balanceTemplateVars: '(支持变量: {siteName}, {balance}, {threshold})',
      multiplierTemplateVars: '(支持变量: {siteName}, {groupName}, {oldRate}, {newRate}, {changeDirection})',
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
      pricingAmount: '调价幅度',
      botNameLabel: '机器人名称标识',
      botNameDingtalkPlaceholder: '例如：钉钉主群',
      botNameFeishuPlaceholder: '例如：飞书主群',
      botNameTelegramPlaceholder: '例如：TG主群',
      addDingtalkBot: '添加钉钉机器人',
      addFeishuBot: '添加飞书机器人',
      addTelegramBot: '添加 TG 机器人',
      emptyDingtalk: '暂无钉钉机器人配置',
      emptyFeishu: '暂无飞书机器人配置',
      emptyTelegram: '暂无 Telegram 机器人配置',
      tabs: {
        strategy: '自动化与策略',
        channels: '通知与渠道',
        templates: '消息模板',
        email: '邮件设置'
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
          multiplierChangeWarningHelp: '当监控的对接分组倍率发生任何变动时，通过机器人发送通知。'
        },
        pricing: {
          title: '分组倍率调价',
          description: '配置对接后的某个分组在倍率上涨时的自动处理策略。',
          enableAutoPricing: '自动调价',
          enableAutoPricingHelp: '当对接的上游分组倍率上涨时，自动调整"我的分组"的倍率。',
          strategy: '调价策略',
          strategyFixed: '固定涨幅 (+)',
          strategyPercentage: '百分比涨幅 (%)',
          fixedValuePlaceholder: '例如 0.1',
          percentageValuePlaceholder: '例如 10'
        },
        channels: {
          title: '通知渠道配置',
          description: '配置接收系统报警的第三方渠道参数（如钉钉、TG、飞书）。',
          dingtalk: '钉钉机器人',
          dingtalkHelp: '配置钉钉群机器人的 Webhook 和加签密钥。',
          feishu: '飞书机器人',
          feishuHelp: '配置飞书群机器人的 Webhook 和加签密钥。',
          telegram: 'Telegram 机器人',
          telegramHelp: '配置 Telegram Bot Token 和接收消息的 Chat ID。',
          webhookUrl: 'Webhook 地址',
          secret: '加签密钥 (Secret)',
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
          balanceTemplatePlaceholder: '例如：【余额预警】{siteName} 站点余额（CNY）已不足 {threshold} 元，当前余额为 {balance} 元。',
          multiplierTemplatePlaceholder: '例如：【倍率变更】{siteName} 的 {groupName} 分组倍率已从 {oldRate}x 变为 {newRate}x。'
        }
      },
      errors: {
        network: '网络或 CORS 请求失败，请检查接口地址与跨域配置。',
        request: '通知渠道测试请求失败，请稍后重试。',
        unknown: '测试通知渠道时发生未知错误。',
        invalidChannel: '通知渠道类型无效。',
        missingWebhook: '请先填写机器人 Webhook 地址。',
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
      openRelease: '查看发布说明',
      openGithubRepository: '在 GitHub 上查看源码',
      errors: {
        network: '系统信息请求失败，请检查网络连接。',
        request: '系统请求失败，请稍后重试。'
      }
    }
  },
  embed: {
    tickets: {
      page: {
        loading: '正在加载工单系统...'
      },
      list: {
        title: '我的工单',
        refresh: '刷新',
        create: '新建工单',
        loading: '正在加载工单...',
        emptyTitle: '暂无工单',
        emptyDescription: '点击"新建工单"提交你的第一个问题。'
      },
      createModal: {
        title: '新建工单'
      },
      form: {
        manualEmail: '联系邮箱',
        manualEmailPlaceholder: '请输入接收回复的邮箱',
        title: '标题',
        titlePlaceholder: '简要描述你的问题',
        body: '问题详情',
        bodyPlaceholder: '请详细描述遇到的问题',
        category: '分类',
        categoryPlaceholder: '请选择分类',
        priority: '优先级',
        priorityPlaceholder: '请选择优先级',
        submit: '提交工单',
        cancel: '取消',
        images: '图片',
        imagesCount: '{count} / {max} 张',
        addImage: '添加图片',
        imagesHint: '仅支持 JPEG/PNG/WEBP/GIF 格式，单张不超过 5MB。'
      },
      detail: {
        back: '返回列表',
        loading: '正在加载工单详情...',
        support: '客服',
        you: '我',
        replyPlaceholder: '输入回复内容...',
        send: '发送',
        closedNotice: '该工单已关闭，无法继续回复。'
      },
      attachments: {
        loadFailed: '图片加载失败'
      },
      status: {
        open: '待处理',
        pending: '待跟进',
        replied: '已回复',
        closed: '已关闭'
      },
      errors: {
        network: '网络或 CORS 请求失败，请检查接口地址与跨域配置。',
        request: '工单接口请求失败，请稍后重试。',
        unknown: '发生未知错误，请稍后重试。',
        missingParams: '当前打开方式缺少必要参数，无法建立会话。如果你是通过"打开工单页面"预览按钮打开的，这是正常现象——请在真实的 Sub2API iframe 环境中打开本页面。',
        formIncomplete: '请填写联系邮箱、标题、问题详情，并选择分类和优先级。',
        configNotFound: '嵌入配置不存在。',
        disabled: '工单功能已被管理员关闭。',
        invalidSrcHost: '来源地址无效。',
        srcHostMismatch: '来源域名不受信任。',
        sub2apiAuth: '身份校验失败，请刷新页面重试。',
        sub2apiRequest: '获取用户信息失败，请稍后重试。',
        userMismatch: '用户身份校验失败。',
        sessionInvalid: '会话已过期，请刷新页面重试。',
        invalidEmail: '请输入有效的邮箱地址。',
        titleRequired: '请输入标题。',
        bodyRequired: '请输入问题详情。',
        categoryRequired: '请选择分类。',
        priorityRequired: '请选择优先级。',
        invalidCategory: '所选分类不属于当前工单系统的配置，请重新选择。',
        invalidPriority: '所选优先级不属于当前工单系统的配置，请重新选择。',
        ticketClosed: '工单已关闭，无法继续回复。',
        tooManyImages: '图片数量超过当前允许的上限。',
        invalidImageType: '仅支持 JPEG/PNG/WEBP/GIF 格式的图片。',
        imageTooLarge: '单张图片大小不能超过 5MB。',
        emptyImage: '图片内容为空，请重新选择。',
        attachmentLoadFailed: '图片加载失败，请稍后重试。'
      }
    },
    leaderboard: {
      eyebrow: '用量排行',
      title: 'Token 排行榜',
      subtitle: '按实际 Token 使用量查看用户排名。',
      refresh: '刷新排行榜',
      errorTitle: '排行榜加载失败',
      emptyTitle: '当前周期暂无用量',
      emptyDescription: '切换统计周期或稍后刷新后再查看。',
      anonymous: '用户 {id}',
      podiumLabel: '前三名用户',
      updatedAt: '更新于 {time}',
      period: { label: '统计周期', today: '今日', '7d': '7 天', '30d': '30 天' },
      metrics: { tokens: 'Token', requests: '请求数', cost: '实际消费' },
      table: { title: '完整排名', caption: '共 {count} 位用户', rank: '排名', user: '用户' },
      errors: {
        network: '网络请求失败，请稍后重试。',
        request: '排行榜请求失败，请稍后重试。',
        missingParams: '当前页面缺少 iframe 会话参数，请从 Sub2API 自定义菜单中打开。',
        configNotFound: '排行榜嵌入配置不存在或尚未绑定来源站点。',
        invalidSrcHost: '来源地址无效。',
        srcHostMismatch: '当前来源站点与嵌入配置不匹配。',
        sourceBinding: '排行榜嵌入来源已变更，请联系管理员重新保存嵌入设置。',
        sub2apiAuth: '身份校验失败，请刷新页面重试。',
        sub2apiRequest: '无法连接来源站点，请稍后重试。',
        userMismatch: '用户身份校验失败。',
        sessionInvalid: '会话已过期，请刷新页面重试。',
        adminSession: '管理员会话不可用，请联系站点管理员重新连接工作区。',
        upstreamUnsupported: '当前 Sub2API 版本不支持排行榜，请联系管理员升级。',
        upstreamRequest: '排行榜数据暂时不可用，请稍后重试。'
      }
    },
    lottery: {
      eyebrow: '抽奖报名',
      title: '抽奖活动',
      subtitle: '参与开放中的活动，保存报名凭证哈希，并在开奖后查看结果。',
      page: {
        loading: '正在加载抽奖活动...'
      },
      common: {
        empty: '无',
        noDescription: '暂无描述。'
      },
      list: {
        title: '活动列表',
        count: '{count} 项',
        loading: '正在加载活动...',
        empty: '暂无可公开访问的抽奖活动。'
      },
      detail: {
        loading: '正在加载活动详情...',
        empty: '请选择一个活动查看详情。'
      },
      sections: {
        schedule: '时间安排',
        prizes: '奖品',
        winners: '公开中奖者',
        integrity: '开奖完整性',
        myEntry: '我的报名',
        myResult: '我的结果',
        entries: '公开报名名册'
      },
      metrics: {
        entries: '{count} 个报名',
        winners: '{count} 个中奖者',
        winnersLabel: '中奖者'
      },
      actions: {
        refresh: '刷新',
        enter: '参与活动',
        withdraw: '撤回报名',
        browseCampaigns: '浏览其他活动',
        returnToDraw: '返回我的开奖场',
        copyVoucher: '复制兑换券'
      },
      transparency: {
        title: '公开透明开奖场',
        description: '报名名册、凭证哈希、随机种子承诺和开奖快照均可公开核验。',
        activeEntries: '{count} 个有效报名',
        algorithmV2: 'v2 使用公开名册中的报名 ID、脱敏邮箱和凭证哈希生成快照，再以公开种子执行确定性 HMAC-SHA256 洗牌。',
        algorithmLegacy: '该活动使用兼容旧版的 v1 算法；仍可核验种子承诺和最终结果，但快照包含旧版内部标识。'
      },
      countdown: {
        opensIn: '报名开始倒计时',
        closesIn: '报名截止倒计时',
        drawsIn: '开奖倒计时',
        drawTime: '开奖时间',
        noTimer: '倒计时',
        days: '{days} 天 {hours} 小时 {minutes} 分 {seconds} 秒',
        hours: '{hours} 小时 {minutes} 分 {seconds} 秒',
        minutes: '{minutes} 分 {seconds} 秒',
        seconds: '{seconds} 秒'
      },
      drawReveal: {
        ariaLabel: '抽奖结果揭晓',
        countdown: {
          eyebrow: '开奖倒计时',
          description: '报名快照已锁定，准备随机抽取。'
        },
        drawing: {
          eyebrow: '公平开奖',
          title: '正在抽取幸运用户',
          description: '正在根据公开算法计算本期结果...'
        },
        won: {
          eyebrow: '恭喜中奖',
          title: '你抽中了「{prize}」',
          description: '中奖记录已经确认，奖励发放状态可在结果页查看。'
        },
        lost: {
          eyebrow: '开奖结果',
          title: '这次没有抽中',
          description: '感谢参与本期活动，期待下次与你分享好运。'
        },
        spectator: {
          eyebrow: '开奖完成',
          title: '本期结果已经揭晓',
          description: '公开中奖名单和开奖完整性信息已经更新。'
        },
        viewResult: '查看开奖结果'
      },
      status: {
        draft: '草稿',
        scheduled: '待开放',
        open: '报名中',
        closed: '报名已关闭',
        drawing: '开奖中',
        drawn: '已开奖',
        fulfilling: '发奖中',
        completed: '已完成',
        partial: '部分完成',
        cancelled: '已取消'
      },
      drawMode: {
        manual: '手动开奖',
        scheduled: '定时开奖'
      },
      prizeType: {
        balance: '余额',
        subscription: '订阅'
      },
      deliveryMode: {
        sub2api_auto: 'Sub2API 自动发放',
        voucher: '中奖后领取兑换券',
        manual: '联系管理员人工兑换'
      },
      entryStatus: {
        active: '有效',
        withdrawn: '已撤回'
      },
      rewardStatus: {
        pending: '待处理',
        processing: '处理中',
        fulfilled: '已发放',
        retryable_failed: '可重试失败',
        manual_attention: '需人工处理',
        failed: '失败'
      },
      fields: {
        algorithmVersion: '算法版本',
        drawAt: '开奖时间',
        deliveryMode: '领取方式',
        enteredAt: '报名时间',
        entrySnapshotHash: '报名快照哈希',
        entryId: '报名 ID',
        entryStatus: '报名状态',
        prize: '奖品',
        prizeSlot: '奖品序号',
        receiptHash: '报名凭证哈希',
        registrationEnd: '报名结束',
        registrationStart: '报名开始',
        revealedSeed: '公开种子',
        rewardMessage: '奖励消息',
        rewardStatus: '奖励状态',
        seedCommitment: '种子承诺',
        voucherCode: '我的兑换券',
        manualContact: '兑换联系方式'
      },
      prizes: {
        empty: '该活动尚未公开奖品。',
        quantity: 'x {count}',
        balanceValue: '余额金额：{amount}',
        subscriptionValue: '分组：{group}，奖励倍率：{multiplier}，有效期 {days} 天'
      },
      winners: {
        private: '中奖者展示由活动发起方控制。',
        empty: '暂无公开中奖者。',
        count: '{count} 个公开中奖者',
        row: '{email} 获得 {prize} #{slot}'
      },
      entries: {
        description: '邮箱已脱敏；每条凭证哈希可用于核对报名与开奖快照。',
        count: '{active} 个有效 / {total} 个记录',
        empty: '当前还没有报名记录。'
      },
      entry: {
        none: '你尚未参与此活动。'
      },
      result: {
        none: {
          title: '尚未报名',
          description: '在报名开放期间参与活动即可获得报名凭证哈希。'
        },
        pending: {
          title: '结果待开奖',
          description: '你的报名仍然有效，开奖后刷新即可查看结果。'
        },
        won: {
          title: '你已中奖',
          description: '中奖奖品和奖励发放状态显示在下方。'
        },
        lost: {
          title: '未被抽中',
          description: '本次开奖已完成，你的报名未被选中。'
        },
        withdrawn: {
          title: '报名已撤回',
          description: '当前活动中，撤回后的报名结果不可重新参与。'
        }
      },
      errors: {
        title: '抽奖加载失败',
        network: '网络请求失败，请检查连接后重试。',
        request: '抽奖接口请求失败，请稍后重试。',
        missingParams: '当前页面缺少 iframe 会话参数，请从 Sub2API 自定义菜单中打开。',
        configNotFound: '抽奖嵌入配置不存在或尚未绑定来源站点。',
        invalidSrcHost: '来源地址无效。',
        srcHostMismatch: '当前来源站点与嵌入配置不匹配。',
        sub2apiAuth: '身份校验失败，请刷新页面重试。',
        sub2apiRequest: '无法连接来源站点，请稍后重试。',
        userMismatch: '用户身份校验失败。',
        userInactive: '来源账号已停用。',
        sessionInvalid: '会话已过期，请刷新页面重试。',
        adminSession: '管理员会话不可用，请联系站点管理员重新连接工作区。',
        sourceBinding: '抽奖嵌入来源已变更，请联系管理员重新保存嵌入设置。',
        campaignNotOpen: '该活动当前未开放报名。',
        entryNotFound: '未找到此活动的有效报名。',
        upstreamRequest: '来源站点数据暂时不可用，请稍后重试。',
        alreadyEntered: '你已经参与过此活动。',
        copy: '复制失败，请手动选择兑换券。'
      }
    }
  }
}
