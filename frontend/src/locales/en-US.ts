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
      openNavigation: 'Open navigation',
      closeNavigation: 'Close navigation'
    },
    menu: {
      dashboard: 'Dashboard',
      upstream: 'Upstream',
      groupManagement: 'Group Management',
      groupRates: 'Group Rates',
      groupAssociations: 'Group Associations',
      connectionHealth: 'Group Health',
      groupRateCampaigns: 'Rate Campaigns',
      settings: 'Settings',
      tickets: 'Tickets',
      massEmail: 'Mass Email',
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
        vsPrev: 'vs prev day'
      },
      loading: 'Loading metrics...',
      loadError: 'Failed to load dashboard metrics.',
      retry: 'Retry',
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
      upstreamKeyUsage: {
        title: 'Today\'s Cost Breakdown',
        subtitle: '{count} keys, {total} total',
        close: 'Close',
        empty: 'No keys with usage today.',
        loadError: 'Failed to load today\'s cost breakdown.',
        retry: 'Retry',
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
            token: 'RT + AT'
          },
          fields: {
            emailPlaceholder: 'Admin email',
            usernamePlaceholder: 'Admin username',
            passwordPlaceholder: 'Admin password',
            accessTokenPlaceholder: 'Access Token (optional, can be empty)',
            refreshTokenPlaceholder: 'Refresh Token (required)',
            tokenTypePlaceholder: 'Token Type (optional, default Bearer)',
            tokenHelp: 'Just a Refresh Token is enough to sign in: the system refreshes once to get the latest expiry and auto-refreshes near expiry.'
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
          reloginRequired: 'Admin verification failed. Please sign in again.'
        }
      }
    },
    groupAssociations: {
      title: 'Group Mappings',
      subtitle: '{count} groups · {associated} associated · {unassociated} unassociated',
      close: 'Close',
      empty: 'No group mappings found.',
      loadError: 'Failed to load group list.',
      runError: 'Failed to run auto-pricing. Please try again.',
      unassociatedLabel: 'No upstream linked',
      unassociatedMultiplier: 'Not available',
      columns: {
        index: '#',
        ownGroup: 'Own Group',
        platform: 'Platform',
        groupType: 'Group Type',
        status: 'Status',
        ownMultiplier: 'Own Multiplier',
        upstreamGroup: 'Upstream Group',
        upstreamMultiplier: 'Upstream Multiplier',
        autoPricing: 'Auto Pricing'
      },
      exclusiveLabels: {
        public: 'Public',
        exclusive: 'Exclusive'
      },
      statusLabels: {
        active: 'Active',
        inactive: 'Inactive'
      },
      autoPricingTip: 'When enabled, automatically adds a markup on top of the upstream multiplier during sync. Supports fixed value or percentage strategies.',
      autoPricingStatus: {
        notConfigured: 'Not configured',
        enabled: 'Enabled',
        savedDisabled: 'Saved, not enabled'
      },
      autoPricingActions: {
        configure: 'Configure',
        edit: 'Edit',
        runNow: 'Run Now',
        runNowFor: 'Run auto-pricing now for {group}'
      },
      lastRun: {
        never: 'Never',
        summary: 'Last: {status} · {trigger} · {time}',
        reason: 'Reason: {reason}',
        triggerManual: 'Manual',
        triggerAfterSync: 'After sync',
        triggerUnknown: 'Unknown trigger',
        reasonUnknown: 'No details available',
        status: {
          applied: 'Success',
          skipped: 'Skipped',
          thresholdExceeded: 'Threshold exceeded',
          failed: 'Failed',
          unknown: 'No run'
        },
        reasons: {
          primary_upstream_not_affected: 'Primary upstream was not affected by the sync.',
          missing_reference_multiplier: 'Reference multiplier is missing.',
          unknown_pricing_source: 'Pricing source is not recognized.',
          status_persist_failed: 'Run status could not be saved.',
          invalid_old_reference_multiplier: 'Previous reference multiplier is invalid.',
          threshold_exceeded: 'Change exceeded the configured threshold.',
          own_group_not_found_in_admin: 'Own group was not found in the admin site.',
          target_unchanged: 'Target multiplier was unchanged.',
          remote_update_failed: 'Remote multiplier update failed.'
        }
      },
      autoPricingDrawer: {
        title: 'Auto-Pricing Config',
        titleWithGroup: '{group} · Auto-Pricing Config',
        enableLabel: 'Enable Auto-Pricing',
        sourceLabel: 'Pricing Source',
        sourcePrimaryUpstream: 'Primary Upstream',
        sourceLowestUpstream: 'Lowest Upstream',
        sourceHighestUpstream: 'Highest Upstream',
        sourceAverageUpstream: 'Average Upstream',
        primaryUpstreamLabel: 'Primary Upstream',
        primaryUpstreamPlaceholder: 'Select primary upstream',
        strategyLabel: 'Markup Method',
        strategyFixed: 'Fixed Increase',
        strategyPercentage: 'Percentage Increase',
        fixedIncreaseLabel: 'Fixed Increase Value',
        percentageIncreaseLabel: 'Percentage Increase Value',
        thresholdLabel: 'Follow Threshold',
        thresholdHelp: 'Auto-follow only when upstream change is within this percentage',
        minMultiplierLabel: 'Min Multiplier',
        maxMultiplierLabel: 'Max Multiplier',
        estimatedMultiplier: 'Estimated Multiplier',
        save: 'Save Config',
        cancel: 'Cancel',
        noUpstreams: 'No upstreams linked to this group. Cannot configure auto-pricing.',
        noMultiplierData: 'No upstream multiplier data available. Cannot compute estimated multiplier.',
        tips: {
          minMultiplier: 'The calculated multiplier will not go below this value. Use it to protect your minimum margin. Leave empty for no lower limit.',
          maxMultiplier: 'The calculated multiplier will not go above this value. Use it to avoid sudden price spikes for users. Leave empty for no upper limit.',
          threshold: 'Auto-follow only when the upstream multiplier changes within this percentage. Larger changes should wait for manual confirmation to avoid abnormal upstream swings changing your group price.',
          minMultiplierAria: 'View min multiplier help',
          maxMultiplierAria: 'View max multiplier help',
          thresholdAria: 'View follow threshold help',
        },
        guidance: {
          title: 'Recommended Settings',
          minMultiplier: 'Min Multiplier: your cost + minimum profit margin',
          maxMultiplier: 'Max Multiplier: the highest price users would still accept',
          threshold: 'Follow Threshold: 10%',
          exampleTitle: 'Calculation Example',
          exampleOld: 'Upstream old multiplier: 1.00',
          exampleNew: 'Upstream new multiplier: 1.08',
          exampleThreshold: 'Follow threshold: 10%',
          exampleMarkup: 'Markup method: upstream + 0.10',
          exampleMin: 'Min multiplier: 1.00',
          exampleMax: 'Max multiplier: 1.30',
          exampleResult: 'The change is 8%, within the 10% threshold, so auto-follow is allowed. The final multiplier is 1.18, which falls within the 1.00–1.30 limit range.',
        },
        notify: {
          sectionTitle: 'Auto-Pricing Success Notification',
          enableLabel: 'Send notification after pricing update',
          enableHelp: 'Send a bot notification when auto-pricing actually updates the group multiplier.',
          botSelectLabel: 'Notification Bots',
          botSelectPlaceholder: 'Select bots to notify',
          noBots: 'No bots available. Please configure bots in System Settings > Notifications & Channels first.',
          templateLabel: 'Notification Template',
          templateHelp: 'Leave empty to use the default template. Supported variables:',
          templatePlaceholder: 'Leave empty to use default template',
          defaultTemplate: '[Auto Pricing] {ownGroup} was adjusted from {oldOwnMultiplier}x to {newOwnMultiplier}x. Reference: {upstreamSiteName} / {upstreamGroupName}, multiplier {oldReference}x -> {newReference}x.',
          variablesTitle: 'Available Variables',
          varOwnGroup: 'Own group name',
          varUpstreamSiteName: 'Upstream site name',
          varUpstreamGroupName: 'Upstream group name / source',
          varOldReference: 'Old reference multiplier',
          varNewReference: 'New reference multiplier',
          varOldOwnMultiplier: 'Multiplier before adjustment',
          varNewOwnMultiplier: 'Multiplier after adjustment',
          varStrategy: 'Pricing strategy',
          varFixedIncrease: 'Fixed increase value',
          varPercentageIncrease: 'Percentage increase value',
          varThreshold: 'Follow threshold',
          copied: 'Copied',
        },
        errors: {
          primaryRequired: 'Primary upstream must be selected in primary upstream mode.',
          increaseNonNegative: 'Increase value cannot be negative.',
          thresholdNonNegative: 'Threshold cannot be negative.',
          multiplierNonNegative: 'Multiplier cannot be negative.',
          minGreaterThanMax: 'Min multiplier cannot be greater than max multiplier.',
          invalidConfig: 'Invalid auto-pricing config. Please check and try again.',
          notifyBotsRequired: 'At least one bot must be selected when notifications are enabled.',
        }
      },
      save: 'Save',
      saveSuccess: 'Saved',
      saving: 'Saving...',
      saveError: 'Save failed. Please try again.'
    },
    connectionHealth: {
      title: 'Group Health',
      subtitle: 'Independent lightweight probing of accounts/channels inside the current admin workspace groups, with health monitoring and automatic degrade/restore.',
      adminSubtitle: 'Shows all groups under the current admin workspace. Click the account count to view accounts/channels and their independent probe status.',
      refresh: 'Refresh',
      empty: 'No probeable accounts/channels under the current admin workspace.',
      adminEmpty: 'No groups under the current admin workspace.',
      notConnected: 'Not connected',
      notProbed: 'Not probed yet',
      notConfigured: 'No probe model configured',
      groupTypes: {
        public: 'Public',
        exclusive: 'Exclusive',
        subscription: 'Subscription'
      },
      groupStatusLabels: {
        active: 'Active',
        inactive: 'Inactive'
      },
      adminColumns: {
        name: 'Name',
        platform: 'Platform',
        type: 'Type',
        multiplier: 'Multiplier',
        accounts: 'Accounts',
        accountsUnit: '',
        status: 'Group Status',
        probeOverview: 'Probe Overview',
        detail: 'Details'
      },
      adminOverview: {
        probeable: 'Probeable {probeable}/{total}',
        noneProbeable: 'No probeable targets',
        noProbe: '{count} pending probe'
      },
      probeUnavailableReasons: {
        credential_unavailable: 'Cannot securely obtain upstream credentials; probing unavailable',
        secure_verification_required: 'Upstream root secure verification is required to read the channel key',
        base_url_unavailable: 'No usable Base URL; probing unavailable',
        model_unavailable: 'No probe model available (configure one in a probe policy)',
        export_unavailable: 'Upstream account export endpoint unavailable; cannot obtain credentials',
        credentials_redacted: 'Upstream credentials are redacted and cannot be used for probing'
      },
      accountsDialog: {
        multiplier: 'Multiplier',
        unknownPlatform: 'Unknown platform',
        unknownStatus: 'Unknown status',
        empty: 'No accounts/channels under this group.',
        noProbeData: 'No probe data',
        unprobeable: 'Not probeable',
        unassignedPolicy: 'No policy assigned',
        unassignedPolicyHint: 'No policy assigned — will not be auto-probed. Manual one-time probing is still available.',
        assignedPolicyCount: '{name} and {count} more',
        assignPolicy: 'Assign policy',
        columns: {
          name: 'Name',
          platform: 'Platform',
          type: 'Type',
          status: 'Status',
          priority: 'Priority',
          concurrency: 'Concurrency',
          weight: 'Weight',
          models: 'Models',
          probeStatus: 'Probe Status',
          policyAssignment: 'Policy Assignment',
          actions: 'Actions'
        }
      },
      summary: {
        total: 'Probe Targets',
        unconfigured: 'Not Configured'
      },
      stateLabels: {
        healthy: 'Healthy',
        degraded: 'Degraded',
        suspended: 'Suspended',
        observing: 'Observing',
        recovering: 'Recovering',
        disabled: 'Disabled'
      },
      providerLabels: {
        gemini: 'Gemini',
        anthropic: 'Anthropic',
        openai: 'OpenAI',
        custom: 'Custom'
      },
      filters: {
        allGroups: 'All Own Groups',
        allSites: 'All Upstream Sites',
        allStates: 'All States',
        allProviders: 'All Model Types',
        searchGroup: 'Search group name...',
        allPlatforms: 'All Platforms',
        allTypes: 'All Types'
      },
      columns: {
        model: 'Model',
        state: 'State',
        weight: 'Weight',
        lastProbe: 'Last Probe',
        lastError: 'Last Error'
      },
      actions: {
        probe: 'Probe Now',
        disable: 'Disable',
        restore: 'Restore',
        viewEvents: 'View Events'
      },
      errorKeys: {
        ok: 'OK',
        network_fluctuation: 'Network Fluctuation',
        rate_limited: 'Rate Limited',
        server_error: 'Upstream Server Error',
        auth: 'Auth Failed',
        model_not_found: 'Model Not Found',
        invalid_response: 'Invalid Response',
        unsupported: 'Unsupported',
        manual_disable: 'Manually Disabled',
        manual_restore: 'Manually Restored'
      },
      topActions: {
        runFlow: 'How it works',
        policies: 'Probe Policies',
        events: 'Probe Events'
      },
      events: {
        title: 'Recent Probes & Remote Actions',
        empty: 'No events yet.',
        emptyForConnection: 'No events for this target yet.',
        showAll: 'Show All'
      },
      eventsDialog: {
        subtitle: 'Review probe health for each model of this probe target (account/channel).',
        globalSubtitle: 'Recent probe and remote action events.',
        viewingConnection: 'Viewing events for this target',
        card: {
          latencyLabel: 'Chat Latency',
          pingLabel: 'Node PING',
          availabilityLabel: 'Availability',
          recentRecordsLabel: 'Last 60 Records',
          past: 'PAST',
          now: 'NOW',
          noData: '-',
          nextProbeIn: 'Next probe: in {seconds}s',
          nextProbeDue: 'Next probe: due, waiting for scheduler',
          nextProbeNoPolicy: 'Next probe: no policy configured',
          nextProbeNeverProbed: 'Next probe: not probed yet',
          nextProbeDisabled: 'Next probe: disabled, not auto-probed',
          remoteActionLine: 'Remote action: {label}'
        }
      },
      remoteActions: {
        unsupported: 'Unsupported (no upstream call made)',
        skippedIndependentProbe: 'Skipped — auto remote action is not enabled',
        sub2apiInactive: 'Sub2API account switched to inactive',
        sub2apiActive: 'Sub2API account switched to active',
        sub2apiInactiveFailed: 'Sub2API account switch to inactive failed',
        sub2apiActiveFailed: 'Sub2API account switch to active failed',
        newapiDisabled: 'NewAPI channel disabled',
        newapiWeight: 'NewAPI channel weight set to {weight}',
        other: '{action}'
      },
      policies: {
        title: 'Probe Policies',
        subtitle: 'Configure model probe targets, thresholds, and auto-degrade/restore behavior.',
        create: 'New Policy',
        empty: 'No probe policies yet. Click "New Policy" to configure one.',
        enabled: 'Enabled',
        disabled: 'Disabled',
        enable: 'Enable',
        disable: 'Disable',
        edit: 'Edit',
        remoteActionOn: 'Remote Action On',
        allGroupsScope: 'All groups',
        modelTargetCount: '{count} model targets'
      },
      policyDrawer: {
        createTitle: 'New Probe Policy',
        editTitle: 'Edit Probe Policy',
        nameLabel: 'Policy Name',
        namePlaceholder: 'Enter policy name',
        enabledLabel: 'Enable this policy',
        ownGroupLabel: 'Policy Scope',
        ownGroupAllOption: 'All groups in current workspace',
        modelTargetsLabel: 'Model Probe Targets',
        addModelTarget: 'Add Model',
        modelNamePlaceholder: 'Model name, e.g. gpt-4o-mini',
        modelEnabledLabel: 'Enabled',
        maxProbeTokensLabel: 'Max tokens',
        probePromptPlaceholder: 'Probe prompt (leave empty for default)',
        probeIntervalLabel: 'Probe Interval (seconds)',
        dailyBudgetLabel: 'Daily Probe Budget',
        failureThresholdLabel: 'Failure Threshold',
        successThresholdLabel: 'Recovery Success Threshold',
        cooldownLabel: 'Cooldown (seconds)',
        observationLabel: 'Observation Window (seconds)',
        recoveryStepLabel: 'Recovery Step Percent',
        autoDegradeLabel: 'Auto Degrade',
        autoDegradeHelp: 'Automatically lower local weight or suspend a link once failures reach the threshold.',
        autoRemoteActionLabel: 'Auto Remote Action',
        autoRemoteActionHelp: 'NewAPI: auto remote action updates the channel weight/status. Sub2API: auto remote action toggles the admin account active/inactive; priority is not auto-adjusted. In the current group-health independent probing path, Sub2API already supports automatic active/inactive switching by policy; the NewAPI target dimension does not implement remote actions yet and is only recorded as unsupported — it never actually calls upstream.',
        providerLabel: 'Model Provider',
        providerPlaceholder: 'Select a provider',
        providerMismatchWarning: 'This policy\'s existing model targets use different providers. Pick one provider above — saving will unify all model targets to the provider you select.',
        cancel: 'Cancel',
        save: 'Save Policy',
        tooltips: {
          ownGroup: 'Describes the business group scope for this policy. Independent group-health probing enables policies through explicit target assignments and uses this policy\'s enabled model targets as the model pool. If the target has its own model list, the intersection of target models and the policy model pool is used; otherwise the policy model pool is used directly.',
          modelTargets: 'The models this policy probes. Both automatic scheduling and manual probes run against exactly these models.',
          provider: 'A probe policy can only use one provider (openai / anthropic / gemini / custom). Every model target added below automatically uses this provider, so a single policy never mixes providers.',
          probeInterval: 'Automatic scheduling checks whether a model is due using "last probe time + this interval". Consecutive failures also trigger an escalating 2/5/10-minute backoff on the backend.',
          dailyBudget: 'Caps how many real probe requests this workspace can run per day. Once the budget is used up, real probe requests are skipped to avoid excessive cost — this is expected, not a system error.',
          failureThreshold: 'Consecutive soft failures reaching this count will suspend/degrade the link. Some hard failures (e.g. auth failure) may suspend it immediately without degrading first.',
          successThreshold: 'During the observation window, this many consecutive successful probes are required before the link is considered truly recovered and returns to healthy.',
          cooldown: 'After a link is suspended, the scheduler will not run automatic probes against it until this cooldown period ends.',
          observation: 'After a manual restore or an automatic recovery flow, the link enters an observation window — consecutive probe results here confirm whether it is actually stable again.',
          recoveryStep: 'During recovery, each successful probe raises local weight by this percentage step, instead of jumping straight to 100%.',
          autoDegrade: 'When enabled, probe results drive the health state machine and adjust local routing weight. When disabled, probe results are only recorded — state and weight never change automatically.',
          autoRemoteAction: 'When enabled, supported upstream actions run when the state machine triggers degrade/recovery: Sub2API group-health targets toggle the admin account active/inactive without changing priority; NewAPI linked-channel probing can update channel weight/status. The current NewAPI target dimension is not implemented yet and is recorded as unsupported. When disabled, probes and state results are recorded without remote disable/restore actions.'
        },
        runFlow: {
          buttonLabel: 'How it works',
          title: 'Probe Health: How It Works',
          subtitle: 'A complete explanation for admins covering how policies, scheduling, the state machine, and manual probing relate to each other.',
          close: 'Close run flow explanation',
          steps: {
            policyScope: {
              title: '1. How a policy takes effect',
              description: 'Group health uses independent probing: probe targets are the accounts (Sub2API) / channels (NewAPI) inside admin groups of the current admin workspace, not real_connections links. An account/channel is auto-probed only after a policy is explicitly assigned to it. The probed models come from the enabled model targets on assigned policies; if the target carries its own model list, the intersection of target models and the policy model pool is used, otherwise the policy model pool is used directly.'
            },
            modelProvider: {
              title: '2. How model targets take effect',
              description: 'Each probe policy corresponds to exactly one provider (openai / anthropic / gemini / custom); every model target added under that policy belongs to that same provider, so a single policy never mixes models from multiple providers. Both automatic scheduling and manual probes fire probe requests against the candidate models derived in the previous step, one by one.'
            },
            schedulerCadence: {
              title: '3. Automatic scheduling cadence',
              description: 'A dedicated backend scheduler scans for due probe tasks roughly every 30 seconds across all probeable targets in the current workspace. The scheduling unit is "one probe target (account/channel) + one model" — multiple candidate models under the same target are split into separate tasks that are each evaluated independently.'
            },
            dueCheck: {
              title: '4. How "due" is determined',
              description: 'A (target, model) pair that has never been probed is scheduled for a probe as soon as possible. Once it has been probed, the next due time is "last probe time + the policy\'s probe interval". Consecutive failures additionally trigger an escalating 2 / 5 / 10-minute backoff so a persistently broken target isn\'t retried too aggressively.'
            },
            budget: {
              title: '5. Budget rules',
              description: 'Every policy has a "daily probe budget" that caps how many real probe requests this workspace can run per day. Once the budget is exhausted, the scheduler skips real probe requests and does not write new probe events — so a model can keep showing "due, waiting for scheduler" with no new events even though it is genuinely due. This is expected behavior caused by the budget limit, not a system fault.'
            },
            stateTransition: {
              title: '6. State transitions',
              description: 'A successful probe clears that model\'s consecutive failure count. Consecutive soft failures (e.g. network fluctuation, rate limiting — recoverable errors) first move the link into a degraded state and gradually lower local weight by the recovery step percentage before the failure threshold is reached; once the threshold is reached the model is suspended. Some hard failures (e.g. auth failure, model not found) may skip degradation and suspend the model immediately.'
            },
            cooldownObservation: {
              title: '7. Cooldown and observation',
              description: 'Once a target/model is suspended, it enters the policy\'s configured cooldown period, during which the scheduler will not run automatic probes against it. After cooldown ends — or after an admin manually restores it — the target enters an observation phase: consecutive probe results during this window determine whether the target has genuinely stabilized, and only enough consecutive successes to reach the "recovery success threshold" moves it back to healthy.'
            },
            autoDegradeVsRemoteAction: {
              title: '8. Auto Degrade vs. Auto Remote Action',
              description: 'Auto Degrade only affects the internal state machine and local display weight; it never calls any upstream platform API, so it is low-risk. Auto Remote Action only calls upstream when the policy explicitly enables it AND the state machine decides a remote action is warranted: for NewAPI linked-channel probing this changes channel weight/status; in the current group-health independent probing path, Sub2API targets toggle the admin account active/inactive (priority is never adjusted), while the NewAPI target dimension does not implement remote actions yet and is recorded as unsupported without calling upstream. When a policy does not enable Auto Remote Action, both paths only record "skipped" and never call any upstream API.'
            },
            manualProbe: {
              title: '9. Manual probing',
              description: 'Manual probing is a one-time, on-demand test, fully isolated from policy auto-probing: opening the dialog has the backend re-resolve credentials for that targetId and query the upstream /v1/models endpoint for the live model list — the frontend never sees base_url/key. After selecting models and clicking "Start Test", results are shown only inside the dialog; nothing is written to probe state/events, no probe budget is consumed, and no auto degrade/restore is triggered. Policy auto-probing only applies to accounts/channels that have been explicitly assigned a policy; accounts/channels without an assignment can still be manually probed at any time, they just won\'t be auto-probed by the scheduler.'
            },
            nextProbeCopy: {
              title: '10. What "next probe" copy means',
              description: '"Next probe: due, waiting for scheduler" means the time-based due point has already passed, but actually running the probe still requires the backend scheduler\'s next scan (roughly every 30 seconds), an available concurrent probe slot, enough remaining daily probe budget, and the target not currently being in failure backoff or cooldown. A probe only actually fires once all of these conditions are met at the same time.'
            }
          }
        },
        errors: {
          nameRequired: 'Please enter a policy name.',
          modelTargetRequired: 'At least one probe target with a model name is required.',
          providerRequired: 'Please select a provider for this policy.'
        }
      },
      probeDialog: {
        title: 'Select Probe Models',
        cancel: 'Cancel',
        confirm: 'Start Probe',
        emptyTitle: 'This target has no probeable models.',
        emptyHint: 'Add and enable model targets in a probe policy first.',
        fromPolicy: 'From policy "{name}"',
        maxTokens: 'Max {count} tokens',
        remoteActionOn: 'Remote Action On',
        noResults: 'Probe ran but returned no results. Try again later or check upstream site connectivity.'
      },
      manualProbeDialog: {
        title: 'Manual One-Time Probe',
        loadingModels: 'Fetching available models from upstream...',
        retryLoad: 'Retry',
        empty: 'No available models were found.',
        selectHint: 'Select the models to test — multiple selection allowed.',
        startTest: 'Start Test',
        testing: 'Testing...',
        resultTitle: 'Test Results',
        resultEmpty: 'No test run yet. Select models and click "Start Test".',
        latency: '{ms}ms',
        selectedCount: '{count} model(s) selected',
        close: 'Close'
      },
      policyAssignment: {
        title: 'Assign Probe Policies',
        subtitle: 'manage automatic probing assignments',
        save: 'Save',
        cancel: 'Cancel',
        empty: 'No probe policies in this workspace yet. Create one first.'
      },
      errors: {
        request: 'Operation failed. Please try again.',
        network: 'Network error. Check your connection and try again.',
        notFound: 'Probe target not found or inaccessible.',
        noMatchingModels: 'Selected models do not match the current probe policy.',
        accountsFetch: 'Failed to load accounts for this group.',
        targetNotFound: 'Probe target not found or not in the current workspace.',
        credentialUnavailable: 'Cannot securely obtain upstream credentials; probing unavailable.',
        secureVerificationRequired: 'Upstream root secure verification is required to read the channel key.',
        baseUrlUnavailable: 'No usable Base URL; probing unavailable.',
        modelUnavailable: 'No probe model available; configure one in a probe policy first.',
        exportUnavailable: 'Upstream account export endpoint unavailable; cannot obtain credentials.',
        credentialsRedacted: 'Upstream credentials are redacted and cannot be used for probing.',
        modelListUnavailable: 'Could not fetch the upstream model list. Please try again later.',
        modelListInvalid: 'The upstream model list response format is not recognized.',
        manualModelsRequired: 'Please select at least one model before starting the test.',
        policyNotFound: 'The selected policy does not exist or is not in the current workspace.'
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
            tokenHelp: 'Use this for Sub2API sites where Cloudflare or two-factor login blocks direct password login.'
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
          rechargeRate: 'Recharge Multiplier',
          rechargeRatePlaceholder: 'Enter the USD to CNY multiplier, e.g. 7',
          rechargeRateHelp: 'Required. CNY amount = USD amount × this multiplier.',
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
        isConnected: 'Integration',
        connected: 'Connected',
        disconnected: 'Disconnected',
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
        network: 'Network or CORS request failed. Check the site URL and cross-origin settings.',
        auth: 'Login failed. Check the account or password.',
        request: 'The upstream API request failed. Try again later.',
        invalidResponse: 'The upstream response could not be parsed.',
        tokenMissing: 'Login succeeded but no access token was returned.',
        detect: 'The platform could not be auto-detected. Choose a platform and try again.',
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
        delta: 'Rise/Fall',
        updatedAt: 'Updated Time',
        actions: 'Actions'
      },
      actions: {
        refresh: 'Refresh Data',
        createCampaign: 'Create Campaign',
        viewHistory: 'View History',
        viewHistoryForRate: 'View rise/fall history for {site} · {group}; current change {delta}',
        closeHistory: 'Close History',
        editType: 'Edit',
        closeEdit: 'Close Group Type Editor',
        connect: 'Click to Connect',
        closeConnect: 'Close connect dialog',
        saveConnect: 'Connect',
        cancel: 'Cancel',
        saveType: 'Save Type'
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
        mapped: 'Mapped',
        unmapped: 'Unmapped',
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
        mapped: 'Connected',
        unmapped: 'Not Connected',
        deleted: 'Deleted'
      },
      empty: {
        title: 'No group rates yet',
        description: 'Synced upstream group multiplier data will appear here.'
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
      edit: {
        title: 'Edit Group Type',
        titleWithGroup: 'Edit Group Type for {site} · {group}',
        description: 'Saving updates the multiplier type for this site group and refreshes the list.',
        typeLabel: 'Group Type',
        typePlaceholder: 'Select group type'
      },
      connect: {
        titleWithGroup: 'Connect {site} · {group}',
        description: 'Choose one of your site groups to add this upstream group to its mapping.',
        ownGroupLabel: 'My Site Groups',
        ownGroupPlaceholder: 'Select my site groups',
        upstreamGroupLabel: 'Connected Group',
        upstreamGroupPlaceholder: 'Select connected group',
        upstreamSiteLabel: 'Upstream Site',
        upstreamGroupNameLabel: 'Upstream Group',
        upstreamMultiplierLabel: 'Upstream Multiplier',
        upstreamPlatformLabel: 'Platform',
        modeData: 'Data Stats',
        modeReal: 'Real Connect',
        realDescription: 'This will automatically create an API key on the upstream site and a forwarding account on your admin site.',
        groupTypeLabel: 'Group Type',
        groupTypePlaceholder: 'Select group type',
        groupTypeOpenai: 'OpenAI',
        groupTypeAnthropic: 'Anthropic',
        groupTypeGemini: 'Gemini',
        groupTypeAntigravity: 'Antigravity',
        channelTypeLabel: 'Channel Type',
        channelTypePlaceholder: 'Select channel type',
        realNotSupported: 'Real connect is not supported for this platform',
        realConnecting: 'Creating connection...',
        realSuccess: 'Real connection created successfully',
        realFailed: 'Failed to create real connection',
        modeBind: 'Manual Bind',
        bindDescription: 'Select an existing upstream Key to bind to this group without creating new resources.',
        bindSelectKey: 'Select Upstream Key',
        bindKeysLoading: 'Loading key list...',
        bindKeysEmpty: 'No keys available for this site',
        bindFailed: 'Failed to bind'
      },
      disconnect: {
        action: 'Disconnect',
        title: 'Disconnect',
        description: 'Confirm disconnecting {site} · {group}?',
        unlinkOnly: 'Unlink Only',
        unlinkOnlyHint: 'Only remove the local binding record, keep the upstream Key and Admin account',
        deleteAll: 'Delete Account & Key',
        deleteAllHint: 'Also delete the upstream Key and the Admin site forwarding account',
        confirm: 'Confirm',
        disconnecting: 'Disconnecting...',
        failed: 'Failed to disconnect'
      },
      format: {
        multiplier: '{value}x',
        deltaMultiplier: '{value}x'
      },
      errors: {
        network: 'Network or CORS request failed. Check the API URL and cross-origin settings.',
        request: 'The group rates API request failed. Try again later.',
        unknown: 'An unknown error occurred while loading group rates.',
        refreshFailed: 'The change was saved, but the list refresh failed. Refresh again to update the view.'
      }
    },
    groupRateCampaigns: {
      title: 'Rate Campaigns',
      subtitle: 'Batch-adjust your own group multipliers, with scheduled start/end and automatic restore.',
      common: {
        placeholder: '—'
      },
      actions: {
        create: 'New Campaign',
        refresh: 'Refresh',
        start: 'Start Now',
        end: 'End Campaign',
        cancel: 'Cancel Campaign',
        viewDetail: 'View Details',
        close: 'Close',
        preview: 'Preview Impact',
        confirmCreate: 'Create Campaign',
        cancelEdit: 'Cancel'
      },
      tabs: {
        all: 'All'
      },
      status: {
        draft: 'Draft',
        scheduled: 'Scheduled',
        running: 'Running',
        ending: 'Ending',
        ended: 'Ended',
        partial: 'Partial',
        failed: 'Failed',
        cancelled: 'Cancelled',
        loading: 'Loading campaigns...'
      },
      fields: {
        name: 'Campaign Name',
        status: 'Status',
        startAt: 'Start Time',
        endAt: 'End Time',
        summary: 'Result',
        createdBy: 'Created By',
        actions: 'Actions'
      },
      empty: {
        title: 'No campaigns yet',
        description: 'Click "New Campaign" to create your first rate campaign.'
      },
      pagination: {
        total: '{total} total',
        pageSize: '{pageSize} per page',
        currentPage: 'Page {page} / {totalPages}',
        previous: 'Previous',
        next: 'Next'
      },
      format: {
        summary: '{applied}/{total} applied'
      },
      errors: {
        network: 'Network or CORS request failed. Check the API URL and cross-origin settings.',
        request: 'The rate campaigns API request failed. Try again later.',
        unknown: 'An unknown error occurred while loading rate campaigns.',
        emptySelection: 'Select at least one group, and every group must exist in your own groups.',
        invalidName: 'Invalid campaign name. It must be between 1 and 80 characters.',
        invalidAdjustment: 'Invalid campaign multiplier. Check that every selected group has a valid fixed rate.',
        invalidSchedule: 'Invalid schedule. Please check the start/end time settings.',
        noNotifyBots: 'Select at least one bot when notifications are enabled.',
        notFound: 'Campaign not found.',
        invalidState: 'This action is not allowed for the campaign\'s current status.',
        duplicateGroup: 'The same group cannot be selected more than once.'
      },
      editor: {
        titleCreate: 'New Rate Campaign',
        sectionInfo: 'Campaign Info',
        nameLabel: 'Campaign Name',
        namePlaceholder: 'e.g. Double 11 Sale',
        descriptionLabel: 'Description',
        descriptionPlaceholder: 'Optional, for your own reference',
        sectionSelection: 'Select Groups',
        selectionHint: 'Set a campaign multiplier for each group',
        groupsEmpty: 'No groups available',
        groupMultiplierPlaceholder: 'Campaign multiplier',
        sectionSchedule: 'Schedule',
        startModeLabel: 'Start Mode',
        startNow: 'Start Now',
        startScheduled: 'Scheduled Start',
        startDraft: 'Save as Draft',
        startAtLabel: 'Start Time',
        endModeLabel: 'End Mode',
        endScheduled: 'Scheduled End',
        endManual: 'Manual End',
        endAtLabel: 'End Time',
        sectionNotify: 'Notifications',
        notifyEnableLabel: 'Enable Bot Notifications',
        notifyBotSelectLabel: 'Select Bots',
        notifyNoBots: 'No bots available. Configure one in system settings first.',
        notifyStartTemplateLabel: 'Start Notification Text',
        notifyEndTemplateLabel: 'End Notification Text',
        notifyVariablesTitle: 'Available variables, click to copy',
        notifyVarActivityName: 'Campaign name',
        notifyVarTotalCount: 'Total target groups',
        notifyVarAppliedCount: 'Applied count',
        notifyVarFailedCount: 'Failed count',
        notifyVarStartTime: 'Start time',
        notifyVarEndTime: 'End time',
        copyVarFailed: 'Copy failed. Please copy the variable manually.',
        previewTitle: 'Preview Affected Groups',
        previewEmpty: 'No preview yet. Click "Preview Impact" to see affected groups.',
        previewGroupName: 'Group Name',
        previewOriginal: 'Original',
        previewCampaign: 'Campaign',
        previewTotal: '{total} groups affected',
        errors: {
          nameRequired: 'Please enter a campaign name',
          selectionEmpty: 'Please select at least one group',
          groupMultiplierInvalid: 'Enter a valid campaign multiplier for every group',
          scheduleInvalid: 'Please check the start/end time settings',
          notifyBotsRequired: 'Please select at least one bot when notifications are enabled'
        }
      },
      detail: {
        title: 'Campaign Details',
        sectionConfig: 'Campaign Config',
        sectionItems: 'Group Details',
        itemGroupName: 'Group Name',
        itemOriginal: 'Original',
        itemCampaign: 'Campaign',
        itemRestored: 'Restored',
        itemApplyStatus: 'Start Status',
        itemRestoreStatus: 'Restore Status',
        noReason: '—',
        confirmEnd: 'Are you sure you want to manually end this campaign? All groups will be restored to their original multipliers immediately.',
        confirmCancel: 'Are you sure you want to cancel this campaign? No pricing changes will be applied.'
      }
    },
    mySites: {
      errors: {
        invalidAutoPricingConfig: 'Invalid auto-pricing config: primary upstream not in linked upstreams, or min multiplier exceeds max.'
      }
    },
    tickets: {
      title: 'Tickets',
      subtitle: 'View and reply to user tickets submitted through the embedded iframe.',
      common: {
        placeholder: '—'
      },
      actions: {
        refresh: 'Refresh',
        viewDetail: 'View Details',
        embedSettings: 'Embed Settings'
      },
      tabs: {
        all: 'All'
      },
      status: {
        open: 'Open',
        pending: 'Pending',
        replied: 'Replied',
        closed: 'Closed',
        loading: 'Loading tickets...'
      },
      fields: {
        title: 'Title',
        status: 'Status',
        category: 'Category',
        priority: 'Priority',
        manualEmail: 'Contact Email',
        sub2apiUser: 'Sub2API User',
        sub2apiSrcHost: 'Source Host',
        lastMessageAt: 'Last Reply',
        actions: 'Actions'
      },
      empty: {
        title: 'No tickets yet',
        description: 'This workspace has not received any tickets yet.'
      },
      pagination: {
        total: '{total} total',
        pageSize: '{pageSize} per page',
        currentPage: 'Page {page} of {totalPages}',
        previous: 'Previous',
        next: 'Next'
      },
      errors: {
        network: 'Network or CORS request failed. Check the API URL and cross-origin settings.',
        request: 'The ticket request failed. Try again later.',
        unknown: 'An unknown error occurred while loading tickets.',
        notFound: 'Ticket not found.',
        invalidStatus: 'Invalid ticket status.',
        bodyRequired: 'Reply content is required.',
        ticketClosed: 'This ticket is closed and can no longer be replied to.',
        noCurrentAccount: 'Please select a workspace first.',
        invalidTemplate: 'Unsupported embed page template.',
        invalidMaxImages: 'Max images per ticket must be between 0 and 9.',
        attachmentLoadFailed: 'Failed to load image, please try again later.',
        invalidCategoryOptions: 'Invalid category options. Check for empty, duplicate, or over-limit entries.',
        invalidPriorityOptions: 'Invalid priority options. Check for empty, duplicate, or over-limit entries.'
      },
      detail: {
        title: 'Ticket Details',
        sectionTicket: 'Ticket Info',
        sectionMessages: 'Messages',
        sectionReply: 'Reply',
        category: 'Category',
        priority: 'Priority',
        manualEmail: 'Contact Email',
        lastMessageAt: 'Last Reply',
        sub2apiUserId: 'Sub2API User ID',
        sub2apiEmail: 'Sub2API Email',
        sub2apiRole: 'Sub2API Role',
        sub2apiSrcHost: 'Source Host',
        authorAdmin: 'Support',
        authorCustomer: 'User',
        replyPlaceholder: 'Type your reply...',
        send: 'Send Reply',
        attachmentLoadFailed: 'Failed to load image',
        previewImage: 'Zoom preview',
        closePreview: 'Close preview'
      },
      embedConfig: {
        title: 'Embed Settings',
        sections: {
          basic: 'Basic Settings',
          category: 'Category',
          priority: 'Priority'
        },
        legacyNotice: 'The "Enable Embedded Tickets" and "Allowed Source Host" settings have been removed. The embed URL is always active; contact an administrator if you need to restrict access.',
        embedUrl: 'Embed URL',
        embedUrlHint: 'Configure this URL in the Sub2API custom iframe. Sub2API automatically appends user identity parameters.',
        copy: 'Copy',
        copied: 'Copied',
        copyFailed: 'Copy failed, please copy manually.',
        openPreview: 'Open Ticket Page',
        openPreviewHint: 'Opens the embed page in a new tab for preview. Identity parameters will be missing outside a real Sub2API iframe — that is expected.',
        template: 'Page Template',
        templates: {
          default: {
            name: 'Default Compact',
            description: 'Standard rounded card style, suitable for default use.'
          },
          minimal: {
            name: 'Minimal',
            description: 'A lighter visual density, suitable for embedding into an existing admin style.'
          },
          support: {
            name: 'Support Panel',
            description: 'A more conversational look, suitable for use as a standalone support panel.'
          }
        },
        maxImages: 'Max Images Per Ticket',
        maxImagesHint: '0 disables image uploads. Maximum 9 images.',
        categoryOptions: 'Category Options',
        priorityOptions: 'Priority Options',
        addOption: 'Add Option',
        addOptionPlaceholder: 'Type a new option and click add',
        removeOption: 'Remove this option',
        restoreDefaults: 'Restore Defaults',
        optionsHint: 'Keep at least 1 option, up to 40 characters each. Customers must pick from these when creating a ticket.',
        saveTemplate: 'Save Settings',
        saving: 'Saving...',
        saveSuccess: 'Settings saved',
        rotateToken: 'Rotate Embed URL',
        confirmRotate: 'Are you sure you want to rotate the embed URL? The old embed URL will stop working immediately.'
      },
      sub2apiProfile: {
        title: 'Sub2API User Profile',
        sectionIdentity: 'Identity',
        userId: 'User ID',
        email: 'Email',
        role: 'Role',
        srcHost: 'Source Host',
        username: 'Username',
        status: 'Account Status',
        sectionBalance: 'Balance & Recharge',
        balance: 'Current Balance',
        totalRecharged: 'Total Recharged',
        registeredAt: 'Registered At',
        frozenBalance: 'Frozen Balance',
        concurrency: 'Concurrency',
        rpmLimit: 'RPM Limit',
        lastUsedAt: 'Last Used At',
        unavailable: 'Not available',
        sectionRechargeHistory: 'Recharge History',
        rechargeHistoryComingSoon: 'Not provided yet',
        historyEmpty: 'No recharge history yet',
        empty: 'No data',
        remoteUnavailable: {
          noUserId: 'This ticket has no recorded Sub2API user ID, so live data cannot be queried.',
          noAdminSession: 'The current workspace is not logged into a Sub2API admin account. Showing ticket snapshot data only.',
          userNotFound: 'Could not fetch live data for this user from Sub2API. Showing ticket snapshot data only.'
        }
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
        itemGeneric: 'Recipient delivery failed. Check the batch later for updated details.'
      }
    },
    settings: {
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
      balanceTemplateVars: '(Variables: {siteName}, {balance}, {threshold})',
      multiplierTemplateVars: '(Variables: {siteName}, {groupName}, {oldRate}, {newRate}, {changeDirection})',
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
      pricingAmount: 'Adjustment Amount',
      botNameLabel: 'Bot Name',
      botNameDingtalkPlaceholder: 'e.g., DingTalk Main',
      botNameFeishuPlaceholder: 'e.g., Feishu Main',
      botNameTelegramPlaceholder: 'e.g., TG Main',
      addDingtalkBot: 'Add DingTalk Bot',
      addFeishuBot: 'Add Feishu Bot',
      addTelegramBot: 'Add TG Bot',
      emptyDingtalk: 'No DingTalk bots configured',
      emptyFeishu: 'No Feishu bots configured',
      emptyTelegram: 'No Telegram bots configured',
      tabs: {
        strategy: 'Strategy & Automation',
        channels: 'Channels & Alerts',
        templates: 'Message Templates',
        email: 'Email Settings'
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
          multiplierChangeWarningHelp: 'Send notifications when any mapped group multiplier changes.'
        },
        pricing: {
          title: 'Auto-Pricing',
          description: 'Configure auto-adjustment rules when an upstream mapped group multiplier increases.',
          enableAutoPricing: 'Auto-Pricing',
          enableAutoPricingHelp: 'Automatically adjust "Own Group" multiplier when an upstream group multiplier increases.',
          strategy: 'Adjustment Strategy',
          strategyFixed: 'Fixed Increase (+)',
          strategyPercentage: 'Percentage Increase (%)',
          fixedValuePlaceholder: 'e.g., 0.1',
          percentageValuePlaceholder: 'e.g., 10'
        },
        channels: {
          title: 'Notification Channels',
          description: 'Configure third-party channels for receiving system alerts (e.g., DingTalk, Telegram, Feishu).',
          dingtalk: 'DingTalk Bot',
          dingtalkHelp: 'Configure Webhook and Secret for DingTalk group bot.',
          feishu: 'Feishu Bot',
          feishuHelp: 'Configure Webhook and Secret for Feishu group bot.',
          telegram: 'Telegram Bot',
          telegramHelp: 'Configure Bot Token and Chat ID for Telegram.',
          webhookUrl: 'Webhook URL',
          secret: 'Secret',
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
          balanceTemplatePlaceholder: 'e.g., [Warning] {siteName} balance (CNY) is below {threshold}, current is {balance}.',
          multiplierTemplatePlaceholder: 'e.g., [Rate Change] {siteName} {groupName} changed from {oldRate}x to {newRate}x.'
        }
      },
      errors: {
        network: 'Network or CORS request failed. Check the API URL and cross-origin settings.',
        request: 'The notification channel test request failed. Try again later.',
        unknown: 'An unknown error occurred while testing the notification channel.',
        invalidChannel: 'The notification channel type is invalid.',
        missingWebhook: 'Enter the robot webhook URL first.',
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
      openRelease: 'View release notes',
      openGithubRepository: 'View source on GitHub',
      errors: {
        network: 'System info request failed. Check your network connection.',
        request: 'System request failed. Please try again later.'
      }
    }
  },
  embed: {
    tickets: {
      page: {
        loading: 'Loading ticket system...'
      },
      list: {
        title: 'My Tickets',
        refresh: 'Refresh',
        create: 'New Ticket',
        loading: 'Loading tickets...',
        emptyTitle: 'No tickets yet',
        emptyDescription: 'Click "New Ticket" to submit your first question.'
      },
      createModal: {
        title: 'New Ticket'
      },
      form: {
        manualEmail: 'Contact Email',
        manualEmailPlaceholder: 'Enter the email to receive replies',
        title: 'Title',
        titlePlaceholder: 'Briefly describe your issue',
        body: 'Details',
        bodyPlaceholder: 'Describe the issue in detail',
        category: 'Category',
        categoryPlaceholder: 'Select a category',
        priority: 'Priority',
        priorityPlaceholder: 'Select a priority',
        submit: 'Submit Ticket',
        cancel: 'Cancel',
        images: 'Images',
        imagesCount: '{count} / {max}',
        addImage: 'Add Image',
        imagesHint: 'JPEG/PNG/WEBP/GIF only, up to 5MB per image.'
      },
      detail: {
        back: 'Back to list',
        loading: 'Loading ticket details...',
        support: 'Support',
        you: 'You',
        replyPlaceholder: 'Type your reply...',
        send: 'Send',
        closedNotice: 'This ticket is closed and can no longer be replied to.'
      },
      attachments: {
        loadFailed: 'Failed to load image'
      },
      status: {
        open: 'Open',
        pending: 'Pending',
        replied: 'Replied',
        closed: 'Closed'
      },
      errors: {
        network: 'Network or CORS request failed. Check the API URL and cross-origin settings.',
        request: 'The ticket request failed. Try again later.',
        unknown: 'An unknown error occurred. Please try again later.',
        missingParams: 'This page is missing required parameters and cannot start a session. If you opened it via the "Open Ticket Page" preview button, this is expected — open this page from within a real Sub2API iframe instead.',
        formIncomplete: 'Please fill in the contact email, title, and details, and select a category and priority.',
        configNotFound: 'The embed configuration does not exist.',
        disabled: 'The ticket feature has been disabled by the administrator.',
        invalidSrcHost: 'Invalid source host.',
        srcHostMismatch: 'The source host is not trusted.',
        sub2apiAuth: 'Identity verification failed. Please refresh the page and try again.',
        sub2apiRequest: 'Failed to fetch user info. Please try again later.',
        userMismatch: 'User identity verification failed.',
        sessionInvalid: 'Session expired. Please refresh the page and try again.',
        invalidEmail: 'Please enter a valid email address.',
        titleRequired: 'Please enter a title.',
        bodyRequired: 'Please enter the details.',
        categoryRequired: 'Please select a category.',
        priorityRequired: 'Please select a priority.',
        invalidCategory: 'The selected category is not part of the current ticket configuration. Please choose again.',
        invalidPriority: 'The selected priority is not part of the current ticket configuration. Please choose again.',
        ticketClosed: 'This ticket is closed and can no longer be replied to.',
        tooManyImages: 'The number of images exceeds the current limit.',
        invalidImageType: 'Only JPEG/PNG/WEBP/GIF images are supported.',
        imageTooLarge: 'Each image must be 5MB or smaller.',
        emptyImage: 'The image is empty, please choose another one.',
        attachmentLoadFailed: 'Failed to load image, please try again later.'
      }
    }
  }
}
