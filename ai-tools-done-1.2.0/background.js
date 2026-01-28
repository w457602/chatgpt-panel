/**
 * ATM Auto Register Extension - Background Script
 * 处理自动注册逻辑
 */

// ==================== 全局变量 ====================

// 记录哪些 tab 的 content script 已经准备好
const contentScriptReady = new Map(); // tabId -> boolean

// Panel API 配置
const PANEL_API_BASE_DEFAULT = 'https://chatgptpanel.zeabur.app';
const PANEL_API_BASE_KEY = 'panelApiBase';
const PANEL_API_TOKEN_KEY = 'panelApiToken';

// 监听 tab 关闭事件，清理状态
chrome.tabs.onRemoved.addListener((tabId) => {
  contentScriptReady.delete(tabId);
});

// ==================== 快捷键监听 ====================

chrome.commands.onCommand.addListener(async (command) => {
  try {
    if (command === 'bind-card') {
      await handleBindCard(null, { onlyFill: false });
    } else if (command === 'fill-only') {
      await handleBindCard(null, { onlyFill: true });
    }
  } catch (error) {}
});

// ==================== 页面类型检测 ====================

/**
 * 检测页面类型（粗粒度 - 仅识别服务商）
 * 细粒度的页面类型判断由各适配器负责
 */
function detectPageType(url, hostname) {
  const urlLower = url.toLowerCase();
  const hostnameLower = hostname.toLowerCase();

  // Augment 服务
  if (hostnameLower === 'auth.augmentcode.com') {
    return 'unknown';
  }

  if (hostnameLower.includes('augmentcode.com')) {
    return 'augment';
  }

  // Stripe 通用支付页面（支持 Cursor、Windsurf 等）
  if (hostnameLower === 'checkout.stripe.com') {
    return 'stripe';
  }

  // OpenAI 支付页面
  if (hostnameLower === 'pay.openai.com') {
    return 'stripe';
  }

  // ChatGPT 支付页面
  if (hostnameLower === 'chatgpt.com' && urlLower.includes('/checkout/openai_llc/')) {
    return 'chatgpt';
  }

  // Cursor 其他页面（用于跳转到绑卡页面）
  if (hostnameLower.includes('cursor.com')) {
    return 'cursor';
  }

  return 'unknown';
}

// ==================== Panel API ====================

function normalizePanelBase(base) {
  if (!base || typeof base !== 'string') return PANEL_API_BASE_DEFAULT;
  return base.replace(/\/+$/, '') || PANEL_API_BASE_DEFAULT;
}

async function getPanelConfig() {
  const result = await chrome.storage.local.get([PANEL_API_BASE_KEY, PANEL_API_TOKEN_KEY]);
  return {
    base: normalizePanelBase(result.panelApiBase || PANEL_API_BASE_DEFAULT),
    token: (result.panelApiToken || '').trim()
  };
}

async function lookupPanelAccountByURL(url) {
  if (!url) return null;
  try {
    const { base, token } = await getPanelConfig();
    const target = `${base}/api/v1/extension/account?url=${encodeURIComponent(url)}`;
    const headers = { Accept: 'application/json' };
    if (token) headers['X-Extension-Token'] = token;
    const resp = await fetch(target, { method: 'GET', headers });
    if (!resp.ok) return null;
    const data = await resp.json().catch(() => null);
    return data;
  } catch (error) {
    return null;
  }
}

async function notifyPanelBillingSuccess(url, accountId) {
  if (!url && !accountId) return false;
  try {
    const { base, token } = await getPanelConfig();
    const payload = {};
    if (url) payload.url = url;
    if (accountId) payload.account_id = accountId;
    const headers = { 'Content-Type': 'application/json', Accept: 'application/json' };
    if (token) headers['X-Extension-Token'] = token;
    const resp = await fetch(`${base}/api/v1/extension/billing-success`, {
      method: 'POST',
      headers,
      body: JSON.stringify(payload)
    });
    return resp.ok;
  } catch (error) {
    return false;
  }
}

