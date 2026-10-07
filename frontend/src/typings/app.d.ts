/** The global namespace for the app */
declare namespace App {
  /** Theme namespace */
  namespace Theme {
    type ColorPaletteNumber = import("@sa/color").ColorPaletteNumber;

    /** NaiveUI theme overrides that can be specified in preset */
    type NaiveUIThemeOverride = import("naive-ui").GlobalThemeOverrides;

    /** Theme setting */
    interface ThemeSetting {
      /** Theme scheme */
      themeScheme: UnionKey.ThemeScheme;
      /** grayscale mode */
      grayscale: boolean;
      /** colour weakness mode */
      colourWeakness: boolean;
      /** Whether to recommend color */
      recommendColor: boolean;
      /** Theme color */
      themeColor: string;
      /** Theme radius */
      themeRadius: number;
      /** Other color */
      otherColor: OtherColor;
      /** Whether info color is followed by the primary color */
      isInfoFollowPrimary: boolean;
      /** Layout */
      layout: {
        /** Layout mode */
        mode: UnionKey.ThemeLayoutMode;
        /** Scroll mode */
        scrollMode: UnionKey.ThemeScrollMode;
      };
      /** Page */
      page: {
        /** Whether to show the page transition */
        animate: boolean;
        /** Page animate mode */
        animateMode: UnionKey.ThemePageAnimateMode;
      };
      /** Header */
      header: {
        /** Header height */
        height: number;
        /** Header breadcrumb */
        breadcrumb: {
          /** Whether to show the breadcrumb */
          visible: boolean;
          /** Whether to show the breadcrumb icon */
          showIcon: boolean;
        };
        /** Multilingual */
        multilingual: {
          /** Whether to show the multilingual */
          visible: boolean;
        };
        globalSearch: {
          /** Whether to show the GlobalSearch */
          visible: boolean;
        };
      };
      /** Tab */
      tab: {
        /** Whether to show the tab */
        visible: boolean;
        /**
         * Whether to cache the tab
         *
         * If cache, the tabs will get from the local storage when the page is refreshed
         */
        cache: boolean;
        /** Tab height */
        height: number;
        /** Tab mode */
        mode: UnionKey.ThemeTabMode;
        /** Whether to close tab by middle click */
        closeTabByMiddleClick: boolean;
      };
      /** Fixed header and tab */
      fixedHeaderAndTab: boolean;
      /** Sider */
      sider: {
        /** Inverted sider */
        inverted: boolean;
        /** Sider width */
        width: number;
        /** Collapsed sider width */
        collapsedWidth: number;
        /** Sider width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or 'top-hybrid-header-first' */
        mixWidth: number;
        /**
         * Collapsed sider width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or
         * 'top-hybrid-header-first'
         */
        mixCollapsedWidth: number;
        /** Child menu width when the layout is 'vertical-mix', 'top-hybrid-sidebar-first', or 'top-hybrid-header-first' */
        mixChildMenuWidth: number;
        /** Whether to auto select the first submenu */
        autoSelectFirstMenu: boolean;
      };
      /** Footer */
      footer: {
        /** Whether to show the footer */
        visible: boolean;
        /** Whether fixed the footer */
        fixed: boolean;
        /** Footer height */
        height: number;
        /**
         * Whether float the footer to the right when the layout is 'top-hybrid-sidebar-first' or
         * 'top-hybrid-header-first'
         */
        right: boolean;
      };
      /** Watermark */
      watermark: {
        /** Whether to show the watermark */
        visible: boolean;
        /** Watermark text */
        text: string;
        /** Whether to use user name as watermark text */
        enableUserName: boolean;
        /** Whether to use current time as watermark text */
        enableTime: boolean;
        /** Time format for watermark text */
        timeFormat: string;
      };
      /** define some theme settings tokens, will transform to css variables */
      tokens: {
        light: ThemeSettingToken;
        dark?: {
          [K in keyof ThemeSettingToken]?: Partial<ThemeSettingToken[K]>;
        };
      };
    }

    interface OtherColor {
      info: string;
      success: string;
      warning: string;
      error: string;
    }

    interface ThemeColor extends OtherColor {
      primary: string;
    }

    type ThemeColorKey = keyof ThemeColor;

    type ThemePaletteColor = {
      [key in ThemeColorKey | `${ThemeColorKey}-${ColorPaletteNumber}`]: string;
    };

    type BaseToken = Record<string, Record<string, string>>;

    interface ThemeSettingTokenColor {
      /** the progress bar color, if not set, will use the primary color */
      nprogress?: string;
      container: string;
      layout: string;
      inverted: string;
      "base-text": string;
    }

    interface ThemeSettingTokenBoxShadow {
      header: string;
      sider: string;
      tab: string;
    }

    interface ThemeSettingToken {
      colors: ThemeSettingTokenColor;
      boxShadow: ThemeSettingTokenBoxShadow;
    }

    type ThemeTokenColor = ThemePaletteColor & ThemeSettingTokenColor;

    /** Theme token CSS variables */
    type ThemeTokenCSSVars = {
      colors: ThemeTokenColor & { [key: string]: string };
      boxShadow: ThemeSettingTokenBoxShadow & { [key: string]: string };
    };
  }

