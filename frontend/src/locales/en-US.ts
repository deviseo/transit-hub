export default {
  brand: {
    name: 'TransitHub',
    logoAlt: 'TransitHub logo'
  },
  nav: {
    features: 'Features',
    integrations: 'Integrations',
    documentation: 'Documentation',
    pricing: 'Pricing',
    signIn: 'Sign In',
    getStarted: 'Get Started'
  },
  hero: {
    badge: 'Introducing TransitHub 2.0',
    title: 'The Ultimate',
    highlight: 'API Gateway',
    subtitle: 'Unify your NewAPI instances, manage keys with ease, and route traffic intelligently. Built for the modern AI infrastructure.',
    startBtn: 'Start Building Now',
    docBtn: 'View Documentation'
  },
  features: {
    title: 'Designed for Scale and Speed',
    subtitle: 'Everything you need to manage massive API traffic across distributed networks, packed into one beautiful interface.',
    items: {
      sync: {
        title: 'Multi-Instance Sync',
        desc: 'Seamlessly synchronize across multiple NewAPI instances with zero downtime and automatic conflict resolution.'
      },
      fallback: {
        title: 'Smart Fallback',
        desc: 'Intelligent routing and automatic fallback ensures your API requests never fail even if a provider goes down.'
      },
      observe: {
        title: 'Global Observability',
        desc: 'Monitor all your API keys, quotas, and latency metrics in real-time across the globe.'
      },
      selfhost: {
        title: 'Self-Hosted Ready',
        desc: 'Deploy anywhere. Full support for Docker, Kubernetes, and bare-metal VPS installations.'
      }
    }
  },
  cta: {
    title: 'Ready to take control?',
    subtitle: 'Join thousands of developers using TransitHub to power their API infrastructure. Get started for free today.',
    deployBtn: 'Deploy Now',
    salesBtn: 'Contact Sales'
  },
  footer: {
    rights: 'TransitHub Operations. All rights reserved.'
  },
  auth: {
    backToHome: 'Back to Home',
    login: {
      title: 'Welcome Back',
      subtitle: 'Enter your email and password to sign in',
      email: 'Email',
      emailPlaceholder: "name{'@'}example.com",
      password: 'Password',
      passwordPlaceholder: 'Enter your password',
      submit: 'Sign In',
      submitting: 'Signing in...',
      success: 'Signed in successfully. Opening the admin console...',
      errors: {
        login: 'Unable to sign in. Check your email and password, then try again.'
      },
      noAccount: 'Don\'t have an account?',
      registerLink: 'Register now'
    },
    register: {
      title: 'Create an Account',
      subtitle: 'Enter your details to register for TransitHub',
      email: 'Email',
      emailPlaceholder: "name{'@'}example.com",
      password: 'Password',
      passwordPlaceholder: 'Set a password',
      code: 'Verification Code',
      codePlaceholder: 'Enter 6-digit code',
      sendCode: 'Send Code',
      sendingCode: 'Sending...',
      codeSent: 'Sent',
      codeSentSuccess: 'Verification code sent. Use {code} to finish registration.',
      submit: 'Register',
      submitting: 'Registering...',
      success: 'Registration complete. Opening the admin console...',
      errors: {
        codeRequest: 'Unable to send the verification code. Check your email and try again.',
        register: 'Unable to register. Check the verification code and try again.'
      },
      hasAccount: 'Already have an account?',
      loginLink: 'Sign In'
    },
    errors: {
      emailRequired: 'Enter an email address first.',
      invalidRegister: 'Enter your email, password, and verification code.',
      invalidLogin: 'Enter your email and password.',
      invalidCode: 'The verification code is incorrect or expired.',
      emailExists: 'This email is already registered.',
      invalidCredentials: 'Email or password is incorrect.',
      unauthorized: 'Your session has expired. Please sign in again to continue.',
      registrationDisabled: 'Public registration is disabled for this deployment. Sign in with the admin account.',
      network: 'Network error. Check your connection and try again.',
      unknown: 'Something went wrong. Please try again.'
    }
  },
  admin: {
    layout: {
      toggleLanguage: 'Toggle language',
      toggleTheme: 'Toggle theme',
      userProfile: 'User profile',
      switchWorkspace: 'Switch Workspace',
      skipToContent: 'Skip to main content',
      openNavigation: 'Open navigation',
      closeNavigation: 'Close navigation'
    },
    menu: {
      dashboard: 'Dashboard',
      upstream: 'Upstream',
      groupRates: 'Group Rates',
      settings: 'Settings',
      userActivity: 'User Analytics',
      sales: 'Sales',
      profit: 'Profit',
      massEmail: 'Mass Email',
      tickets: 'Tickets',
      client: 'Client',
      signOut: 'Sign Out'
    },
    adminAccounts: {
      title: 'Select Workspace',
      subtitle: 'Choose an admin workspace to continue, or add a new one.',
      empty: 'No workspaces yet. Add your first workspace to get started.',
      currentLabel: 'Current workspace',
      addWorkspace: 'Add Workspace',
      addWorkspaceHint: 'Connect a new site admin account',
      creating: 'Creating workspace...',
      delete: {
        actionLabel: 'Delete workspace {name}',
        title: 'Delete {name}',
        localDataWarning: 'All TransitHub local workspace data for this workspace will be permanently deleted. This cannot be undone.',
        remoteResourcesRetained: 'Remote upstream resources and accounts are retained and will not be deleted.',
        phraseInstruction: 'Type the exact phrase below to confirm: {phrase}',
        inputLabel: 'Confirmation phrase',
        inputPlaceholder: 'Type the phrase manually',
        cancel: 'Cancel',
        confirm: 'Delete Workspace',
        deleting: 'Deleting...',
        cleanupPending: 'Workspace deletion completed, but local runtime, cache, or file cleanup is still pending and will retry.'
      },
      errors: {
        noCurrentAccount: 'Please select a workspace first.',
        notFound: 'Workspace not found.',
        deleteFailed: 'Workspace deletion failed. Type the confirmation phrase again and retry.',
        request: 'Operation failed. Please try again.',
        network: 'Network error. Check your connection and try again.'
      }
    },
    dashboard: {
      metrics: {
        todayProfit: "Today's Revenue",
        siteBalance: 'Site User Balance',
        todayPurchase: "Today's Cost",
        netProfit: "Today's Net Profit",
        upstreamBalance: 'Upstream Total Balance',
        profitMargin: "Today's Margin",
        groupCount: 'My Groups',
        groupCountCaption: 'Click to view group details'
      },
      charts: {
        title: 'Trend Analytics',
        subtitle: 'Track continuous revenue, site user balance, cost, net profit and upstream balance over time.',
        trendTitle: '{metric} Trend'
      },
      period: {
        label: 'Period',
        week: 'Week',
        month: 'Month'
      },
      delta: {
        vsPrev: 'vs prev day',
        percentagePoints: '{value} pts'
      },
      common: {
        unavailable: 'N/A'
      },
      performance: {
        title: 'Business Performance',
        subtitle: 'Revenue, cost, and net profit over the same period',
        periodRevenue: 'Period Revenue',
        periodCost: 'Period Cost',
        periodProfit: 'Period Net Profit',
        chartAria: 'Combined revenue, cost, and net profit trend chart'
      },
      capital: {
        title: 'Capital Safety',
        subtitle: 'Balance coverage and cost runway',
        siteBalance: 'Site User Balance',
        upstreamBalance: 'Upstream Balance',
        coverage: 'Balance Coverage',
        coverageHint: 'Upstream balance relative to total site user balance',
        runway: 'Estimated Runway',
        runwayHint: 'Based on average daily cost for this period',
        runwayValue: '{value} days'
      },
      groups: {
        title: 'Group Contribution',
        subtitle: 'Top groups by revenue today',
        total: '{count} groups',
        amount: "Today's Revenue",
        topThreeShare: 'Top 3 revenue share',
        empty: 'No group revenue data available.',
        loadError: 'Group contribution data is temporarily unavailable.',
        chartAria: 'Today group revenue contribution ranking chart'
      },
      attention: {
        title: 'Needs Attention',
        subtitle: 'Upstream balance and connection issues',
        upstreamTitle: 'Upstream Balance Issues',
        upstreamDescription: 'Some upstream sites have unknown balances or connection errors',
        allClearTitle: 'No Clear Issues',
        allClearDescription: 'No upstream balance or connection issue currently requires attention.',
        unavailableTitle: 'Operational Status Unavailable',
        unavailableDescription: 'Core business metrics are unaffected. Retry upstream status shortly.',
        refresh: 'Refresh operational status',
        partialLoadError: 'Some operational data failed to load'
      },
      loading: 'Loading metrics...',
      loadError: 'Failed to load dashboard metrics.',
      retry: 'Retry',
      dataStatus: {
        refreshing: 'Refreshing latest data',
        updatedAt: 'Updated at {time}',
        waiting: 'Waiting for first update',
        failed: 'Refresh failed; showing previous data',
        refresh: 'Refresh dashboard data'
      },
      loadingModal: {
        title: 'Loading Dashboard Data',
        progress: '{progress}% complete',
        steps: {
          auth: 'Verifying admin credentials...',
          data: 'Loading metrics and trends...',
          done: 'Preparing data and rendering page...'
        }
      },
      groupUsage: {
        title: 'Today\'s Revenue by Group',
        subtitle: '{count} groups, {total} total',
        close: 'Close',
        empty: 'No group usage data available.',
        loadError: 'Failed to load group usage data.',
        retry: 'Retry',
        columns: {
          groupName: 'Group Name',
          amount: 'Today\'s Amount'
        },
        sort: {
          desc: 'Amount: High to Low',
          asc: 'Amount: Low to High'
        }
      },
      cost: {
        syncedAt: 'From {count} upstream sites · earliest sync {time}',
        staleSites: '{count} upstream sites have not synced successfully today and are not counted (retrying in the background)'
      },
      pricing: {
        title: 'My site pricing',
        subtitle: 'Converts revenue and user balances to CNY',
        label: 'Price per $1 (1 unit) of site balance',
        unit: 'CNY / $1',
        helpRevenue: 'Revenue and total user balance = site amount × price. Defaults to 1, i.e. $1 counts as ¥1.',
        helpExample: 'If you sell $1 of credit for ¥0.8, enter 0.8; for ¥7.2, enter 7.2.',
        helpCost: 'Upstream cost uses each upstream site\'s recharge multiplier (what you actually paid per $1 of upstream credit), so both sides are in CNY and net profit and margin are meaningful.',
        helpHistory: 'Changes apply from today; days that were already finalized are not recalculated.',
        cardNote: 'At ¥{price}/$1',
        basis: 'Revenue and site balance use your price of ¥{price}/$1; cost uses each upstream site\'s recharge multiplier.',
        edit: 'Edit price',
        save: 'Save',
        cancel: 'Cancel',
        close: 'Close',
        errors: {
          invalid: 'Enter a price greater than 0 and at most 10000.',
          request: 'Could not save the price. Please try again.'
        }
      },
      upstreamKeyUsage: {
        title: 'Today\'s Cost Breakdown',
        subtitle: '{count} keys, {total} total',
        liveNote: 'Queried key by key at {time}. The card uses each upstream site\'s account total from its latest sync, so the totals can differ slightly: the query times differ and keys deleted today are not listed here.',
        close: 'Close',
        empty: 'No keys with usage today.',
        loadError: 'Failed to load today\'s cost breakdown. Check upstream site connections and retry.',
        partialWarning: '{failed} of {total} upstream sites are unavailable. Totals include successful sites only.',
        retry: 'Retry',
        errors: {
          unavailable: 'Key usage is unavailable for every upstream site. Check site connections or credentials.'
        },
        columns: {
          siteName: 'Upstream Site',
          keyName: 'Key Name',
          groupName: 'Group',
          amount: 'Today\'s Amount'
        },
        sort: {
          desc: 'Amount: High to Low',
          asc: 'Amount: Low to High'
        }
      },
      upstreamBalanceBreakdown: {
        title: 'Upstream Balance Breakdown',
        subtitle: '{count} sites, {total} total',
        close: 'Close',
        empty: 'No upstream site balance data available.',
        loadError: 'Failed to load upstream balance breakdown.',
        retry: 'Retry',
        unknownBalance: 'Unknown',
        neverSynced: 'Never synced',
        columns: {
          siteName: 'Upstream Site',
          status: 'Status',
          lastSyncedAt: 'Last Synced',
          balance: 'Balance'
        },
        sort: {
          desc: 'Balance: High to Low',
          asc: 'Balance: Low to High'
        }
      },
      balanceFilter: {
        title: 'Balance Filter',
        subtitle: 'Configure filtering rules for site user balance calculation.',
        close: 'Close',
        excludeAdmin: 'Exclude admin accounts',
        excludeAdminHelp: 'Do not include admin role users in the balance total.',
        excludeBalances: 'Exclude specific balance values',
        excludeBalancesHelp: 'Users whose balance equals any of these values will be excluded.',
        addPlaceholder: 'Enter a balance value to exclude',
        add: 'Add',
        cancel: 'Cancel',
        save: 'Save',
        saving: 'Saving...',
        loadError: 'Failed to load filter config.',
        saveError: 'Failed to save filter config.'
      },
      adminAuth: {
        loggedInAs: 'Current admin: {identity}',
        logout: 'Sign out current admin',
        notLoggedIn: 'No admin account signed in',
        login: 'Sign in admin account',
        expiresAt: 'Expires',
        timeUnknown: 'Unknown',
        updateCredentials: 'Update Admin Credentials',
        updatingCredentials: 'Updating...',
        logoutConfirm: {
          title: 'Sign out current admin?',
          description: 'After signing out you must sign in and pass admin verification again to view dashboard data.',
          confirm: 'Sign out',
          cancel: 'Cancel'
        },
        dataLocked: {
          title: 'Sign in an admin account first',
          description: 'Dashboard stats are only visible after signing in and verifying a site account with admin permission.'
        },
        modal: {
          title: 'Sign in admin account',
          subtitle: 'The dashboard requires a site account with admin permission.',
          close: 'Close',
          platformLabel: 'Platform',
          platform: {
            sub2api: 'Sub2API',
            newapi: 'New-API'
          },
          comingSoon: 'Coming soon',
          newApiPasswordOnly: 'New-API supports username & password login only.',
          siteUrlLabel: 'Site address (domain or IP)',
          siteUrlPlaceholder: 'e.g. https://sub.example.com or http://1.2.3.4:5555',
          methodLabel: 'Login method',
          method: {
            password: 'Email & Password',
            token: 'RT + AT',
            admin_key: 'Admin Key'
          },
          fields: {
            emailPlaceholder: 'Admin email',
            usernamePlaceholder: 'Admin username',
            passwordPlaceholder: 'Admin password',
            accessTokenPlaceholder: 'Access Token (optional, can be empty)',
            refreshTokenPlaceholder: 'Refresh Token (enter this or an Access Token)',
            tokenTypePlaceholder: 'Token Type (optional, default Bearer)',
            tokenHelp: 'Enter an Access Token or Refresh Token. When a Refresh Token is present, near-expiry credentials refresh automatically.',
            sub2apiAdminKeyPlaceholder: 'Sub2API Admin API Key',
            sub2apiAdminKeyHelp: 'This key is sent only to Sub2API admin routes through x-api-key.',
            newApiAdminKeyPlaceholder: 'New-API access token (nap_…) or system access token',
            newApiAdminKeyHelp: 'Recommended: a New-API personal access token (PAT) created by the Root account with the profile:read, user:read, log:read, group:read, option:read, option:write, channel:read and channel:write scopes (add channel:sensitive_write if TransitHub should create or delete channels) and a long expiry. Legacy system access tokens also need the user ID.',
            userIdPlaceholder: 'User ID (optional, required for legacy system access tokens)',
            newApiPasswordHelp: 'Password sign-in on current New-API lasts at most 30 days, after which you need to sign in again. Accounts with two-step verification cannot sign in automatically; use an access token instead.'
          },
          submit: 'Sign in & verify',
          submitting: 'Verifying...'
        },
        errors: {
          request: 'Admin login request failed. Try again later.',
          missingCredentials: 'Fill in the site address and required fields for the selected method.',
          invalidUrl: 'Invalid site address. Enter a valid domain or IP and retry.',
          adminOnly: 'This account is not admin or verification failed. Check the credentials and retry.',
          network: 'Network or CORS request failed. Check the site URL.',
          platformUnsupported: 'Unsupported platform. Please choose Sub2API or New-API.',
          unknown: 'An unknown error occurred during admin login.',
          reloginRequired: 'Admin verification failed. Please sign in again.',
          verificationRequired: 'This account has two-step verification enabled and cannot sign in automatically. Use an access token instead.',
          tokenRejected: 'Access token rejected. Legacy New-API system access tokens also need the user ID; on current New-API, make sure the token is valid, not expired, and has the required scopes.'
        }
      }
    },
      upstream: {
        searchPlaceholder: 'Search site name...',
        addSite: 'Add Site',
        summary: '{connected} / {total} upstream sites connected',
        refresh: {
          action: 'Refresh Data',
          refreshing: 'Refreshing...',
          countdown: 'Refresh in {seconds}s',
          disabled: 'Auto refresh disabled'
        },
      modal: {
        title: 'Add Upstream Site',
        editTitle: 'Edit Upstream Site',
        cancel: 'Cancel',
        submit: 'Add Site',
        updateSubmit: 'Save Changes',
        submitting: 'Connecting...',
        form: {
          siteName: 'Site Name',
          siteNamePlaceholder: 'Enter site name',
          siteUrl: 'Site URL',
          siteUrlPlaceholder: 'Enter full site URL, e.g. https://api.example.com',
          platform: 'Platform',
          platforms: {
            auto: 'Auto Detect',
            sub2api: 'Sub2API',
            newapi: 'New-API'
          },
          authMode: 'Authentication Method',
          authModes: {
            password: 'Account Password',
            passwordHelp: 'Sign in with the site account and password, then save the session automatically.',
            token: 'Access Token / Refresh Token',
            tokenHelp: 'Use this for Sub2API sites where Cloudflare or two-factor login blocks direct password login.',
            userKey: 'Access Token',
            userKeyHelp: 'Use a New-API personal access token (PAT) or a legacy system access token. Suits accounts with two-step verification or that need to stay signed in long-term.',
            passwordHelpNewApi: 'Sign in with the site account and password. Password sign-in on current New-API lasts at most 30 days; accounts with two-step verification should use an access token.'
          },
          account: 'Account',
          accountPlaceholder: 'Enter account',
          password: 'Password',
          passwordPlaceholder: 'Enter password',
          passwordEditPlaceholder: 'Leave blank to keep current password',
          passwordEditHelp: 'Leaving this blank keeps the current login session. Enter a new password only when you want to re-login and update credentials.',
          accessToken: 'Access Token',
          accessTokenPlaceholder: 'Paste access_token, or leave blank and provide only refresh_token',
          refreshToken: 'Refresh Token',
          refreshTokenPlaceholder: 'Paste refresh_token; the server refreshes it once for the latest expiry',
          tokenType: 'Token Type',
          tokenTypePlaceholder: 'Defaults to Bearer',
          tokenHelp: 'When refresh_token is provided, the server refreshes it first to obtain the latest access_token and expiration time.',
          userId: 'User ID (optional)',
          userIdPlaceholder: 'Required for legacy system access tokens; leave empty for current access tokens',
          userKey: 'Access Token',
          userKeyPlaceholder: 'Paste a New-API access token (nap_…) or system access token',
          userKeyHelp: 'Recommended: a personal access token (PAT) with the profile:read, usage:read, api_key:read and api_key:reveal scopes (add api_key:write if TransitHub should create upstream keys) and a long expiry.',
          rechargeRate: 'Recharge Multiplier',
          rechargeRatePlaceholder: 'What you paid per $1 of upstream credit (CNY), e.g. 1 or 0.5',
          rechargeRateHelp: 'Required. Enter how much CNY you actually paid for $1 of upstream credit: 1 if ¥1 buys $1, 0.5 at half price, or the exchange rate (about 7.2) only if you pay in USD. Today\'s cost = upstream spend (USD) × this multiplier.',
          remark: 'Remark',
          remarkPlaceholder: 'Enter remark (optional)'
        }
      },
      currency: {
        usdValue: '{amount} USD',
        cnyValue: '{amount} CNY'
      },
      fields: {
        siteName: 'Site Name',
        siteUrl: 'Site URL',
        platform: 'Platform',
        balance: 'Balance',
        todayConsume: 'Today Consume',
        historyRecharge: 'History Recharge',
        groupName: 'Group Name',
        groupMultiplier: 'Group Multiplier',
        availableGroups: 'Available Groups',
        viewAvailableGroups: 'View Available Groups',
        closeGroupsModal: 'Close',
        dedicatedMultiplierBadge: 'Dedicated Rate',
        dedicatedMultiplierTooltip: 'This user has a sub2api dedicated rate override. Business calculations use the rate on the right.',
        unknownPlatform: 'Unknown Platform',
        lastUpdated: 'Last Updated',
        notSynced: 'Not synced yet'
      },
      status: {
        connecting: 'Connecting',
        syncing: 'Syncing',
        connected: 'Connected',
        error: 'Error'
      },
      empty: {
        title: 'No upstream sites found',
        description: 'Adjust your search or add an upstream site.'
      },
      delete: {
        action: 'Delete site',
        title: 'Delete this upstream site?',
        description: 'You are deleting "{name}". To restore it, you will need to add and log in again.',
        cancel: 'Cancel',
        confirm: 'Delete'
      },
      action: {
        sync: 'Sync',
        syncing: 'Syncing',
        edit: 'Edit Site',
        settings: 'Site Settings',
        actions: 'Actions'
      },
      siteSettings: {
        title: 'Site Alert Settings',
        balanceThreshold: 'Custom Balance Threshold',
        balanceThresholdHelp: 'Enable to use a site-specific threshold. Disable to use the global default.',
        balanceThresholdPlaceholder: 'Enter threshold amount',
        save: 'Save',
        saveSuccess: 'Saved',
        saving: 'Saving...',
        cancel: 'Cancel'
      },
      viewMode: {
        list: 'List Mode',
        card: 'Card Mode'
      },
      syncStream: {
        syncing: 'Syncing...',
        done: 'Sync complete',
        error: 'Sync failed',
      },
      errors: {
        invalidUrl: 'The site URL is invalid. Check it and try again.',
        network: 'Could not reach the upstream site (network, proxy, or TLS error). Check the site URL, or adjust the proxy under System Settings → Network Proxy.',
        auth: 'Login failed. Check the account or password.',
        request: 'The upstream API request failed. Try again later.',
        invalidResponse: 'The upstream response could not be parsed.',
        tokenMissing: 'Login succeeded but no access token was returned.',
        detect: 'The platform could not be auto-detected. Choose a platform and try again.',
        verificationRequired: 'This account has two-step verification enabled and cannot sign in automatically. Add the site with an access token instead.',
        newApiTokenRejected: 'Access token rejected. Legacy New-API system access tokens also need the user ID; on current New-API, make sure the token is valid, not expired, and has the required scopes.',
        unknown: 'An unknown error occurred while connecting the upstream site.'
      }
    },
    groupRates: {
      badge: 'Rate Sync Ledger',
      title: 'Group Rates',
      subtitle: 'Review current upstream group multipliers and recent changes, then inspect multiplier history.',
      common: {
        placeholder: '—',
        allTypes: 'All Types',
        allPlatforms: 'All Platforms',
        unknown: 'Unknown'
      },
      platforms: {
        newapi: 'New-API',
        sub2api: 'Sub2API'
      },
      summary: {
        totalLabel: 'Total Group Rates',
        updatedLabel: 'Synced Records'
      },
      table: {
        title: 'Rate List',
        description: 'Rows preserve the ordering returned by the backend.'
      },
      fields: {
        siteName: 'Site Name',
        groupName: 'Group Name',
        type: 'Group Type',
        platform: 'Site Platform',
        currentMultiplier: 'Current Multiplier',
        effectiveMultiplier: 'Converted Cost Multiplier',
        multiplierFormula: 'Upstream {upstream} × recharge factor {recharge}',
        delta: 'Rise/Fall',
        updatedAt: 'Updated Time'
      },
      actions: {
        refresh: 'Refresh Data',
        viewHistory: 'View History',
        viewHistoryForRate: 'View rise/fall history for {site} · {group}; current change {delta}',
        closeHistory: 'Close History'
      },
      filters: {
        searchLabel: 'Search',
        searchPlaceholder: 'Search site or group...',
        typeLabel: 'Group Type',
        platformLabel: 'Site Platform'
      },
      sort: {
        label: 'Sort',
        multiplierAsc: 'Multiplier Low to High',
        multiplierDesc: 'Multiplier High to Low',
        siteNameAsc: 'Site Name A-Z',
        groupNameAsc: 'Group Name A-Z'
      },
      tabs: {
        all: 'All',
        deleted: 'Deleted'
      },
      pagination: {
        previous: 'Previous',
        next: 'Next',
        currentPage: 'Page {page} / {totalPages}',
        total: '{total} total',
        pageSize: '{pageSize} per page'
      },
      status: {
        loading: 'Loading group rates...',
        deleted: 'Deleted'
      },
      empty: {
        title: 'No group rates yet',
        description: 'Synced upstream group multiplier data will appear here.',
        filteredTitle: 'No records match these filters',
        filteredDescription: 'Change the status, search, type, or platform filter.'
      },
      history: {
        title: 'Multiplier History',
        titleWithGroup: '{site} · {group} Multiplier History',
        subtitle: 'Platform: {platform}',
        loading: 'Loading history records...',
        emptyTitle: 'No history records',
        emptyDescription: 'This site group has not returned multiplier history yet.',
        multiplier: 'Multiplier',
        delta: 'Rise/Fall',
        createdAt: 'Recorded Time'
      },
      format: {
        multiplier: '{value}x',
        deltaMultiplier: '{value}x'
      },
      errors: {
        network: 'Network or CORS request failed. Check the API URL and cross-origin settings.',
        request: 'The group rates API request failed. Try again later.',
        unknown: 'An unknown error occurred while loading group rates.'
      }
    },
    massEmail: {
      common: {
        placeholder: '-'
      },
      filters: {
        search: 'Search users',
        searchPlaceholder: 'Enter an email keyword',
        noSearch: 'No search term',
        status: 'User Status',
        role: 'Role',
        allStatuses: 'All statuses',
        allRoles: 'All roles'
      },
      template: {
        label: 'Email Template',
        placeholder: 'Select a template',
        noSubject: 'No subject selected'
      },
      selection: {
        title: 'Recipient Selection',
        count: '{count} selected across pages',
        selectPage: 'Select all users on this page',
        selectUser: 'Select {email}'
      },
      users: {
        title: 'Recipients'
      },
      fields: {
        email: 'Email',
        role: 'Role',
        status: 'Status',
        createdAt: 'Created',
        actions: 'Actions'
      },
      roles: {
        user: 'User',
        admin: 'Admin'
      },
      userStatus: {
        active: 'Active',
        disabled: 'Disabled',
        inactive: 'Inactive',
        banned: 'Banned'
      },
      actions: {
        search: 'Search',
        clearSearch: 'Clear search',
        refresh: 'Refresh',
        clearSelection: 'Clear',
        sendSelected: 'Send selected',
        sendPage: 'Send page',
        sendFilter: 'Send filter',
        sendRow: 'Send',
        cancelBatch: 'Cancel',
        closeConfirm: 'Close confirmation',
        openBatches: 'Batches',
        openBatchDetail: 'Details',
        previewTemplate: 'Preview'
      },
      status: {
        loadingUsers: 'Loading recipients...',
        loadingItems: 'Loading outcomes...'
      },
      batchStatus: {
        queued: 'Queued',
        running: 'Running',
        completed: 'Completed',
        completed_with_errors: 'Completed with errors',
        failed: 'Failed',
        cancelled: 'Cancelled',
        cancelling: 'Cancelling'
      },
      itemStatus: {
        pending: 'Pending',
        sending: 'Sending',
        sent: 'Sent',
        failed: 'Failed',
        uncertain: 'Uncertain',
        cancelled: 'Cancelled'
      },
      empty: {
        usersTitle: 'No recipients match this filter',
        usersDescription: 'Change the search, status, or role filter and try again.',
        batches: 'No mass-email batches yet.',
        detail: 'Select a batch to view recipient outcomes.'
      },
      pagination: {
        total: '{total} total',
        pageSize: '{pageSize} per page',
        currentPage: 'Page {page} of {totalPages}',
        previous: 'Previous',
        next: 'Next'
      },
      batches: {
        title: 'Batches',
        active: '{count} active',
        progress: '{done}/{total} done, {percent}%',
        close: 'Close batches'
      },
      detail: {
        title: 'Batch Detail',
        recipients: '{total} recipient outcomes',
        close: 'Close batch detail'
      },
      preview: {
        title: 'Template Preview',
        close: 'Close preview',
        iframeTitle: 'Email template preview'
      },
      summary: {
        sent: 'Sent',
        failed: 'Failed',
        uncertain: 'Uncertain',
        cancelled: 'Cancelled'
      },
      confirm: {
        selectedTitle: 'Send email to selected recipients?',
        selectedDescription: 'This will create a mass-email batch for {count} selected recipients.',
        allTitle: 'Send email to all recipients in the current filter?',
        allDescription: 'This will create a mass-email batch for {count} recipients matching the current filter.',
        recipients: 'Recipients: {count}',
        template: 'Template: {name}',
        filters: 'Filters: {status}, {role}, search: {search}',
        cancel: 'Cancel',
        submit: 'Create batch'
      },
      success: {
        created: 'Mass-email batch created.',
        cancelled: 'Batch cancellation requested.'
      },
      errors: {
        network: 'Network or CORS request failed. Check the API URL and cross-origin settings.',
        request: 'Mass-email request failed. Try again later.',
        templates: 'Failed to load email templates.',
        unknown: 'An unknown error occurred while loading mass-email data.',
        invalidRequest: 'The mass-email request is invalid.',
        invalidSelection: 'Select at least one valid recipient.',
        templateNotFound: 'The selected email template no longer exists.',
        smtpNotReady: 'SMTP settings are not ready.',
        upstreamAuth: 'Upstream admin authentication failed.',
        upstreamRequest: 'Upstream request failed.',
        notFound: 'Mass-email batch not found.',
        invalidState: 'This batch cannot perform the requested action in its current state.',
        persistence: 'Mass-email data could not be saved.',
        sendFailed: 'Email sending failed.',
        activeBatchExists: 'Another mass-email batch is already active in this workspace. Cancel or wait for it to finish before creating a new one.',
        recipientLimitReached: 'This selection exceeds the 10,000-recipient limit. Narrow the filters and try again.',
        itemGeneric: 'Recipient delivery failed. Check the batch later for updated details.',
        segmentUnavailable: 'Sending by activity segment is unavailable.',
        segmentNotSynced: 'User data has not been synced yet. It syncs in the background after you sign in to your site, or you can sync it now on the User Analytics page.',
        invalidPublicUrl: 'Invalid unsubscribe link address. Enter a full http(s) URL.',
        invalidSuppression: 'Enter valid email addresses (up to 1,000 at a time).',
        suppressed: 'The recipient unsubscribed; nothing was sent.'
      }
    },
    userActivity: {
      subtitle: 'Syncs users and daily usage from this workspace\'s site (sub2api / new-api) into TransitHub to analyze activity, inactivity, day/week/month-over-month and year-over-year changes, and email filtered users.',
      common: {
        placeholder: '—'
      },
      platforms: {
        sub2api: 'sub2api',
        newapi: 'new-api'
      },
      sync: {
        title: 'Data sync',
        never: 'Not synced yet',
        lastSynced: 'Synced {time}',
        coverage: 'Usage covers {range}',
        users: '{count} users',
        running: 'Syncing users and usage…',
        runningBackfill: 'Backfilling older usage history…',
        failed: 'Last sync failed: {reason}'
      },
      time: {
        justNow: 'just now',
        minutesAgo: '{count} min ago',
        hoursAgo: '{count} h ago',
        daysAgo: '{count} d ago'
      },
      actions: {
        refresh: 'Refresh',
        syncNow: 'Sync now',
        startSync: 'Start sync',
        goDashboard: 'Sign in on the dashboard',
        sendEmail: 'Email filtered users',
        suppressions: 'Unsubscribed ({count})',
        search: 'Search',
        clearSearch: 'Clear search',
        close: 'Close',
        cancel: 'Cancel'
      },
      status: {
        loading: 'Loading…'
      },
      empty: {
        title: 'No user data yet',
        description: 'After you sign in to your site, TransitHub starts syncing in the background within an hour: it reads the site\'s user list and daily usage with this workspace\'s admin session (the last 62 days first, then hourly incremental syncs that gradually backfill older history). The data stays in TransitHub for analysis. You can also start right away with the button below.',
        runningTitle: 'Running the first sync',
        runningDescription: 'Reading site users and the last 62 days of usage. This usually takes 1–3 minutes; the page refreshes automatically when it finishes.'
      },
      money: {
        title: 'Money',
        methodRecords: 'From the site\'s top-up records: online payments, redeem codes, admin adjustments and affiliate rebates',
        methodSnapshot: 'From changes in each user\'s "balance + total spent" between syncs (since {time}); includes online top-ups, redeem codes, admin adjustments and check-in / invite rewards, without a per-channel split',
        todayRecharge: 'Credited today',
        deduction: 'deducted {value}',
        yesterdayFull: 'yesterday {value}',
        events: '{count} record | {count} records',
        noRechargeToday: 'Nothing credited yet today',
        noRechargeData: 'No credit data yet; it appears after a sync',
        notUpToDate: 'The latest sync could not read top-up records, so these numbers may be out of date',
        todayCost: 'Spent today',
        totalBalance: 'Total user balance',
        balanceChange: '{value} vs. yesterday\'s close',
        usersWithBalance: '{count} users with balance',
        lifetimeRecharged: 'Lifetime top-ups {value} (site field, excludes admin adjustments)',
        rechargeUsers: 'Users credited today',
        lowBalance: 'Active with balance under $1',
        channels: {
          payment: 'Online payments',
          redeem: 'Redeem codes',
          admin_add: 'Admin top-ups',
          admin_deduct: 'Admin deductions',
          affiliate: 'Affiliate rebates',
          credit: 'Balance increases',
          debit: 'Balance decreases'
        }
      },
      warnings: {
        rechargeFetch: 'The sync could not read the site\'s top-up records (the admin key may lack permission to view redeem codes, or the network failed). Activity and usage data are unaffected; the next sync retries.',
        rechargeUnsupported: 'This site version cannot list redeem records by usage time, so credits cannot be counted yet. Upgrade sub2api to enable it.',
        rechargeTruncated: 'Too many top-up records to read, so no credit data was written this time; the next sync tries again.'
      },
      kpi: {
        totalUsers: 'Total users',
        totalUsersHint: '{withEmail} with email · {disabled} disabled',
        activeToday: 'Active today',
        todayHint: '{requests} requests · {cost} spent',
        active7d: 'Active in the last 7 days',
        vsPrevious7d: 'vs. the previous 7 days',
        active30d: 'Active in the last 30 days',
        vsPrevious30d: 'vs. the previous 30 days',
        noUsage: 'Never used',
        noUsageHint: 'Signed up without any usage; click to view'
      },
      inactive: {
        title: 'Inactivity',
        hint: 'Excludes disabled users ({count} users); click a bucket to view the list',
        bucket: 'Inactive > {days} days'
      },
      periods: {
        day: {
          title: 'Day over day',
          description: 'Yesterday vs. the day before'
        },
        week: {
          title: 'Week over week',
          description: 'Last 7 days vs. the previous 7'
        },
        month: {
          title: 'Month over month',
          description: 'Last 30 days vs. the previous 30'
        },
        metric: 'Metric',
        current: 'Current',
        sequential: 'vs. prev.',
        yearOverYear: 'YoY',
        previousMissing: 'Local data does not cover the previous period yet; it appears after backfilling',
        lastYearMissing: 'No local data for the same period last year yet (sub2api keeps 90 days of usage, so TransitHub needs a full year of syncing)'
      },
      metrics: {
        activeUsers: 'Active users',
        requests: 'Requests',
        tokens: 'Tokens',
        cost: 'Spend (USD)',
        newUsers: 'Sign-ups',
        recharge: 'Credits (USD)'
      },
      retention: {
        retained: 'Retained {count}',
        churned: 'Churned {count}',
        gained: 'Newly active {count}'
      },
      delta: {
        fromZero: 'New'
      },
      trend: {
        range: 'Trend range',
        lastDays: 'Last {days} days',
        completeDays: 'Through yesterday (complete days)',
        showTable: 'Table',
        showChart: 'Chart',
        day: 'Date',
        activeUsersTitle: 'Daily active users',
        costTitle: 'Daily spend (USD)',
        latest: '{day}: {value}',
        usersValue: '{count} users',
        activeUsersAria: 'Daily active users over the last {days} days',
        costAria: 'Daily spend over the last {days} days',
        rechargeTitle: 'Daily credits (USD)',
        rechargeAria: 'Daily credits over the last {days} days',
        noData: 'No data (not covered yet)'
      },
      users: {
        title: 'Users',
        subtitle: '"Last N days" always ends yesterday. Click a user for daily usage and comparisons.',
        matched: '{count} matching users',
        empty: 'No users match these filters'
      },
      filters: {
        segment: 'Segment',
        inactiveDays: 'Inactive days',
        status: 'Status',
        role: 'Role',
        search: 'Search',
        searchPlaceholder: 'Email, username or user ID',
        allStatuses: 'All statuses',
        allRoles: 'All roles',
        searchSummary: 'search "{search}"'
      },
      segmentGroups: {
        basic: 'All',
        active: 'Active',
        inactive: 'Inactive',
        change: 'Usage change',
        money: 'Money'
      },
      segments: {
        all: 'All users',
        active_1d: 'Used in the last 24 hours',
        active_7d: 'Used in the last 7 days',
        active_30d: 'Used in the last 30 days',
        inactive_2d: 'Inactive > 2 days',
        inactive_7d: 'Inactive > 7 days',
        inactive_14d: 'Inactive > 14 days',
        inactive_30d: 'Inactive > 30 days',
        inactive_60d: 'Inactive > 60 days',
        inactive_90d: 'Inactive > 90 days',
        inactive: 'Custom inactive days',
        inactiveCustom: 'Inactive > {days} days',
        no_usage: 'Never used',
        churned_7d: 'Churned this week (active the week before)',
        declining: 'Declining (weekly spend halved)',
        growing: 'Growing (weekly spend ≥ 1.5×)',
        recharged_today: 'Credited today',
        low_balance: 'Active with balance under $1'
      },
      userStatus: {
        active: 'Active',
        disabled: 'Disabled'
      },
      roles: {
        user: 'User',
        admin: 'Admin',
        root: 'Root'
      },
      fields: {
        user: 'User',
        status: 'Status',
        lastUsed: 'Last used',
        costToday: 'Spend today',
        cost7d: 'Spend, 7 days',
        change7d: 'WoW',
        cost30d: 'Spend, 30 days',
        activeDays30d: 'Active days (30d)',
        balance: 'Balance',
        totalRecharged: 'Total recharged',
        totalUsed: 'Total spent',
        createdAt: 'Signed up',
        today: 'Spend today',
        requestsCount: '{count} request | {count} requests',
        daysValue: '{count} day | {count} days',
        previousValue: 'prev. 7 days {value}',
        rechargeToday: 'Credited today',
        recharge30d: 'Credits, 30 days'
      },
      lastUsed: {
        today: 'Today',
        daysAgo: '{count} day ago | {count} days ago',
        longAgo: 'Long ago',
        never: 'Never'
      },
      detail: {
        identity: 'User ID {id} · Username {username}',
        trendTitle: 'Daily usage, last 90 days',
        requestsChartAria: 'Daily requests for this user over the last 90 days',
        costChartAria: 'Daily spend for this user over the last 90 days',
        rechargeChartAria: 'Daily credits for this user over the last 90 days'
      },
      email: {
        title: 'Email users matching these filters',
        loading: 'Counting recipients…',
        matched: 'Matching',
        withEmail: 'With email',
        suppressed: 'Unsubscribed',
        sendable: 'Will receive',
        previewHint: 'Recipients come from the locally synced user snapshot. Invalid or duplicate addresses are skipped when the batch is created; track progress on the Mass Email page.',
        template: 'Email template',
        templatePlaceholder: 'Select a template',
        noTemplates: 'No email templates yet. Create one under Settings - Email templates first.',
        subject: 'Subject:',
        unsubscribe: 'Include an unsubscribe link',
        unsubscribeHint: 'Each email gets the recipient\'s own unsubscribe link at the end. Put {placeholder} in the template to choose where it goes.',
        unsubscribePlaceholderFound: '{placeholder} in the template is replaced with each recipient\'s own unsubscribe link.',
        publicBaseUrl: 'Public TransitHub API address',
        publicBaseUrlHint: 'Unsubscribe links open /email-unsubscribe under this address, so it must be reachable from the internet.',
        privateUrlWarning: 'This looks like a local or private address; recipients outside your network may not be able to open the unsubscribe link.',
        submit: 'Create batch ({count} recipients)',
        created: 'Batch created: {count} recipients, {skipped} skipped (invalid, duplicate or unsubscribed).',
        viewProgress: 'View progress'
      },
      suppressions: {
        title: 'Unsubscribe list',
        description: 'Addresses on this list never receive mass email, whichever way recipients are selected. Recipients are added automatically when they click an unsubscribe link.',
        addLabel: 'Add addresses manually',
        addPlaceholder: 'One per line, or separated by commas',
        add: 'Add to list',
        searchPlaceholder: 'Search email',
        email: 'Email',
        reason: 'Source',
        createdAt: 'Added',
        empty: 'The unsubscribe list is empty',
        remove: 'Remove from list',
        removeAria: 'Remove {email} from the unsubscribe list',
        removeConfirm: 'Remove {email} from the unsubscribe list? It will receive mass email again.',
        added: 'Added {added} addresses; ignored {invalid} invalid ones.',
        removed: 'Removed {email} from the unsubscribe list.',
        reasons: {
          unsubscribe: 'Recipient unsubscribed',
          manual: 'Added manually'
        }
      },
      pagination: {
        total: '{total} total',
        previous: 'Previous',
        next: 'Next'
      },
      errors: {
        network: 'Network request failed. Check that the TransitHub service is reachable.',
        request: 'User analytics request failed. Please try again later.',
        authRequired: 'An admin session for this workspace\'s site is required: sign in to the sub2api / new-api admin account on the dashboard first.',
        platformUnsupported: 'User analytics only supports sub2api and new-api sites.',
        syncInProgress: 'A sync is already running. Please try again shortly.',
        usersFetch: 'Could not read the site\'s user list. Check that the admin session is valid.',
        usageFetch: 'Could not read the site\'s usage data. sub2api needs an admin session; new-api needs admin rights (access tokens need the log:read scope) and data dashboard statistics enabled.',
        interrupted: 'The last sync was interrupted by a service restart or timeout. It resumes in the background automatically; no action needed.',
        invalidFilter: 'Invalid filter.',
        notFound: 'The user does not exist or was deleted from the site.',
        tooManyUsers: 'Sites with more than 50,000 users are not supported yet.',
        tooManyRecipients: 'More than 10,000 recipients match. Narrow the filters.',
        notSynced: 'No user data has been synced yet. Run a sync first.'
      }
    },
    sales: {
      title: 'Sales',
      subtitle: 'Credit sources, paying users, conversion and repeat purchases, revenue concentration, and top payers. Read-only, based on the data synced by User Analytics.',
      common: {
        placeholder: '—'
      },
      source: {
        lastSynced: 'Data synced by User Analytics · last sync {time}',
        neverSynced: 'User data has not been synced yet',
        basis: 'Amounts converted at ¥{price} per $1 (same as dashboard revenue)'
      },
      range: {
        label: 'Period',
        lastDays: 'Last {days} days',
        from: 'Start date',
        to: 'End date',
        apply: 'Apply',
        current: 'Period: {range} (complete days through yesterday, or a custom range)',
        previous: 'Compared with: {range}'
      },
      actions: {
        refresh: 'Refresh',
        changePrice: 'Change price',
        goUserActivity: 'Sync data in User Analytics',
        showTable: 'Table',
        showChart: 'Chart',
        viewDetail: 'View details of {name}'
      },
      empty: {
        title: 'No data to analyze yet',
        description: 'Sales analysis uses the users, credits, and usage that User Analytics syncs locally. It syncs in the background after you sign in to your site, or you can sync now on the User Analytics page.'
      },
      coverage: {
        partial: 'Credit data starts on {from}, so the selected period is incomplete. Totals only include covered days.',
        snapshot: 'For new-api sites, credits are measured from balance changes between syncs (since {from}). Their source and first-time payers cannot be identified.',
        usagePartial: 'Usage data only covers {range}, so the selected period is incomplete',
        previousMissing: 'The previous period has no complete credit data, so no change is shown'
      },
      kpi: {
        credits: 'Credits',
        creditsHint: '{count} credits · plus {deductions} deducted',
        payers: 'Paying users',
        payersHint: '{newPayers} first-time · {repeat} repeat',
        arppu: 'Credits per paying user',
        arppuHint: 'Credits ÷ paying users',
        consumption: 'User spend',
        consumptionHint: 'Credits ÷ spend = {ratio}',
        consumptionNone: 'No spend in this period',
        balanceChange: 'Balance growth',
        balanceChangeHint: 'User balance at period end: {value}',
        balanceUnknown: 'Missing a balance snapshot at the start or end'
      },
      trend: {
        title: 'Daily credits and spend',
        subtitle: 'Bars: credits stacked by source; line: user spend',
        total: 'Total credits',
        deductions: 'Deducted',
        consumption: 'User spend',
        day: 'Date',
        noData: 'No data',
        aria: 'Daily credits by source and user spend for {range}'
      },
      channels: {
        title: 'Credit sources',
        subtitle: 'Deductions (admin deductions, balance decreases) are listed separately and not counted in credits',
        channel: 'Source',
        amount: 'Amount',
        share: 'Share',
        events: 'Count',
        users: 'Users',
        perEvent: 'Average',
        notCounted: 'Not counted',
        empty: 'No credits in this period.',
        recordsNote: 'Online top-ups come from site payment orders; redeem codes may be sold or given away; referral rebates are not cash income.'
      },
      conversion: {
        title: 'Conversion, repeat purchases, and concentration',
        registered: 'New-user conversion',
        registeredHint: '{paid} of {registered} users registered in the period have credits',
        repeat: 'Repeat rate',
        repeatHint: '{repeat} of {payers} paying users have credits on 2 or more days',
        concentration: 'Top 10 share',
        concentrationHint: 'Top user: {top1}',
        distribution: 'Credits per paying user',
        bucket: '{min} – {max}',
        bucketOpen: '{min} and above',
        bucketUsers: '{count} users'
      },
      payers: {
        title: 'Top payers',
        subtitle: 'Ranked by credits in the period, top 20; select a user for details',
        user: 'User',
        credits: 'Credits',
        events: 'Credits / days',
        eventsValue: '{events} / {days} days',
        channels: 'Sources',
        consumption: 'Spend in period',
        balance: 'Current balance',
        lastCredit: 'Last credit',
        empty: 'No paying users in this period.'
      },
      footnote: 'Credits include online top-ups, redeem codes, admin top-ups, and referral rebates (balance increases on new-api), converted to CNY at your site price. They are not the same as cash received. Balance growth is the change in total user balance over the period.',
      errors: {
        network: 'Network request failed. Check your connection and try again.',
        request: 'Failed to load sales data. Please try again later.',
        invalidRange: 'Invalid period: the start date must not be after the end date, the end date must not be after today, and the period must not exceed 366 days.'
      }
    },
    tickets: {
      title: 'Tickets',
      subtitle: 'Questions from TransitLink users. New tickets and follow-ups are sent to the ticket notification bots chosen in System Settings → Strategy.',
      refresh: 'Refresh',
      loading: 'Loading…',
      disabledHint: "Tickets aren't turned on yet. Turn them on in the Client page and users can send you questions from TransitLink.",
      openClientPage: 'Open the Client page',
      tabs: {
        all: 'All'
      },
      status: {
        open: 'Awaiting reply',
        pending: 'Follow-up',
        replied: 'Replied',
        closed: 'Closed'
      },
      empty: 'No tickets yet.',
      emptyFiltered: 'No tickets with this status.',
      unknownUser: 'Unknown user',
      page: 'Page {page} of {pages}',
      previous: 'Previous',
      next: 'Next',
      badge: '{count} tickets need a reply',
      detail: {
        loading: 'Loading…',
        status: 'Status',
        close: 'Close',
        user: 'User',
        userWithId: '{name} (ID {id})',
        email: 'Email',
        contact: 'Contact',
        client: 'Client',
        you: 'Support (you)',
        closedNote: 'This ticket is closed. Replying reopens it.',
        reply: 'Reply',
        replyPlaceholder: 'Type a reply. Ctrl/⌘ + Enter to send',
        send: 'Send',
        attachmentLoadFailed: "Couldn't load the image",
        closePreview: 'Close preview'
      },
      errors: {
        network: 'Network error. Check your connection.',
        request: 'Request failed. Please try again later.'
      }
    },
    client: {
      title: 'Client',
      subtitle: 'Send your users one link: it opens TransitLink, connects them to your site with your branding, and one-click setup uses the groups and models you recommend.',
      save: 'Save',
      saving: 'Saving…',
      saved: 'Saved',
      cancel: 'Cancel',
      loading: 'Loading…',
      link: {
        title: 'Site link',
        description: 'Opening this link starts TransitLink, connects your site and applies your branding.',
        enabled: 'On',
        enabledHelp: 'When off, the link and the config API stop working. Users who already connected keep working, but branding and notices no longer update.',
        hubUrl: 'Public TransitHub address',
        hubUrlHelp: "Users' computers must be able to reach this address. Only paths under /api/client/ need to be public; the admin console does not.",
        siteUrl: 'Site address for users',
        siteUrlHelp: "Defaults to the workspace's site address. Fill this in if users reach the site at a different address, such as a separate API domain.",
        minVersion: 'Minimum TransitLink version',
        minVersionPlaceholder: 'e.g. 0.2.0; leave blank for no minimum',
        minVersionHelp: 'Users on an older TransitLink are asked to update to the latest version.',
        versions: 'TransitLink versions',
        link: 'Site link',
        unsaved: 'Save once to create the link.',
        needHubUrl: 'The link appears once you fill in the public TransitHub address.',
        otherDomain: "TransitHub's public address and the site address are on different domains, so the first time users open the link, TransitLink asks them to make sure it came from you. Put TransitHub on the site's domain (for example hub.your-domain) and they won't see this.",
        disabledNote: 'Not turned on yet, so the link does not work.',
        copy: 'Copy',
        copied: 'Copied',
        reset: 'Reset link',
        resetConfirm: 'The old link stops working right away, and connected users will fail to refresh their config. Continue?',
        configUrl: 'Config URL',
        openConfig: 'View in browser'
      },
      brand: {
        title: 'Branding',
        description: 'Once connected, TransitLink shows your name, logo and theme color.',
        name: 'Name',
        namePlaceholder: "Leave blank to use the site's own name",
        color: 'Theme color',
        logo: 'Logo',
        upload: 'Upload image',
        remove: 'Remove',
        logoUrl: 'Or enter an image URL (https)',
        logoHelp: 'PNG, JPG, WebP or GIF, up to 256 KB. Square images work best.',
        uploaded: 'Image uploaded. It takes effect after you save.',
        welcome: 'Welcome message',
        welcomePlaceholder: 'For example: Welcome! Contact support any time',
        preview: 'Preview',
        previewName: 'Site name',
        previewHome: 'Home',
        previewTools: 'Tools'
      },
      recommend: {
        title: 'Recommended setup',
        description: 'One-click setup uses these groups and models by default. Users can still change them.',
        claudeCode: 'Claude Code',
        codex: 'Codex',
        group: 'Group',
        model: 'Model',
        groupAuto: 'Not set (the client picks by group name)',
        groupManual: 'Group name (new-api) or group ID (sub2api)',
        missingGroup: '{group} (no longer on the site)',
        claudeModelPlaceholder: "Not set (Claude Code's default model)",
        codexModelPlaceholder: 'Not set (the newest GPT model)',
        groupsLoading: "Loading the site's groups…",
        groupsFailed: "Couldn't load the site's groups: {error}. You can type the group instead."
      },
      notice: {
        title: 'Notice and support',
        description: "The notice and support contacts appear on TransitLink's home page.",
        notice: 'Notice',
        noticePlaceholder: 'For example: maintenance this Saturday 2:00–4:00',
        support: 'Support contacts',
        supportEmpty: 'No support contacts yet.',
        add: 'Add',
        remove: 'Remove',
        type: 'Type',
        label: 'Name, for example: Telegram group',
        value: 'Link, number or email',
        types: {
          telegram: 'Telegram',
          qq: 'QQ',
          wechat: 'WeChat',
          discord: 'Discord',
          email: 'Email',
          url: 'Website',
          other: 'Other'
        }
      },
      tickets: {
        title: 'Tickets',
        description: 'When on, users can send you questions from TransitLink, and you answer them on the Tickets page.',
        enabled: 'On',
        categories: 'Categories',
        categoriesHelp: 'One per line; users pick one when they submit. Up to 20, each at most 40 characters.',
        notifyHelp: 'New tickets and follow-ups can be sent to your notification bots: choose them in System Settings → Strategy.',
        openTickets: 'View tickets'
      },
      errors: {
        invalid: 'Something is not in the right format. Please check and save again.',
        logoTooLarge: 'The logo is too large. Please use an image up to 256 KB.',
        logoType: 'The logo must be PNG, JPG, WebP or GIF.',
        request: 'The request failed. Please try again later.',
        network: 'Network error. Please check your connection.'
      }
    },
    update: {
      title: 'Check for updates',
      checking: 'Checking…',
      current: 'Current version',
      latest: 'Latest version',
      publishedAt: 'released {date}',
      upToDate: "You're on the latest version.",
      available: 'Version {version} is available',
      newVersion: 'Version {version} is available. Click for details',
      badge: 'Update available',
      notes: 'Release notes',
      howTo: 'How to update (Docker)',
      step1: 'Change the image version in docker-compose.prod.yml to {version} (skip this if you use latest).',
      step2: 'Run this in your deployment folder to pull the new image and restart:',
      copy: 'Copy command',
      viewRelease: 'View this release on GitHub',
      recheck: 'Check again',
      close: 'Close',
      online: {
        title: 'Online update',
        description: 'The server downloads {version} from GitHub, verifies it, installs it and restarts. This usually takes a minute or two. The page is briefly unavailable during the restart and reloads by itself when done.',
        start: 'Update to {version}',
        confirm: 'Update to {version} now? Background collections that are running will stop during the restart; you can start them again afterwards.',
        confirmButton: 'Update now',
        cancel: 'Cancel'
      },
      progress: {
        downloading: 'Downloading {version}…',
        downloadLabel: 'Download progress',
        downloaded: '{done} / {total} MB downloaded ({percent}%)',
        installing: 'Download complete. Verifying and installing…',
        restarting: 'Restarting into {version}…',
        reloading: 'Updated to {version}. Reloading the page…',
        keepOpen: 'Keep this page open. It reloads by itself when the update is done.',
        stalled: "The server hasn't come back after 3 minutes. Reload the page in a moment; if it still won't open, check the server logs (docker compose logs app).",
        reverted: "The new version didn't start, so the server went back to {version}. Check the server logs (docker compose logs app) for the cause, or update with Docker as shown below.",
        reload: 'Reload page'
      },
      deployment: {
        notPersistent: "The data directory isn't a persistent volume. Online updates and the credential encryption key are stored there and are lost when the container is recreated. Add ../data/app:/app/data to the app service in docker-compose.prod.yml, then recreate the container.",
        notEncrypted: 'Saved credentials (site sessions, bot secrets, proxy passwords) are not encrypted. Mount the data directory or set CREDENTIAL_ENCRYPTION_KEY, then restart, and they are encrypted automatically.'
      },
      reasons: {
        notAdmin: 'Only the system admin can update online.',
        unsupported: "This server can't update online (it needs to run on Linux with a writable data folder).",
        noPackage: "This release doesn't include an online update package."
      },
      errors: {
        checkFailed: "Couldn't check for updates: GitHub is unreachable or rate-limited. Please try again later.",
        forbidden: 'Only the system admin can update online.',
        inProgress: 'An update is already running. Please wait.',
        unavailable: 'There is no new version to install right now. Please check again.',
        failed: 'The update failed; the current version is unaffected. Try again later, and check the server logs (docker compose logs app) if it keeps failing.'
      }
    },
    collections: {
      running: 'Collecting in background',
      runningProfit: 'Profit data is being collected in the background',
      runningUsers: 'User data is being synced in the background',
      separator: '; ',
      view: 'View',
      goDashboard: 'Sign in on the dashboard',
      dismiss: 'Dismiss',
      unknownError: 'Collection failed. Check again later.',
      profit: {
        done: 'Profit collection finished',
        firstDone: 'First profit collection finished',
        resumed: 'Profit collection resumed and finished',
        doneDetail: 'Data is up to date through {date}',
        failed: 'Profit collection failed'
      },
      users: {
        done: 'User data sync finished',
        firstDone: 'First user data sync finished',
        resumed: 'User data sync resumed and finished',
        doneDetail: '{count} user; sales analysis is updated too | {count} users; sales analysis is updated too',
        failed: 'User data sync failed',
        viewSales: 'View sales'
      }
    },
    profit: {
      title: 'Profit',
      subtitle: 'Gross profit by upstream and group: revenue comes from each route on your own site (sub2api upstream accounts / new-api channels) split by group, cost comes from the usage of registered upstream sites, matched automatically by upstream domain with no manual binding. Read-only.',
      common: {
        placeholder: '—'
      },
      sync: {
        never: 'Not collected yet',
        lastSynced: 'Collected {time}',
        coverage: 'Data covers {range}',
        routes: '{count} route | {count} routes',
        running: 'Collecting…',
        failed: 'The last collection failed: {reason}'
      },
      sources: {
        sub2api_groups: 'Revenue by account and group',
        newapi_flow: 'Revenue from the data dashboard (by channel and group)',
        newapi_log_stat: 'Revenue from per-channel log queries'
      },
      source: {
        basis: 'Revenue converted at ¥{price}/$1 (same as the dashboard)'
      },
      range: {
        label: 'Range',
        lastDays: 'Last {days} days',
        from: 'Start date',
        to: 'End date',
        apply: 'Apply',
        current: 'Range: {range} (complete days up to yesterday)',
        previous: 'Compared with: {range}'
      },
      actions: {
        refresh: 'Refresh',
        sync: 'Collect now',
        startSync: 'Start first collection',
        changePrice: 'Change price',
        goDashboard: 'Sign in on the dashboard',
        goUpstream: 'Register upstream',
        showTable: 'Table',
        showChart: 'Chart',
        expand: 'Show sites and routes of {name}',
        collapse: 'Hide {name}'
      },
      empty: {
        title: 'No profit data yet',
        description: 'Profit analysis reads the revenue of every route on your own site by group, plus the daily usage of registered upstream sites, and matches them by upstream domain. After you sign in to your site, the first collection starts in the background within an hour (it backfills the last 30 days and may take a few minutes), then catches up every hour. You can also start right away with the button below.',
        runningTitle: 'Running the first collection',
        runningDescription: 'Backfilling revenue and upstream cost for the last 30 days. This may take a few minutes; the page refreshes when it finishes.',
        requirements: 'Sign in to your own site’s admin account on the dashboard first, and register upstream sites with their recharge rates under Upstream.'
      },
      coverage: {
        partial: 'Not every day in this range is collected or final yet: collected {range}, final through {complete}. Days without data are not included.',
        previousMissing: 'The previous period has no complete data, so no comparison is available'
      },
      kpi: {
        revenue: 'Revenue',
        revenueHint: 'Usage by your users, converted at your price',
        cost: 'Upstream cost',
        costHint: 'Usage on registered upstream sites × recharge rate',
        costFailedHint: '{count} site-day failed to load; cost may be understated | {count} site-days failed to load; cost may be understated',
        profit: 'Gross profit',
        profitHint: 'Margin {margin}',
        previousMargin: ' (previous {margin})',
        profitDelta: '{value} vs previous',
        attributed: 'Attributed',
        attributedHint: 'Unattributed revenue {value} (cost unknown)',
        unmatched: 'Unmatched cost',
        unmatchedHint: 'Upstreams with cost but no route pointing to them'
      },
      insights: {
        title: 'Needs attention',
        severity: {
          critical: 'Critical',
          warning: 'Warning',
          info: 'Info'
        },
        negativeMargin: 'Upstream {subject} is losing money: cost exceeds revenue by {value}.',
        unmatchedCost: 'Upstream {subject} has cost {value} but no route points to it. The route address may use a different domain than the registered site, or the key is used outside this site.',
        unregistered: '{value} of revenue comes from the unregistered upstream {subject}; its cost is not counted. Register it to see the margin.',
        lowRunway: 'The balance of {subject} lasts about {days}; top up {topUp}.',
        costSpike: 'Yesterday, cost on upstream {subject} was {ratio} of revenue, well above the 7-day median of {baseline}. The upstream may have raised prices, or the key is used outside this site.',
        noRate: '{subject} has no recharge rate, so its cost counts as 0.',
        costFailed: 'Cost for {subject} failed to load on {count} day; that day is not counted. | Cost for {subject} failed to load on {count} days; those days are not counted.',
        priceIncrease: '{subject} raised its multiplier by {change}; based on the last 7 days this costs about {value} more per day.',
        priceIncreaseUnknown: '{subject} raised its multiplier by {change}; the cost impact cannot be estimated yet.'
      },
      trend: {
        title: 'Daily revenue, cost and profit',
        subtitle: 'Lines: revenue and upstream cost; bars: gross profit (may be negative)',
        revenue: 'Revenue',
        cost: 'Upstream cost',
        profit: 'Gross profit',
        margin: 'Margin',
        day: 'Date',
        noData: 'Not collected',
        aria: 'Daily revenue, upstream cost and gross profit for {range}'
      },
      upstreams: {
        title: 'By upstream',
        subtitle: 'Sites and routes under the same domain form one upstream (api.example.com and example.com count as one); click a row for its sites and routes',
        upstream: 'Upstream',
        revenue: 'Revenue',
        cost: 'Cost',
        profit: 'Profit',
        margin: 'Margin',
        balance: 'Balance',
        runway: 'Runway',
        topUp: 'Suggested top-up',
        topUpHint: 'What is missing to cover 7 days at the 7-day average daily cost',
        routes: '{count} route | {count} routes',
        noRoutes: 'No routes',
        noRoutesHint: 'No route points to this upstream; its cost counts as unmatched.',
        runwayDays: '{days} days',
        runwayLong: 'Long',
        sites: 'Upstream sites',
        site: 'Site',
        rechargeRate: 'Recharge rate',
        rangeCost: 'Cost in range',
        avgDailyCost: '7-day avg daily cost',
        deleted: 'Deleted',
        noRate: 'No recharge rate',
        failedDays: '{count} day failed | {count} days failed',
        routeList: 'Routes',
        route: 'Route',
        groups: 'Groups',
        routeRevenue: 'Revenue in range',
        routeDisabled: 'Disabled',
        routeError: 'Error',
        routeDeleted: 'Deleted',
        empty: 'No upstream sites are registered, or this range has no cost or revenue.'
      },
      groups: {
        title: 'By group',
        subtitle: 'Group cost is allocated by the group’s share of revenue on each upstream; profit and margin only cover attributed revenue',
        group: 'Group',
        revenue: 'Revenue',
        cost: 'Allocated cost',
        profit: 'Profit',
        margin: 'Margin',
        unattributed: 'Unattributed',
        none: '(no group)',
        empty: 'No revenue in this range.'
      },
      unattributed: {
        title: 'Unattributed revenue',
        subtitle: 'This revenue has no matching registered upstream; its cost is unknown and it is excluded from margins',
        category: 'Category',
        address: 'Upstream address',
        routes: 'Routes',
        revenue: 'Revenue',
        categories: {
          official: 'Official account / no address',
          unregistered: 'Unregistered upstream',
          unknown: 'Unknown route'
        },
        unknownRoute: 'Deleted or unrecorded route',
        empty: 'All revenue in this range is attributed.'
      },
      priceChanges: {
        title: 'Upstream price changes',
        subtitle: 'Group multiplier changes detected while syncing upstream sites (last 30 days), with the daily cost impact estimated from the group’s last 7 days of usage. Notice only, nothing is repriced.',
        detectedAt: 'Detected',
        site: 'Upstream site',
        group: 'Group',
        multiplier: 'Multiplier',
        avgDailyCost: '7-day avg daily cost',
        impact: 'Daily cost impact',
        noUsage: 'Unused in 7 days',
        noRate: 'No recharge rate',
        failed: 'Cannot estimate',
        empty: 'No upstream group multiplier changes in the last 30 days.'
      },
      footnote: 'Revenue is your users’ usage (site credits) converted to CNY at your site price; cost is the usage on registered upstream sites × recharge rate. Route addresses and upstream sites are matched by domain, and group cost is allocated by revenue share. Data covers complete days (Beijing time) and is collected every hour at :10; runway and suggested top-up use the 7-day average daily cost.',
      warnings: {
        costFailed: 'Some upstream sites failed to report cost, so cost on those days may be understated; the last two days are retried automatically.',
        flowFallback: 'The site’s data dashboard is off (or the version is older), so revenue is queried channel by channel and collection is slower.',
        tooManyRoutes: 'There are more than 500 routes; only the first 500 are counted.'
      },
      errors: {
        authRequired: 'The admin sign-in for your own site has expired. Sign in again on the dashboard.',
        platformUnsupported: 'This site type is not supported (only sub2api and new-api).',
        syncInProgress: 'A collection is already running. Try again later.',
        routesFetch: 'Failed to read the site’s accounts / channels. Try again later.',
        revenueFetch: 'Failed to read the site’s revenue statistics. Try again later.',
        network: 'Network request failed. Check your connection and try again.',
        interrupted: 'The last collection was interrupted by a service restart. It resumes in the background about 2 minutes after startup; no action needed.',
        invalidRange: 'Invalid range: the start cannot be after the end, the end cannot be after yesterday, and the span cannot exceed 366 days.',
        request: 'Failed to load profit data. Try again later.'
      }
    },
    settings: {
      ticketNotify: {
        title: 'Ticket notifications',
        help: 'When a TransitLink user opens a ticket or follows up, notify the selected bots. Select none to turn this off.'
      },
      dailyReport: {
        title: 'Daily business report',
        help: 'Every day at the set time, send yesterday\'s revenue, cost, gross profit, credits, active users, and upstream balance to your bots. It uses local dashboard and User Analytics data and never contacts your sites.',
        time: 'Send time (Beijing time)',
        timeHelp: 'If the service starts after this time, the report is still sent once that day',
        preview: 'Preview yesterday\'s report',
        sendTest: 'Send a test now',
        sent: 'Sent to {count} bots',
        previewTitle: 'Report preview',
        saveHint: 'The switch, time, and bots take effect after you select Save at the top; preview and test sending work without saving.',
        errors: {
          noBots: 'Select at least one bot to receive the report.',
          request: 'Failed to generate or send the report. Please try again later.'
        }
      },
      title: 'System Settings',
      subtitle: 'Manage system parameters, notification channels, and automation strategies.',
      save: 'Save Settings',
      saving: 'Saving...',
      saveSuccess: 'Saved',
      strategyDescription: 'Configure data refresh interval, alert thresholds, and automation strategies.',
      requiresRefresh: 'Enable data refresh interval first so the system can detect changes automatically.',
      balanceWarningAmount: 'Trigger Amount (CNY)',
      notifyBots: 'Send Notifications To',
      customTemplate: 'Custom Template',
      templateEditor: {
        formatLabel: 'Template format',
        formatHelp: 'The live preview uses sample values. Delivery is adapted to each notification channel\'s supported rich-text format.',
        editor: 'Template content',
        preview: 'Live preview',
        previewTitle: 'Live notification template preview',
        formats: {
          text: 'Plain text',
          markdown: 'Markdown',
          html: 'HTML'
        },
        samples: {
          siteName: 'Example upstream',
          balance: '8.50',
          threshold: '10.00',
          groupName: 'Default group',
          oldRate: '1.0000',
          newRate: '1.1200',
          changeDirection: 'increased',
          costImpact: 'about ¥4.00 more per day, based on a 7-day average of ¥20.00'
        }
      },
      balanceTemplateVars: '(Variables: {siteName}, {balance}, {threshold})',
      multiplierTemplateVars: '(Variables: {siteName}, {groupName}, {oldRate}, {newRate}, {changeDirection}, {costImpact})',
      unnamedBot: 'Unnamed Bot',
      noBotsConfigured: 'Configure bots in "Channels & Alerts" first',
      mustSelectBot: 'At least one notification bot must be selected',
      varSiteName: 'Site name',
      varBalance: 'Current balance (CNY)',
      varThreshold: 'Threshold (CNY)',
      varGroupName: 'Group name',
      varOldRate: 'Old rate',
      varNewRate: 'New rate',
      varChangeDirection: 'Change direction',
      varCostImpact: 'Cost impact',
      botNameLabel: 'Bot Name',
      botNameDingtalkPlaceholder: 'e.g., DingTalk Main',
      botNameWecomPlaceholder: 'e.g., WeCom Main',
      botNameQQPlaceholder: 'e.g., QQ Direct Alerts',
      botNameFeishuPlaceholder: 'e.g., Feishu Main',
      botNameTelegramPlaceholder: 'e.g., TG Main',
      addDingtalkBot: 'Add DingTalk Bot',
      addWecomBot: 'Add WeCom Bot',
      addQQBot: 'Add QQ Bot',
      addFeishuBot: 'Add Feishu Bot',
      addTelegramBot: 'Add TG Bot',
      emptyDingtalk: 'No DingTalk bots configured',
      emptyWecom: 'No WeCom bots configured',
      emptyQQ: 'No QQ bots configured',
      emptyFeishu: 'No Feishu bots configured',
      emptyTelegram: 'No Telegram bots configured',
      tabs: {
        strategy: 'Strategy & Automation',
        channels: 'Channels & Alerts',
        templates: 'Message Templates',
        email: 'Email Settings',
        network: 'Network Proxy'
      },
      proxy: {
        title: 'Network Proxy',
        subtitle: 'The proxy TransitHub uses to reach upstream sites (including your own sub2api / new-api site). Changes take effect immediately without a restart. Notification bots are not affected; Telegram has its own proxy setting.',
        loading: 'Loading proxy settings…',
        mode: {
          environment: 'In effect: proxy from the startup environment variables, {proxy}',
          environmentDirect: 'In effect: startup environment variables (no proxy set, connecting directly)',
          proxy: 'In effect: using proxy {proxy}',
          direct: 'In effect: no proxy (connecting directly)'
        },
        unconfiguredHint: 'Proxy settings have not been saved here yet; the form below is prefilled from the startup environment variables. Once saved, these settings take over.',
        readOnly: 'Proxy settings apply to the whole TransitHub instance and can only be changed by the system administrator (the ADMIN_EMAIL account).',
        enable: 'Enable proxy',
        enableHelp: 'When off, all upstream requests connect directly.',
        url: 'Proxy address',
        urlPlaceholder: 'For example http://127.0.0.1:7897; http, https and socks5 are supported',
        urlHelp: "For a proxy that needs credentials, use http://user:password{'@'}host:port. A saved password is shown as *** and is kept unless you change it.",
        urlRequired: 'Enter a proxy address to enable the proxy.',
        bypass: 'Addresses that skip the proxy',
        bypassPlaceholder: 'For example milus.one, *.example.com, 10.0.0.0/8',
        bypassHelp: 'Separate entries with commas or new lines. A domain also matches its subdomains; IPs and CIDR ranges are supported. localhost and 127.0.0.1 always connect directly.',
        save: 'Save proxy settings',
        saved: 'Saved and applied',
        test: {
          title: 'Test connection',
          help: 'Use the unsaved settings above to request an address and check that it is reachable, such as your upstream site URL.',
          target: 'Test address',
          targetPlaceholder: 'https://your-upstream-site',
          run: 'Test connection',
          running: 'Testing…',
          success: 'Connected ({route}, HTTP {status}, {ms} ms)',
          failure: 'Connection failed ({route}, {ms} ms): {reason}',
          routes: {
            proxy: 'via proxy',
            direct: 'direct'
          },
          failures: {
            dns: 'The domain could not be resolved. Check the address.',
            proxy: 'The proxy could not reach the target site, or the proxy itself is unavailable. Check that the proxy is running and its rules or node, or add the address to the bypass list.',
            tls: 'TLS handshake or certificate verification failed.',
            timeout: 'The connection timed out.',
            refused: 'The connection was refused. Check the address and port.',
            network: 'The network connection failed.'
          }
        },
        errors: {
          forbidden: 'Only the system administrator (the ADMIN_EMAIL account) can change proxy settings.',
          invalidUrl: 'Invalid proxy address. Use an http, https or socks5 address such as http://127.0.0.1:7897.',
          invalidTarget: 'Invalid test address. Use an http or https address.',
          request: 'The proxy settings request failed. Try again later.',
          unavailable: 'Proxy settings are currently unavailable.'
        }
      },
      sections: {
        basic: {
          title: 'Basic Settings',
          description: 'Configure basic system operation parameters.',
          refreshInterval: 'Data Refresh Interval',
          refreshIntervalHelp: 'Set the interval for automatically fetching upstream site data. Minimum 60 seconds.',
          seconds: 'Seconds'
        },
        thresholds: {
          title: 'Site Warning Thresholds',
          description: 'Configure default alarm triggers for all upstream sites.',
          balanceWarning: 'Balance Warning',
          balanceWarningHelp: 'Send an alert when an upstream site\'s balance (converted to CNY via recharge rate) falls below the configured amount.',
          multiplierChangeWarning: 'Multiplier Change Alert',
          multiplierChangeWarningHelp: 'Send notifications when the multiplier of any group on an upstream site changes.'
        },
        channels: {
          title: 'Notification Channels',
          description: 'Configure third-party channels for receiving system alerts (e.g., DingTalk, WeCom, QQ, Telegram, Feishu).',
          dingtalk: 'DingTalk Bot',
          dingtalkHelp: 'Configure Webhook and Secret for DingTalk group bot.',
          wecom: 'WeCom Bot',
          wecomHelp: 'Configure the Webhook for a WeCom group bot. No signing secret is required.',
          qq: 'QQ Bot',
          qqHelp: 'Configure the app credentials and recipient user OpenID for an official QQ bot.',
          feishu: 'Feishu Bot',
          feishuHelp: 'Configure Webhook and Secret for Feishu group bot.',
          telegram: 'Telegram Bot',
          telegramHelp: 'Configure Bot Token and Chat ID for Telegram.',
          webhookUrl: 'Webhook URL',
          secret: 'Secret',
          appId: 'AppID',
          appIdPlaceholder: 'Enter the bot AppID',
          appSecret: 'AppSecret',
          appSecretPlaceholder: 'Enter the bot AppSecret',
          userOpenId: 'User OpenID',
          userOpenIdPlaceholder: 'Enter the recipient user openid',
          userOpenIdHelp: 'Add the user to the message-list sandbox, have them scan and message the bot, then read it from the C2C_MESSAGE_CREATE event.',
          botToken: 'Bot Token',
          chatId: 'Chat ID',
          proxyUrl: 'Proxy URL (Optional)',
          proxyUrlPlaceholder: 'e.g. http://127.0.0.1:7890',
          proxyUrlHelp: 'Use a proxy when the server cannot reach Telegram directly. Leave blank for direct connection.',
          loading: 'Loading notification channel settings...',
          testConnection: 'Test Connection',
          testConnectionSuccess: 'Sent Successfully'
        },
        templates: {
          balanceDefaultTemplate: '🔴 **Balance warning**\n\n🏷️ **Site:** {siteName}\n💰 **Current balance:** CNY {balance}\n⚠️ **Warning threshold:** CNY {threshold}\n\nPlease review and recharge the upstream account to avoid service interruption.',
          multiplierDefaultTemplate: '🟠 **Multiplier change warning**\n\n🏷️ **Site:** {siteName}\n📦 **Group:** {groupName}\n📊 **Rate:** {oldRate}x → **{newRate}x** ({changeDirection})\n💸 **Cost impact:** {costImpact}\n\n🔎 Review the cost change and confirm whether downstream pricing needs adjustment.',
          balanceTemplatePlaceholder: 'e.g., 🔴 {siteName} has CNY {balance} remaining, below CNY {threshold}.',
          multiplierTemplatePlaceholder: 'e.g., 🟠 {siteName} / {groupName}: {oldRate}x → {newRate}x.'
        }
      },
      errors: {
        network: 'Network or CORS request failed. Check the API URL and cross-origin settings.',
        request: 'The notification channel test request failed. Try again later.',
        unknown: 'An unknown error occurred while testing the notification channel.',
        invalidChannel: 'The notification channel type is invalid.',
        missingWebhook: 'Enter the robot webhook URL first.',
        missingQQConfig: 'Enter the QQ bot AppID, AppSecret, and User OpenID first.',
        missingTelegramConfig: 'Enter the Telegram Bot Token and Chat ID first.',
        sendFailed: 'Failed to send the test message. Check the robot configuration and network connectivity.'
      },
      smtp: {
        title: 'SMTP Email Settings',
        description: 'Configure the SMTP server used to send system emails.',
        host: 'SMTP Host',
        port: 'Port',
        tlsMode: 'TLS Mode',
        tlsStarttls: 'STARTTLS (587)',
        tlsImplicit: 'Implicit TLS (465)',
        username: 'Username',
        password: 'Password',
        passwordConfigured: 'Password saved',
        passwordNotConfigured: 'No password saved',
        passwordKeepPlaceholder: 'Leave blank to keep the saved password',
        passwordNewPlaceholder: 'Enter SMTP password',
        fromEmail: 'From Email',
        fromName: 'From Name',
        testRecipient: 'Test Recipient',
        saveSuccess: 'SMTP settings saved',
        testEmail: 'Send Test Email',
        testEmailSuccess: 'Test email sent',
        dirtyBeforeTest: 'Save the current SMTP settings before sending a test email',
        errors: {
          validation: 'Check the SMTP settings.',
          missingConfig: 'Save the SMTP settings first.',
          invalidTlsMode: 'The TLS mode is invalid.',
          invalidEmail: 'The email address is invalid.',
          invalidPort: 'Port must be an integer between 1 and 65535.',
          encryptionKeyUnavailable: 'The server has no SMTP encryption key configured.',
          decryptFailed: 'Unable to read the saved SMTP password.',
          sendFailed: 'Failed to send the test email.',
          persistence: 'Failed to save the SMTP settings.'
        }
      },
      emailTemplates: {
        title: 'Email Templates',
        description: 'Create reusable HTML emails and send a test from every saved template.',
        library: 'Template library',
        editor: 'Template editor',
        add: 'New Template',
        builtIn: 'Built-in',
        loading: 'Loading email templates...',
        empty: 'No templates available',
        name: 'Template Name',
        subject: 'Email Subject',
        htmlBody: 'HTML Body',
        preview: 'Email Preview',
        code: 'View source',
        previewTitle: 'Sandboxed email template preview',
        save: 'Save Template',
        delete: 'Delete',
        test: 'Send Test Email',
        testRecipient: 'Test Recipient',
        testRecipientPlaceholder: "name{'@'}example.com",
        unsaved: 'You have unsaved changes',
        dirtyBeforeTest: 'Save the template before sending a test email.',
        discardConfirm: 'This template has unsaved changes. Discard them?',
        deleteConfirm: 'Delete “{name}”? This action cannot be undone.',
        newTemplateName: 'Custom Template',
        newTemplateSubject: 'Enter an email subject',
        newTemplateHtml: '<div style="font-family:Arial,sans-serif;padding:32px"><h1>Write your headline here</h1><p>Compose your email content here.</p></div>',
        createSuccess: 'Template created',
        saveSuccess: 'Template saved',
        deleteSuccess: 'Template deleted',
        testEmailSuccess: 'Test email sent',
        errors: {
          validation: 'Enter a name, a single-line subject, and HTML content no larger than 100KB.',
          invalidEmail: 'The test recipient email is invalid.',
          notFound: 'The email template does not exist or was deleted.',
          builtInProtected: 'Built-in templates cannot be deleted, but they remain editable.',
          limitReached: 'You can create up to 50 custom templates.',
          persistence: 'Failed to save the email template. Try again later.'
        }
      }
    },
    system: {
      version: 'Version {version}',
      openGithubRepository: 'View source on GitHub',
      errors: {
        network: 'System info request failed. Check your network connection.',
        request: 'System request failed. Please try again later.'
      }
    }
  },
  tickets: {
    errors: {
      invalid: 'Something is not valid. Please check and try again.',
      notFound: "This ticket doesn't exist or was deleted.",
      noCurrentAccount: 'Choose a workspace first.',
      request: 'Request failed. Please try again later.'
    }
  }
}