async function sendMessageToAllFrames(tabId, message) {
  const sendToFrame = async (frameId) => {
    try {
      await chrome.tabs.sendMessage(tabId, message, { frameId });
    } catch (error) {}
  };

  try {
    const frames = await new Promise((resolve) => {
      chrome.webNavigation.getAllFrames({ tabId }, (results) => {
        resolve(Array.isArray(results) ? results : []);
      });
    });
    if (frames.length === 0) {
      await sendToFrame(0);
      return;
    }
    for (const frame of frames) {
      await sendToFrame(frame.frameId);
    }
  } catch (error) {
    await sendToFrame(0);
  }
}

// ==================== 扩展安装/更新监听 ====================

/**
 * 监听扩展安装或更新
 */
chrome.runtime.onInstalled.addListener(async (details) => {
  if (details.reason === 'install') {
    // 首次安装时设置默认配置
    await chrome.storage.local.set({
      autoOpenSetting: 'detect',
      autoRegisterEnabled: false,
      selectedBin: '',
      autoFillDelay: 1500,
      hasOpenedPanel: false,
      panelApiBase: PANEL_API_BASE_DEFAULT,
      panelApiToken: ''
    });
  }
});

// ==================== 标签页更新监听（自动打开 + 自动填充） ====================

/**
 * 监听标签页更新，实现自动打开控制面板和自动填充
 */
chrome.tabs.onUpdated.addListener(async (tabId, changeInfo, tab) => {
  // 只在页面加载完成时处理
  if (changeInfo.status !== 'complete') {
    return;
  }

  const url = tab.url || '';
  if (!url) {
    return;
  }

  try {
    const hostname = new URL(url).hostname.toLowerCase();
    const pageType = detectPageType(url, hostname);

    // 在需要自动处理的页面时触发（粗粒度 - 只识别服务商）
    const autoPages = ['augment', 'stripe', 'chatgpt'];
    if (autoPages.includes(pageType)) {
      // 读取设置
      const result = await chrome.storage.local.get([
        'autoOpenSetting',
        'autoRegisterEnabled',
        'autoFillDelay'
      ]);
      const autoRegisterEnabled = result.autoRegisterEnabled || false;
      const autoFillDelay = result.autoFillDelay || 1500;

      // 如果开启了自动注册，则自动填充
      if (autoRegisterEnabled) {
        await handleAutoFill(tabId, pageType, autoFillDelay, url);
      }
    }
  } catch (error) {}
});

// ==================== 消息监听 ====================