  /** Global namespace */
  namespace Global {
    type VNode = import("vue").VNode;
    type RouteLocationNormalizedLoaded =
      import("vue-router").RouteLocationNormalizedLoaded;
    type RouteKey = import("@elegant-router/types").RouteKey;
    type RouteMap = import("@elegant-router/types").RouteMap;
    type RoutePath = import("@elegant-router/types").RoutePath;
    type LastLevelRouteKey = import("@elegant-router/types").LastLevelRouteKey;

    /** The router push options */
    type RouterPushOptions = {
      query?: Record<string, string>;
      params?: Record<string, string>;
      force?: boolean;
    };

    /** The global header props */
    interface HeaderProps {
      /** Whether to show the logo */
      showLogo?: boolean;
      /** Whether to show the menu toggler */
      showMenuToggler?: boolean;
      /** Whether to show the menu */
      showMenu?: boolean;
    }

    /** The global menu */
    type Menu = {
      /**
       * The menu key
       *
       * Equal to the route key
       */
      key: string;
      /** The menu label */
      label: string;
      /** The menu i18n key */
      i18nKey?: I18n.I18nKey | null;
      /** The route key */
      routeKey: RouteKey;
      /** The route path */
      routePath: RoutePath;
      /** The menu icon */
      icon?: () => VNode;
      /** The menu children */
      children?: Menu[];
    };

    type Breadcrumb = Omit<Menu, "children"> & {
      options?: Breadcrumb[];
    };

    /** Tab route */
    type TabRoute = Pick<
      RouteLocationNormalizedLoaded,
      "name" | "path" | "meta"
    > &
      Partial<
        Pick<RouteLocationNormalizedLoaded, "fullPath" | "query" | "matched">
      >;

    /** The global tab */
    type Tab = {
      /** The tab id */
      id: string;
      /** The tab label */
      label: string;
      /**
       * The new tab label
       *
       * If set, the tab label will be replaced by this value
       */
      newLabel?: string;
      /**
       * The old tab label
       *
       * when reset the tab label, the tab label will be replaced by this value
       */
      oldLabel?: string;
      /** The tab route key */
      routeKey: LastLevelRouteKey;
      /** The tab route path */
      routePath: RouteMap[LastLevelRouteKey];
      /** The tab route full path */
      fullPath: string;
      /** The tab fixed index */
      fixedIndex?: number | null;
      /**
       * Tab icon
       *
       * Iconify icon
       */
      icon?: string;
      /**
       * Tab local icon
       *
       * Local icon
       */
      localIcon?: string;
      /** I18n key */
      i18nKey?: I18n.I18nKey | null;
    };

    /** Form rule */
    type FormRule = import("naive-ui").FormItemRule;

    /** The global dropdown key */
    type DropdownKey =
      | "closeCurrent"
      | "closeOther"
      | "closeLeft"
      | "closeRight"
      | "closeAll"
      | "pin"
      | "unpin";
  }

  /**
   * I18n namespace
   *
   * Locales type
   */
  namespace I18n {
    type RouteKey = import("@elegant-router/types").RouteKey;

    type LangType = "en-US" | "zh-CN";

    type LangOption = {
      label: string;
      key: LangType;
    };

    type I18nRouteKey = Exclude<RouteKey, "root" | "not-found">;

    type FormMsg = {
      required: string;
      invalid: string;
    };