// 监听来自 content script 和 popup 的消息
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  // 检查消息对象是否有效
  if (!message || typeof message !== 'object') {
    sendResponse({ success: false, error: 'Invalid message' });
    return false;
  }

  // 处理 type 属性的消息
  if (message.type && typeof message.type === 'string') {
    if (message.type === 'AUTO_REGISTER_STATUS') {
      // 可以在这里处理状态更新
    }

    if (message.type === 'SESSION_COOKIE_EXTRACTED') {
      // 可以在这里处理 session cookie
    }

    if (message.type === 'ATM_LOG_ENTRY') {
      handleLogEntry(message).catch((error) => {
        console.error('[ATM] handleLogEntry failed', error);
      });
    }

    if (message.type === 'ATM_BILLING_SUCCESS') {
      // 异步处理，避免阻塞消息响应
      handleBillingSuccess(message).catch((error) => {
        console.error('[ATM] handleBillingSuccess failed', error);
      });
      notifyPanelBillingSuccess(message.url || '', message.accountId || '').catch((error) => {
        console.error('[ATM] notifyPanelBillingSuccess failed', error);
      });
    }

    if (message.type === 'POPUP_STATUS_TOAST') {
      const state = message.state || 'info';
      const icon = message.icon || '';
      const text = message.text || '';

      const sendToTab = (tabId) => {
        if (!tabId) {
          sendResponse({ success: false, error: 'No active tab' });
          return;
        }

        chrome.tabs
          .sendMessage(tabId, {
            type: 'SHOW_STATUS_TOAST',
            state,
            icon,
            text
          })
          .then(() => {
            sendResponse({ success: true });
          })
          .catch((err) => {
            sendResponse({
              success: false,
              error: err && err.message ? err.message : 'sendMessage failed'
            });
          });
      };

      if (sender && sender.tab && sender.tab.id) {
        sendToTab(sender.tab.id);
      } else {
        chrome.tabs
          .query({ active: true, currentWindow: true })
          .then((tabs) => {
            const tab = tabs && tabs[0];
            sendToTab(tab && tab.id);
          })
          .catch((err) => {
            console.error('[ATM] query active tab failed', err);
            sendResponse({
              success: false,
              error: err && err.message ? err.message : 'query failed'
            });
          });
      }

      return true;
    }

    sendResponse({ success: true });
    return false;
  }

  // 处理 content script 准备就绪通知
  if (message.type === 'CONTENT_SCRIPT_READY') {
    const tabId = sender.tab?.id;
    if (tabId) {
      contentScriptReady.set(tabId, true);
    }
    sendResponse({ success: true });
    return false;
  }

  // 处理 action 属性的消息
  if (message.action === 'bindCard') {
    // 使用 Promise 处理异步操作
    handleBindCard(sender, message)
      .then((result) => {
        sendResponse(result);
      })
      .catch((error) => {
        sendResponse({ success: false, message: error.message });
      });
    return true; // 保持消息通道开启，等待异步操作完成
  }

  // 处理获取 cookies 的请求
  if (message.action === 'getCookies') {
    const domain = message.domain;
    if (!domain) {
      sendResponse({ success: false, error: 'Domain is required' });
      return false;
    }

    chrome.cookies.getAll({ domain }, (cookies) => {
      if (chrome.runtime.lastError) {
        sendResponse({
          success: false,
          error: chrome.runtime.lastError.message
        });
      } else {
        sendResponse({ success: true, cookies });
      }
    });
    return true; // 异步响应
  }

  // 处理获取特定 cookie 的请求
  if (message.action === 'getCookie') {
    const { name, domain, url } = message;
    if (!name) {
      sendResponse({ success: false, error: 'Cookie name is required' });
      return false;
    }

    // chrome.cookies.get 只接受 url 和 name 参数
    // 如果提供了 domain，需要构造完整的 URL
    let cookieUrl = url;
    if (!cookieUrl && domain) {
      // 从 domain 构造 URL（假设使用 https）
      cookieUrl = `https://${domain.replace(/^\/\./, '')}`;
    }

    if (!cookieUrl) {
      sendResponse({
        success: false,
        error: 'Either url or domain is required'
      });
      return false;
    }

    const details = { url: cookieUrl, name };

    chrome.cookies.get(details, (cookie) => {
      if (chrome.runtime.lastError) {
        sendResponse({
          success: false,
          error: chrome.runtime.lastError.message
        });
      } else {
        if (cookie && cookie.value && name === 'session') {
          let maskedValue = cookie.value;
          if (maskedValue.length > 24) {
            maskedValue =
              maskedValue.slice(0, 10) + '...' + maskedValue.slice(-10);
          }
          handleLogEntry({
            level: 'info',
            app: 'Augment',
            scope: 'session',
            message: `getCookie 返回 ${name}: domain=${
              cookie.domain || ''
            }, path=${cookie.path || ''}, value=${maskedValue}`
          }).catch((error) => {});
        }
        sendResponse({ success: true, cookie });
      }
    });
    return true; // 异步响应
  }

  sendResponse({ success: true });
  return false;
});

// 监听页面导航
chrome.webNavigation.onCompleted.addListener((details) => {
  // 检测是否到达 auto_import 页面
  if (
    details.url.includes('auth.augmentcode.com') &&
    details.url.includes('auto_import=true')
  ) {
    // 通知 content script
    chrome.tabs
      .sendMessage(details.tabId, {
        type: 'AUTO_IMPORT_PAGE_DETECTED',
        url: details.url
      })
      .catch((err) => {});
  }
});

// ==================== 一键绑卡处理 ====================

async function handleBindCard(sender, message) {
  try {
    // 获取当前活动标签页
    const [tab] = await chrome.tabs.query({
      active: true,
      currentWindow: true
    });

    if (!tab || !tab.id) {
      return { success: false, message: '无法获取当前标签页' };
    }

    const url = tab.url || '';
    const hostname = new URL(url).hostname.toLowerCase();

    const onlyFill = !!(message && message.onlyFill);

    // 检测页面类型（粗粒度）
    const pageType = detectPageType(url, hostname);

    if (pageType === 'augment' || pageType === 'stripe' || pageType === 'chatgpt') {
      // 统一使用 triggerAdapterAutoFill（适配器自己判断细粒度页面类型并处理）
      await triggerAdapterAutoFill(tab.id, pageType, url, { onlyFill });
      return { success: true, message: '一键绑卡已触发' };
    } else if (pageType === 'cursor') {
      // Cursor 其他页面，需要先获取绑卡URL
      await navigateToCursorBindPage(tab.id, onlyFill);
      return { success: true, message: '正在跳转到 Cursor 绑卡页面...' };
    } else {
      return { success: false, message: '当前页面不支持一键绑卡' };
    }
  } catch (error) {
    return { success: false, message: error.message };
  }
}

async function handleLogEntry(message) {
  try {
    const result = await chrome.storage.local.get(['autoRegisterLogs']);
    const logs = Array.isArray(result.autoRegisterLogs)
      ? result.autoRegisterLogs
      : [];

    const entry = {
      id: Date.now(),
      timestamp: message.timestamp || new Date().toISOString(),
      level: message.level || 'info',
      app: message.app || 'Unknown',
      message: message.message || ''
    };

    logs.push(entry);

    const MAX_LOGS = 300;
    const trimmed = logs.slice(-MAX_LOGS);

    await chrome.storage.local.set({ autoRegisterLogs: trimmed });

    chrome.runtime
      .sendMessage({
        type: 'AUTO_REGISTER_LOG_APPENDED',
        entry: entry
      })
      .catch(() => {});
  } catch (error) {
    console.error('[ATM] handleBillingSuccess outer error', error);
  }
}

// ==================== 绑卡成功后处理（后台导入 Session） ====================

async function handleBillingSuccess(message) {
  try {
    const appName = message.appName || '';
    const url = message.url || '';

    // 目前仅对 Augment 应用执行自动导入 Session
    if (appName !== 'Augment') {
      return;
    }

    // 检查自动注册开关，只有开启时才自动导入 ATM
    const result = await chrome.storage.local.get(['autoRegisterEnabled']);
    const autoRegisterEnabled =
      result.autoRegisterEnabled !== undefined
        ? result.autoRegisterEnabled
        : false;

    if (!autoRegisterEnabled) {
      try {
        await handleLogEntry({
          level: 'info',
          app: appName || 'Augment',
          scope: 'billing',
          message: '检测到 Augment 绑卡成功（自动注册关闭，跳过自动导入ATM）'
        });
      } catch (e) {}
      return;
    }

    try {
      await handleLogEntry({
        level: 'info',
        app: appName || 'Augment',
        scope: 'billing',
        message: '检测到 Augment 绑卡成功，正在导入 Session...'
      });
    } catch (e) {}

    // 获取 auth.augmentcode.com 的 session cookie
    const sessionCookie = await new Promise((resolve) => {
      chrome.cookies.get(
        { url: 'https://auth.augmentcode.com', name: 'session' },
        (cookie) => {
          if (chrome.runtime.lastError) {
            resolve(null);
          } else {
            resolve(cookie);
          }
        }
      );
    });

    if (!sessionCookie || !sessionCookie.value) {
      try {
        await handleLogEntry({
          level: 'warn',
          app: appName || 'Augment',
          scope: 'billing',
          message: '未找到 Session Cookie，无法导入 ATM'
        });
      } catch (e) {}
      return;
    }

    const sessionValue = sessionCookie.value;

    let importResponse;
    try {
      importResponse = await fetch('http://127.0.0.1:8766/api/import/session', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ session: sessionValue })
      });
    } catch (error) {
      try {
        await handleLogEntry({
          level: 'error',
          app: appName || 'Augment',
          scope: 'billing',
          message: `API 调用失败: ${error.message}`
        });
      } catch (e) {}
      return;
    }

    let data = null;
    try {
      data = await importResponse.json();
    } catch (e) {}

    if (importResponse.ok) {
      try {
        await handleLogEntry({
          level: 'info',
          app: appName || 'Augment',
          scope: 'billing',
          message: 'Session 导入成功！'
        });
      } catch (e) {}
    } else {
      // 特殊处理邮箱重复错误（409 CONFLICT）
      if (
        importResponse.status === 409 &&
        data &&
        data.code === 'DUPLICATE_EMAIL'
      ) {
        try {
          await handleLogEntry({
            level: 'warn',
            app: appName || 'Augment',
            scope: 'billing',
            message: '该邮箱已存在于 ATM 中'
          });
        } catch (e) {}
      } else {
        const errorMsg = (data && (data.error || data.message)) || '未知错误';
        try {
          await handleLogEntry({
            level: 'error',
            app: appName || 'Augment',
            scope: 'billing',
            message: `Session 导入失败: ${errorMsg}`
          });
        } catch (e) {}
      }
    }
  } catch (error) {}
}