    type Schema = {
      system: {
        title: string;
        updateTitle: string;
        updateContent: string;
        updateConfirm: string;
        updateCancel: string;
      };
      common: {
        action: string;
        add: string;
        addSuccess: string;
        backToHome: string;
        batchDelete: string;
        cancel: string;
        close: string;
        check: string;
        selectAll: string;
        expandColumn: string;
        columnSetting: string;
        config: string;
        confirm: string;
        delete: string;
        deleteSuccess: string;
        confirmDelete: string;
        edit: string;
        warning: string;
        error: string;
        index: string;
        keywordSearch: string;
        logout: string;
        logoutConfirm: string;
        lookForward: string;
        modify: string;
        modifySuccess: string;
        noData: string;
        operate: string;
        pleaseCheckValue: string;
        refresh: string;
        reset: string;
        search: string;
        switch: string;
        tip: string;
        trigger: string;
        update: string;
        updateSuccess: string;
        userCenter: string;
        yesOrNo: {
          yes: string;
          no: string;
        };
        save: string;
      };
      request: {
        logout: string;
        logoutMsg: string;
        logoutWithModal: string;
        logoutWithModalMsg: string;
        refreshToken: string;
        tokenExpired: string;
      };
      theme: {
        themeDrawerTitle: string;
        tabs: {
          appearance: string;
          layout: string;
          general: string;
          preset: string;
        };
        appearance: {
          themeSchema: { title: string } & Record<UnionKey.ThemeScheme, string>;
          grayscale: string;
          colourWeakness: string;
          themeColor: {
            title: string;
            followPrimary: string;
          } & Record<Theme.ThemeColorKey, string>;
          recommendColor: string;
          recommendColorDesc: string;
          themeRadius: {
            title: string;
          };
          preset: {
            title: string;
            apply: string;
            applySuccess: string;
            [key: string]:
              | {
                  name: string;
                  desc: string;
                }
              | string;
          };
        };
        layout: {
          layoutMode: { title: string } & Record<
            UnionKey.ThemeLayoutMode,
            string
          > & {
              [K in `${UnionKey.ThemeLayoutMode}_detail`]: string;
            };
          tab: {
            title: string;
            visible: string;
            cache: string;
            cacheTip: string;
            height: string;
            mode: { title: string } & Record<UnionKey.ThemeTabMode, string>;
            closeByMiddleClick: string;
            closeByMiddleClickTip: string;
          };
          header: {
            title: string;
            height: string;
            breadcrumb: {
              visible: string;
              showIcon: string;
            };
          };
          sider: {
            title: string;
            inverted: string;
            width: string;
            collapsedWidth: string;
            mixWidth: string;
            mixCollapsedWidth: string;
            mixChildMenuWidth: string;
            autoSelectFirstMenu: string;
            autoSelectFirstMenuTip: string;
          };
          footer: {
            title: string;
            visible: string;
            fixed: string;
            height: string;
            right: string;
          };
          content: {
            title: string;
            scrollMode: { title: string; tip: string } & Record<
              UnionKey.ThemeScrollMode,
              string
            >;
            page: {
              animate: string;
              mode: { title: string } & Record<
                UnionKey.ThemePageAnimateMode,
                string
              >;
            };
            fixedHeaderAndTab: string;
          };
        };
        general: {
          title: string;
          watermark: {
            title: string;
            visible: string;
            text: string;
            enableUserName: string;
            enableTime: string;
            timeFormat: string;
          };
          multilingual: {
            title: string;
            visible: string;
          };
          globalSearch: {
            title: string;
            visible: string;
          };
        };
        configOperation: {
          saveConfig: string;
          saveSuccessMsg: string;
          resetConfig: string;
          resetSuccessMsg: string;
        };
      };
      route: Record<I18nRouteKey, string>;
      page: {
        login: {
          common: {
            loginOrRegister: string;
            userNamePlaceholder: string;
            phonePlaceholder: string;
            codePlaceholder: string;
            passwordPlaceholder: string;
            confirmPasswordPlaceholder: string;
            codeLogin: string;
            confirm: string;
            back: string;
            validateSuccess: string;
            loginSuccess: string;
            welcomeBack: string;
          };
          pwdLogin: {
            title: string;
            rememberMe: string;
            forgetPassword: string;
            register: string;
            otherAccountLogin: string;
            otherLoginMode: string;
            superAdmin: string;
            admin: string;
            user: string;
          };
          codeLogin: {
            title: string;
            getCode: string;
            reGetCode: string;
            sendCodeSuccess: string;
            imageCodePlaceholder: string;
          };
          register: {
            title: string;
            agreement: string;
            protocol: string;
            policy: string;
          };
          resetPwd: {
            title: string;
          };
          bindWeChat: {
            title: string;
          };
        };
        home: {
          greeting: string;
          todayTrainings: string;
          totalTrainings: string;
          totalVocabulary: string;
          totalNotes: string;
          trainingTrend: string;
          trainingType: string;
          trainingTypeStats: string;
          trainingCount: string;
          creativity: string;
          projectNews: {
            title: string;
            desc1: string;
            desc2: string;
            desc3: string;
            desc4: string;
            desc5: string;
            moreNews: string;
          };
        };
        system: {
          user: {
            title: string;
            userName: string;
            nickname: string;
            role: string;
            createdAt: string;
            actions: string;
            searchPlaceholder: string;
            addUser: string;
            editUser: string;
            deleteUserConfirm: string;
            createSuccess: string;
            updateSuccess: string;
            deleteSuccess: string;
            passwordPlaceholder: string;
            passwordEditPlaceholder: string;
            nicknamePlaceholder: string;
            rolePlaceholder: string;
            userNameRequired: string;
            passwordRequired: string;
            roleRequired: string;
            quotaMonth: string;
            quotaUnlimited: string;
            quotaPlaceholder: string;
            proxyLogin: string;
            proxyLoginConfirm: string;
            proxyLoginSuccess: string;
            resetPassword: string;
            resetPasswordTitle: string;
            resetModeRandom: string;
            resetModeSpecify: string;
            newPasswordPlaceholder: string;
            newPasswordRequired: string;
            passwordStrengthHint: string;
            resetSuccess: string;
            generatedPasswordLabel: string;
            generatedPasswordTip: string;
            copyPassword: string;
            copySuccess: string;
            resetClear2FA: string;
          };
          permission: {
            title: string;
            roleList: string;
            permissionConfig: string;
            addRole: string;
            addPermission: string;
            saveConfig: string;
            selectRoleTip: string;
            superAdminTip: string;
            permissionSaveSuccess: string;
            roleCreateSuccess: string;
            roleDeleteSuccess: string;
            permissionCreateSuccess: string;
            permissionUpdateSuccess: string;
            permissionDeleteSuccess: string;
            deleteConfirm: string;
            deleteRoleConfirm: string;
            deletePermissionConfirm: string;
            roleName: string;
            roleNamePlaceholder: string;
            roleCode: string;
            roleCodePlaceholder: string;
            description: string;
            descriptionPlaceholder: string;
            permissionName: string;
            permissionNamePlaceholder: string;
            permissionCode: string;
            permissionCodePlaceholder: string;
            groupName: string;
            groupNamePlaceholder: string;
            roleInfoRequired: string;
          };
          aiConfig: {
            title: string;
            addProvider: string;
            editProvider: string;
            addModel: string;
            editModel: string;
            manageModel: string;
            providerName: string;
            providerNamePlaceholder: string;
            apiKeyPlaceholder: string;
            baseUrlPlaceholder: string;
            enabled: string;
            modelCode: string;
            modelCodePlaceholder: string;
            displayName: string;
            displayNamePlaceholder: string;
            setDefault: string;
            runParams: string;
            status: string;
            default: string;
            notConfigured: string;
            enabledStatus: string;
            disabledStatus: string;
            deleteProviderConfirm: string;
            deleteModelConfirm: string;
            createSuccess: string;
            updateSuccess: string;
            deleteSuccess: string;
            actions: string;
            testConnection: string;
            testSuccess: string;
            testFailed: string;
            testing: string;
            toolManagement: string;
            addTool: string;
            editTool: string;
            toolName: string;
            toolDescription: string;
            toolParams: string;
            toolEnabled: string;
            selectToolType: string;
            noTools: string;
            allToolsAdded: string;
            backToSelect: string;
            deleteToolConfirm: string;
            testNeedApiKey: string;
            apiKeyEditPlaceholder: string;
            toolConfirmRequired: string;
            configuring: string;
            paramDefaultSuffix: string;
          };
          config: {
            title: string;
            on: string;
            off: string;
            register: string;
            registerDesc: string;
            registerEnabledMsg: string;
            registerDisabledMsg: string;
            telegram: string;
            telegramDesc: string;
            botToken: string;
            botTokenPlaceholder: string;
            webhookUrl: string;
            webhookPlaceholder: string;
            saveTelegram: string;
            telegramEnabledMsg: string;
            telegramDisabledMsg: string;
            telegramSaved: string;
            timeout: string;
            timeoutDesc: string;
            aiTimeout: string;
            aiTlsHandshake: string;
            aiResponseHeader: string;
            httpTimeout: string;
            minutes: string;
            seconds: string;
            saveTimeout: string;
            timeoutSaved: string;
            memory: string;
            memoryDesc: string;
            extractionModel: string;
            extractionModelPlaceholder: string;
            sessionIdle: string;
            minUserMessages: string;
            messages: string;
            saveMemory: string;
            memoryEnabledMsg: string;
            memoryDisabledMsg: string;
            memorySaved: string;
            smtp: string;
            smtpDesc: string;
            smtpHost: string;
            smtpPort: string;
            encryption: string;
            noEncryption: string;
            username: string;
            usernamePlaceholder: string;
            password: string;
            passwordPlaceholder: string;
            fromEmail: string;
            fromName: string;
            fromNamePlaceholder: string;
            saveSmtp: string;
            smtpEnabledMsg: string;
            smtpDisabledMsg: string;
            smtpSaved: string;
            sendTestEmail: string;
            s3: string;
            s3Desc: string;
            s3EndpointPlaceholder: string;
            s3RegionPlaceholder: string;
            s3MaxUpload: string;
            virtualHostStyle: string;
            saveS3: string;
            s3EnabledMsg: string;
            s3DisabledMsg: string;
            s3Saved: string;
            rag: string;
            ragDesc: string;
            ragBaseUrl: string;
            ragModel: string;
            ragApiKeyPlaceholder: string;
            chunkSize: string;
            chunkOverlap: string;
            topK: string;
            saveRag: string;
            testConnection: string;
            ragEnabledMsg: string;
            ragDisabledMsg: string;
            ragSaved: string;
            sendTestEmailTitle: string;
            recipient: string;
            recipientPlaceholder: string;
            recipientRequired: string;
            emailSubject: string;
            testEmailSubject: string;
            emailContent: string;
            emailContentPlaceholder: string;
            testEmailContent: string;
            send: string;
            testEmailSent: string;
            sendFailed: string;
            loadFailed: string;
            saveFailed: string;
            unknownError: string;
            embeddingConfigRequired: string;
            embeddingTestFailed: string;
            embeddingTestFailedWithReason: string;
            connectionSuccess: string;
            remarkRegister: string;
            remarkTelegramEnabled: string;
            remarkTelegramWebhook: string;
            remarkAiTimeout: string;
            remarkAiTlsHandshake: string;
            remarkAiResponseHeader: string;
            remarkHttpTimeout: string;
            remarkSmtpEnabled: string;
            remarkSmtpHost: string;
            remarkSmtpPort: string;
            remarkSmtpEncryption: string;
            remarkSmtpUser: string;
            remarkSmtpPassword: string;
            remarkSmtpFrom: string;
            remarkSmtpFromName: string;
            remarkS3Enabled: string;
            remarkS3Endpoint: string;
            remarkS3Region: string;
            remarkS3Secure: string;
            remarkS3PathStyle: string;
            remarkS3MaxUpload: string;
            remarkRagEnabled: string;
            remarkRagBaseUrl: string;
            remarkRagModel: string;
            remarkRagApiKey: string;
            remarkRagChunkSize: string;
            remarkRagChunkOverlap: string;
            remarkRagTopK: string;
            remarkMemoryEnabled: string;
            remarkMemoryModel: string;
            remarkMemoryIdle: string;
            remarkMemoryMinMessages: string;
          };
        };
        ai: {
          training: {
            title: string;
            subtitle: string;
            customTraining: string;
            addTraining: string;
            editTraining: string;
            createTraining: string;
            noCustomTraining: string;
            noCustomTrainingTip: string;
            noDescription: string;
            deleteConfirm: string;
            titleLabel: string;
            titlePlaceholder: string;
            descLabel: string;
            descPlaceholder: string;
            promptLabel: string;
            promptPlaceholder: string;
            welcomeLabel: string;
            welcomePlaceholder: string;
            iconLabel: string;
            colorLabel: string;
            langLabel: string;
            speedLabel: string;
            chinese: string;
            english: string;
            japanese: string;
            createSuccess: string;
            updateSuccess: string;
            deleteSuccess: string;
            titleRequired: string;
            promptRequired: string;
            loadFailed: string;
            operationFailed: string;
            deleteFailed: string;
            trainingNotExist: string;
            icons: Record<string, string>;
            colors: Record<string, string>;
          };
          chat: Record<string, string>;
          history: Record<string, string>;
          share: Record<string, string>;
          vocabulary: Record<string, string>;
          note: {
            title: string;
            noteTitle: string;
            noteTitlePlaceholder: string;
            noTitle: string;
            category: string;
            content: string;
            noteContentPlaceholder: string;
            createdAt: string;
            actions: string;
            view: string;
            edit: string;
            deleteConfirm: string;
            loadFailed: string;
            deleteSuccess: string;
            deleteFailed: string;
            fieldsRequired: string;
            updateSuccess: string;
            addSuccess: string;
            operationFailed: string;
            searchTitlePlaceholder: string;
            searchCategoryPlaceholder: string;
            addNote: string;
            viewNote: string;
            editNote: string;
            saveSuccess: string;
          };
          exercise: Record<string, string>;
        };
        tool: {
          stockAlert: {
            title: string;
            addRule: string;
            code: string;
            codePlaceholder: string;
            codeInvalid: string;
            name: string;
            namePlaceholder: string;
            ruleTypeLabel: string;
            ruleType: {
              price_above: string;
              price_below: string;
              change_pct_above: string;
              change_pct_below: string;
            };
            threshold: string;
            thresholdRequired: string;
            priceHint: string;
            pctHint: string;
            enabled: string;
            lastTriggeredAt: string;
            actions: string;
            loadFailed: string;
            updateFailed: string;
            deleteFailed: string;
            createFailed: string;
            createSuccess: string;
            emptyTip: string;
            addFirstRule: string;
          };
          backtest: {
            title: string;
            conditions: string;
            addCondition: string;
            conditionsTip: string;
            params: string;
            startYear: string;
            endYear: string;
            holdDays: string;
            maxStocks: string;
            run: string;
            running: string;
            runFailed: string;
            yearInvalid: string;
            emptyTip: string;
            periodCount: string;
            meanReturn: string;
            medianReturn: string;
            winRate: string;
            bestReturn: string;
            worstReturn: string;
            cumulativeRet: string;
            skipped: string;
            periodChart: string;
            periodTable: string;
            rebalanceDate: string;
            sellDate: string;
            stockCount: string;
            return: string;
            between: string;
            field: {
              price: string;
              changePct: string;
              turnoverRate: string;
              amount: string;
              peTtm: string;
              pb: string;
            };
          };
          watchlist: {
            filters: string;
            keywordPlaceholder: string;
            group: string;
            groupPlaceholder: string;
            industry: string;
            industryPlaceholder: string;
            concept: string;
            conceptPlaceholder: string;
            securityType: string;
            securityTypePlaceholder: string;
            market: string;
            marketPlaceholder: string;
            excludeSt: string;
            metricFilters: string;
            add: string;
            valuePlaceholder: string;
            toPlaceholder: string;
            refresh: string;
            reset: string;
            totalPrefix: string;
            totalSuffix: string;
            empty: string;
            defaultGroup: string;
            loadFailed: string;
            deleted: string;
            deleteFailed: string;
            deleteConfirm: string;
            detail: string;
            between: string;
            marketSh: string;
            marketSz: string;
            marketBj: string;
            typeStock: string;
            typeIndex: string;
            typeEtf: string;
            code: string;
            name: string;
            type: string;
            price: string;
            changePct: string;
            peTtm: string;
            pb: string;
            roe: string;
            turnoverRate: string;
            marketCap: string;
            revenueYoy: string;
            field: {
              peTtm: string;
              pb: string;
              roe: string;
              revenueYoy: string;
              netProfitYoy: string;
              changePct: string;
              turnoverRate: string;
              marketCap: string;
            };
          };
          stockScreen: {
            filters: string;
            keywordPlaceholder: string;
            industry: string;
            industryPlaceholder: string;
            concept: string;
            conceptPlaceholder: string;
            securityType: string;
            securityTypePlaceholder: string;
            market: string;
            marketPlaceholder: string;
            excludeSt: string;
            metricFilters: string;
            add: string;
            valuePlaceholder: string;
            toPlaceholder: string;
            start: string;
            saveFilters: string;
            savedConditions: string;
            totalPrefix: string;
            totalSuffix: string;
            syncing: string;
            lastSyncFailed: string;
            lastSyncDone: string;
            noSyncRecord: string;
            statusLoading: string;
            detail: string;
            addWatchlist: string;
            screenFailed: string;
            noConditions: string;
            savedName: string;
            saveSuccess: string;
            saveFailed: string;
            loadFilterFailed: string;
            deleted: string;
            deleteFailed: string;
            syncFailed: string;
            syncDone: string;
            between: string;
            marketSh: string;
            marketSz: string;
            marketBj: string;
            typeStock: string;
            typeIndex: string;
            typeEtf: string;
            code: string;
            name: string;
            type: string;
            price: string;
            changePct: string;
            peTtm: string;
            pb: string;
            roe: string;
            turnoverRate: string;
            marketCap: string;
            revenueYoy: string;
            field: {
              peTtm: string;
              pb: string;
              roe: string;
              revenueYoy: string;
              netProfitYoy: string;
              changePct: string;
              turnoverRate: string;
              marketCap: string;
            };
          };
          macro: {
            title: string;
            syncing: string;
            lastSync: string;
            sync: string;
            depositRate: string;
            loanRate: string;
            lpr: string;
            reserveRatio: string;
            moneySupplyMonth: string;
            moneySupplyYear: string;
            gdp: string;
            cpi: string;
            pmi: string;
            ppi: string;
            loadFailed: string;
            syncFailed: string;
            syncDone: string;
            syncStarted: string;
            syncStartFailed: string;
          };
          calendar: {
            title: string;
            subtitle: string;
            agentTab: string;
            create: string;
            edit: string;
            dayHeader: string;
            count: string;
            empty: string;
            emptyTip: string;
            notified: string;
            repeatNone: string;
            repeatDaily: string;
            repeatWeekly: string;
            repeatMonthly: string;
            repeatYearly: string;
            deleteConfirmTitle: string;
            deleteMemoConfirm: string;
            deleteRepeatConfirm: string;
            deleteThisOnly: string;
            deleteAll: string;
            created: string;
            updated: string;
            titleField: string;
            titlePlaceholder: string;
            titleRequired: string;
            contentField: string;
            contentPlaceholder: string;
            advanceNotify: string;
            minutes: string;
            remindAt: string;
            repeat: string;
            repeatEnd: string;
            repeatEndPlaceholder: string;
            agentTask: {
              desc: string;
              create: string;
              createModal: string;
              editModal: string;
              disabledTag: string;
              nextRun: string;
              email: string;
              runNow: string;
              runNowTip: string;
              deleteConfirm: string;
              empty: string;
              executor: string;
              executorPlaceholder: string;
              taskContent: string;
              taskContentPlaceholder: string;
              scheduleType: string;
              once: string;
              cronRadio: string;
              runAt: string;
              cronPreset: string;
              cronExpr: string;
              cronExprPlaceholder: string;
              resultNotify: string;
              enabled: string;
              cronPresetDaily: string;
              cronPresetWeekday: string;
              cronPresetMonday: string;
              cronPresetMonthly: string;
              cronPresetCustom: string;
              orchestrationLabel: string;
              orchestrationFallback: string;
              deletedAgent: string;
              deletedOrchestration: string;
              agentRequired: string;
              inputRequired: string;
              runAtRequired: string;
              runAtInvalid: string;
              cronRequired: string;
              loadFailed: string;
              saveFailed: string;
              created: string;
              updated: string;
              runFailed: string;
              runTriggered: string;
              operationFailed: string;
              enabledMsg: string;
              disabledMsg: string;
              deleteFailed: string;
              deleted: string;
              scheduleCron: string;
            };
          };
          stockDetail: {
            backToScreen: string;
            syncing: string;
            syncLatest: string;
            addWatchlist: string;
            localData: string;
            realtime: string;
            basicInfo: string;
            financeIndicators: string;
            syncStatusTitle: string;
            concepts: string;
            klineTrend: string;
            hourlyK: string;
            dailyK: string;
            weeklyK: string;
            monthlyK: string;
            chartView: string;
            tableView: string;
            noPeriodData: string;
            syncKlineFirst: string;
            tableLimit: string;
            financeHistoryTitle: string;
            recent16: string;
            all: string;
            groupProfit: string;
            groupGrowth: string;
            groupOperation: string;
            groupSolvency: string;
            codeRequired: string;
            loadFailed: string;
            financeLoadFailed: string;
            klineLoadFailed: string;
            syncFailedWithReason: string;
            syncStartFailed: string;
            syncInProgress: string;
            syncDone: string;
            syncRunningDetected: string;
            date: string;
            status: string;
            suspended: string;
            trading: string;
            open: string;
            high: string;
            low: string;
            preclose: string;
            close: string;
            changePct: string;
            turnoverPct: string;
            volume: string;
            volumeWan: string;
            yiValue: string;
            peTtm: string;
            pb: string;
            psTtm: string;
            pcf: string;
            time: string;
            reportDate: string;
            roePct: string;
            grossMarginPct: string;
            netMarginPct: string;
            revenueWan: string;
            revenueYoyPct: string;
            netProfitWan: string;
            netProfitYoyPct: string;
            eps: string;
            equityYoyPct: string;
            assetYoyPct: string;
            epsYoyPct: string;
            debtRatioPct: string;
            currentRatio: string;
            quickRatio: string;
            cashRatio: string;
            nrTurn: string;
            invTurn: string;
            caTurn: string;
            assetTurn: string;
            cfoToOr: string;
            cfoToNp: string;
            closePrice: string;
            changePctFull: string;
            axisPrice: string;
            unitTimes: string;
            unitPctPerX: string;
            infoCode: string;
            infoName: string;
            infoMarket: string;
            infoIndustry: string;
            infoPrice: string;
            infoChangePct: string;
            tipChangePct: string;
            infoTurnover: string;
            tipTurnover: string;
            infoAmount: string;
            infoMarketCap: string;
            tipMarketCap: string;
            infoFloatMcap: string;
            fiRoe: string;
            tipRoe: string;
            tipPeTtm: string;
            tipPb: string;
            fiRevenueGrowth: string;
            tipRevenueGrowth: string;
            fiNetProfitGrowth: string;
            tipNetProfitGrowth: string;
            fiGrossMargin: string;
            tipGrossMargin: string;
            fiNetMargin: string;
            tipNetMargin: string;
            fiDebtRatio: string;
            tipDebtRatio: string;
            tipCurrentRatio: string;
            tipQuickRatio: string;
            tipCashRatio: string;
            fiNrTurnover: string;
            tipNrTurnover: string;
            fiInvTurnover: string;
            tipInvTurnover: string;
            fiEquityYoy: string;
            tipEquityYoy: string;
            fiAssetYoy: string;
            tipAssetYoy: string;
            tipCfoToOr: string;
            dailyKTo: string;
            weeklyKTo: string;
            monthlyKTo: string;
            hourlyKTo: string;
            klineSyncedAt: string;
            financeTo: string;
            financeSyncedAt: string;
            synced: string;
            syncFailed: string;
            notSynced: string;
            noFinanceData: string;
            klineStatusLabel: string;
            financeStatusLabel: string;
          };
        };
        userPortrait: {
          portrait: string;
          experiences: string;
          noPortrait: string;
          noExperience: string;
          edit: string;
          delete: string;
          extract: string;
          extractSuccess: string;
          extractionOn: string;
          extractionOff: string;
          edited: string;
          updatedAt: string;
          editPortrait: string;
          summary: string;
          summaryPlaceholder: string;
          tags: string;
          tagsPlaceholder: string;
          dimensions: string;
          addDimension: string;
          keyPlaceholder: string;
          valuePlaceholder: string;
          dimensionLabel: Record<string, string>;
          addExperience: string;
          editExperience: string;
          domainLabel: string;
          eventTypeLabel: string;
          importanceLabel: string;
          titleField: string;
          titlePlaceholder: string;
          titleRequired: string;
          content: string;
          timeRange: string;
          timeRangePlaceholder: string;
          occurredAt: string;
          memoryLevelLabel: string;
          evidenceLabel: string;
          statusLabel: string;
          save: string;
          cancel: string;
          confirm: string;
          saveSuccess: string;
          deleteSuccess: string;
          deleteConfirmTitle: string;
          deleteConfirm: string;
          domain: {
            career: string;
            education: string;
            project: string;
            skill: string;
            technology: string;
            finance: string;
            health: string;
            lifestyle: string;
            relationship: string;
            community: string;
            legal: string;
            travel: string;
            hobby: string;
            habit: string;
            personality: string;
            preference: string;
            achievement: string;
            challenge: string;
            other: string;
          };
          eventType: {
            start: string;
            ongoing: string;
            complete: string;
            achieve: string;
            fail: string;
            abandon: string;
            decide: string;
            change: string;
            participate: string;
            publish: string;
            compete: string;
            volunteer: string;
            relocate: string;
            recover: string;
            experiment: string;
            maintain: string;
          };
          status: {
            planned: string;
            ongoing: string;
            completed: string;
            abandoned: string;
            paused: string;
            unknown: string;
          };
          memoryLevel: {
            core: string;
            long_term: string;
            temporary: string;
          };
        };
        userProfile: {
          title: string;
          basicInfo: string;
          editNickname: string;
          changePassword: string;
          themeSettings: string;
          telegramBinding: string;
          userName: string;
          nickname: string;
          email: string;
          emailPlaceholder: string;
          role: string;
          createdAt: string;
          updatedAt: string;
          lastLoginAt: string;
          nicknamePlaceholder: string;
          nicknameRequired: string;
          oldPassword: string;
          oldPasswordPlaceholder: string;
          oldPasswordRequired: string;
          newPassword: string;
          newPasswordPlaceholder: string;
          newPasswordRequired: string;
          passwordMinLength: string;
          confirmPassword: string;
          confirmPasswordPlaceholder: string;
          confirmPasswordRequired: string;
          passwordMismatch: string;
          updateSuccess: string;
          passwordChangeSuccess: string;
          telegramBound: string;
          telegramNotBound: string;
          telegramUsername: string;
          telegramBindCode: string;
          telegramBindCodeHint: string;
          telegramBindCodeExpire: string;
          telegramGenerateCode: string;
          telegramUnbind: string;
          telegramUnbindConfirm: string;
          telegramUnbindSuccess: string;
          telegramGenerateSuccess: string;
          telegramBotNotConfigured: string;
          twoFAQrFailed: string;
          twoFACodeRequired: string;
          twoFAEnableSuccess: string;
          twoFADisableSuccess: string;
        };
        cut: {
          name: string;
          type: string;
          startTime: string;
          endTime: string;
          search: string;
          inputName: string;
          selectType: string;
          inputStartTime: string;
          inputEndTime: string;
          /** 表单与操作 */
          addItem: string;
          addMaterial: string;
          materialType: string;
          itemMaterialTypeRequired: string;
          startCutting: string;
          clearAll: string;
          inputInvalid: string;
          inputMaterialInvalid: string;
          inputItemsRequired: string;
          noResultWarning: string;
          newMaterialLabel: string;
          newMaterialLength: string;
          newMaterialInvalid: string;
          loading: string;
          /** 汇总卡片 */
          summaryTitle: string;
          summaryMaterialCount: string;
          summaryMaterialLength: string;
          summaryCutLength: string;
          summaryUtilization: string;
          summaryRemaining: string;
          summaryScrapCount: string;
          /** 按材料类型分组统计表 */
          summaryByTypeTitle: string;
          summaryTypeUtilization: string;
          summaryNewMaterial: string;
          summaryBinCount: string;
          summaryTotalArea: string;
          summaryUsedArea: string;
          summaryUnplacedCount: string;
          unitBar: string;
          /** 未排入件警示 */
          unplacedAlert: string;
          unplacedLabel: string;
          unplacedReason: string;
          reasonOversized: string;
          reasonExhausted: string;
          /** 旧料库/余料入库 */
          scrapLibrary: string;
          scrapStockInSuccess: string;
          scrapFromCutting: string;
          scrapApplied: string;
          scrapLabelName: string;
          scrapSize: string;
          scrapLength: string;
          scrapWidth: string;
          scrapHeight: string;
          scrapQuantity: string;
          scrapNote: string;
          scrapNotePlaceholder: string;
          scrapCreatedAt: string;
          scrapAdd: string;
          scrapAddSuccess: string;
          scrapDeleteConfirm: string;
          scrapDeleteSuccess: string;
          scrapApply: string;
          scrapApplyNone: string;
          scrapInputInvalid: string;
          scrapMaterialLabel: string;
          /** 导出与打印 */
          exportPng: string;
          exportPdf: string;
          printChart: string;
          exportFailed: string;
          allowPopup: string;
        };
        share: {
          notFound: string;
        };
      };
      form: {
        required: string;
        userName: FormMsg;
        phone: FormMsg;
        pwd: FormMsg;
        confirmPwd: FormMsg;
        code: FormMsg;
        email: FormMsg;
      };
      dropdown: Record<Global.DropdownKey, string>;
      icon: {
        themeConfig: string;
        themeSchema: string;
        lang: string;
        fullscreen: string;
        fullscreenExit: string;
        reload: string;
        collapse: string;
        expand: string;
        pin: string;
        unpin: string;
      };
      datatable: {
        itemCount: string;
        fixed: {
          left: string;
          right: string;
          unFixed: string;
        };
      };
      proxy: {
        desc: string;
        exit: string;
      };
    };