// ==================== Cursor 页面跳转到绑卡页 ====================

async function navigateToCursorBindPage(tabId, onlyFill = false) {
  try {
    // 在页面上下文中执行获取绑卡URL的操作
    const results = await chrome.scripting.executeScript({
      target: { tabId: tabId },
      func: async () => {
        // 调用 utils/cursor.js 中的 getCheckoutUrl 函数
        if (typeof getCheckoutUrl === 'function') {
          return await getCheckoutUrl('pro');
        } else {
          throw new Error('getCheckoutUrl 函数未定义');
        }
      }
    });

    if (!results || !results[0] || !results[0].result) {
      throw new Error('无法获取绑卡URL');
    }

    const checkoutResult = results[0].result;

    if (checkoutResult.success && checkoutResult.url) {
      // 跳转到绑卡页面
      await chrome.tabs.update(tabId, { url: checkoutResult.url });

      // 等待页面加载完成后自动填充
      chrome.tabs.onUpdated.addListener(function listener(
        updatedTabId,
        changeInfo,
        tab
      ) {
        if (updatedTabId === tabId && changeInfo.status === 'complete') {
          // 延迟填充，确保页面完全加载
          setTimeout(async () => {
            try {
              await triggerAdapterAutoFill(tabId, 'stripe', tab.url, { onlyFill });
            } catch (error) {}
          }, 1000);

          // 移除监听器
          chrome.tabs.onUpdated.removeListener(listener);
        }
      });
    } else {
      throw new Error(checkoutResult.error || '获取绑卡URL失败');
    }
  } catch (error) {
    throw error;
  }
}

// ==================== 自动填充逻辑 ====================

/**
 * 等待 content script 准备好（优化版：使用 Promise + 事件）
 */
async function waitForContentScript(tabId, maxWait = 5000) {
  // 如果已经准备好，直接返回
  if (contentScriptReady.get(tabId)) {
    return true;
  }

  // 使用 Promise + 超时
  return new Promise((resolve) => {
    const timeout = setTimeout(() => {
      resolve(false);
    }, maxWait);

    // 轮询检查（每 100ms）
    const checkInterval = setInterval(() => {
      if (contentScriptReady.get(tabId)) {
        clearInterval(checkInterval);
        clearTimeout(timeout);
        resolve(true);
      }
    }, 100);
  });
}

/**
 * 触发适配器自动填充（公共逻辑）
 * 优化版：粗粒度架构，细节由适配器处理
 * @param {number} tabId - 标签页 ID
 * @param {string} pageType - 页面类型（粗粒度：augment, cursor_stripe）
 * @param {string} url - 页面 URL（传给适配器做细粒度判断）
 */
async function triggerAdapterAutoFill(tabId, pageType, url, options = {}) {
  // 尝试从面板查找邮箱并填充
  try {
    if (pageType === 'stripe' || pageType === 'chatgpt') {
      const data = await lookupPanelAccountByURL(url);
      if (data && data.email) {
        await sendMessageToAllFrames(tabId, {
          type: 'FILL_EMAIL',
          email: data.email
        });
      }
    }
  } catch (error) {}

  // 读取用户配置
  const result = await chrome.storage.local.get([
    'selectedBin',
    'patternInput',
    'maxRetryAttempts',
    'autoRetryEnabled',
    'addressRegion',
    'bins',
    'onlyChangeCardNumber'
  ]);
  const selectedBin = result.selectedBin || '';
  const patternInput = result.patternInput || '';
  const autoRetryEnabled = result.autoRetryEnabled || false;
  const maxRetries = autoRetryEnabled
    ? Number(result.maxRetryAttempts) || 0
    : 0;
  const addressRegion = result.addressRegion || 'US_TAX_FREE';
  const onlyChangeCardNumber = !!result.onlyChangeCardNumber;

  const onlyFill = !!options.onlyFill;

  // 查找选中 BIN 的配置（通过 id 查找，兼容旧数据用 value）
  let bin = selectedBin;
  let binAddress = '';
  if (selectedBin && Array.isArray(result.bins)) {
    const binObj = result.bins.find(
      (b) => b.id === selectedBin || b.value === selectedBin
    );
    if (binObj) {
      bin = binObj.value; // 使用实际的 BIN 码
      if (binObj.address) {
        binAddress = binObj.address;
      }
    }
  }

  // Augment 服务：由适配器处理所有子页面
  // Stripe 服务：由适配器处理所有 Stripe 支付页面
  if (pageType === 'augment' || pageType === 'stripe' || pageType === 'chatgpt') {
    // 优先直接尝试发送消息；避免依赖 service worker 内存态
    const trySend = async () => {
      try {
        await chrome.tabs.sendMessage(tabId, {
          type: 'TRIGGER_AUTO_FILL',
          config: {
            bin: bin,
            patternInput: patternInput,
            maxRetries: maxRetries,
            addressRegion: addressRegion,
            binAddress: binAddress,
            onlyFill: onlyFill,
            onlyChangeCardNumber: onlyChangeCardNumber
          }
        });
        return true;
      } catch (error) {
        return false;
      }
    };

    let sent = await trySend();
    if (!sent) {
      // 退而求其次：等待最多 5s 观察 content script 就绪，再重试
      await waitForContentScript(tabId, 5000);
      // 再试最多 5 次，每次间隔 300ms
      for (let i = 0; i < 5 && !sent; i++) {
        await new Promise((r) => setTimeout(r, 300));
        sent = await trySend();
      }

      if (!sent) {
        try {
          await handleLogEntry({
            level: 'error',
            app: 'AutoFill',
            message: 'Content script 未准备好或通信失败'
          });
        } catch (e) {}
      }
    }
  }
}

/**
 * 自动填充处理（用于自动注册流程）
 * 特点：有防重复、有延迟
 */
async function handleAutoFill(tabId, pageType, delay, url) {
  try {
    // 根据页面类型生成状态文字（粗粒度）
    let statusText = '检测到页面';
    if (pageType === 'augment') {
      statusText = '检测到 Augment 页面';
    } else if (pageType === 'stripe') {
      statusText = '检测到 Stripe 支付页面';
    } else if (pageType === 'chatgpt') {
      statusText = '检测到 ChatGPT 支付页面';
    }

    try {
      await handleLogEntry({
        level: 'info',
        app: 'AutoFill',
        message: statusText
      });
    } catch (e) {}

    // 延迟执行，确保页面完全加载
    await new Promise((resolve) => setTimeout(resolve, delay));

    // 调用公共逻辑（传入 URL 供适配器做细粒度判断）
    await triggerAdapterAutoFill(tabId, pageType, url);
  } catch (error) {
    try {
      await handleLogEntry({
        level: 'error',
        app: 'AutoFill',
        message: '填充失败: ' + error.message
      });
    } catch (e) {}
  }
}