    type GetI18nKey<
      T extends Record<string, unknown>,
      K extends keyof T = keyof T,
    > = K extends string
      ? T[K] extends Record<string, unknown>
        ? `${K}.${GetI18nKey<T[K]>}`
        : K
      : never;

    type I18nKey = GetI18nKey<Schema>;

    type TranslateOptions<Locales extends string> =
      import("vue-i18n").TranslateOptions<Locales>;

    interface $T {
      (key: I18nKey): string;
      (
        key: I18nKey,
        plural: number,
        options?: TranslateOptions<LangType>,
      ): string;
      (
        key: I18nKey,
        defaultMsg: string,
        options?: TranslateOptions<I18nKey>,
      ): string;
      (
        key: I18nKey,
        list: unknown[],
        options?: TranslateOptions<I18nKey>,
      ): string;
      (key: I18nKey, list: unknown[], plural: number): string;
      (key: I18nKey, list: unknown[], defaultMsg: string): string;
      (
        key: I18nKey,
        named: Record<string, unknown>,
        options?: TranslateOptions<LangType>,
      ): string;
      (key: I18nKey, named: Record<string, unknown>, plural: number): string;
      (
        key: I18nKey,
        named: Record<string, unknown>,
        defaultMsg: string,
      ): string;
    }
  }

  /** Service namespace */
  namespace Service {
    /** Other baseURL key */
    type OtherBaseURLKey = "demo";

    interface ServiceConfigItem {
      /** The backend service base url */
      baseURL: string;
      /** The proxy pattern of the backend service base url */
      proxyPattern: string;
    }

    interface OtherServiceConfigItem extends ServiceConfigItem {
      key: OtherBaseURLKey;
    }

    /** The backend service config */
    interface ServiceConfig extends ServiceConfigItem {
      /** Other backend service config */
      other: OtherServiceConfigItem[];
    }

    interface SimpleServiceConfig extends Pick<ServiceConfigItem, "baseURL"> {
      other: Record<OtherBaseURLKey, string>;
    }

    /** The backend service response data */
    type Response<T = unknown> = {
      /** The backend service response code */
      code: string;
      /** The backend service response message */
      msg: string;
      /** The backend service response data */
      data: T;
    };

    /** The demo backend service response data */
    type DemoResponse<T = unknown> = {
      /** The backend service response code */
      status: string;
      /** The backend service response message */
      message: string;
      /** The backend service response data */
      result: T;
    };
  }
}
