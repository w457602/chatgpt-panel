/**
 * Content Script
 * 注入到页面中的脚本,可以访问 DOM 和部分 Chrome API
 */

// 按顺序注入脚本到页面上下文（必须串行加载，保证依赖顺序）
const scripts = [
  'dist/injected.bundle.js' // 打包后的单文件入口
];

let currentIndex = 0;

/**
 * 串行加载脚本（一个接一个）
 */
function loadNextScript() {
  if (currentIndex >= scripts.length) {
    // 所有脚本加载完成
    // 通知 background script 已准备好
    chrome.runtime
      .sendMessage({
        type: 'CONTENT_SCRIPT_READY',
        url: window.location.href
      })
      .catch((err) => {
        console.error('[ATM Extension] Failed to notify background:', err);
      });
    return;
  }

  const src = scripts[currentIndex];
  const script = document.createElement('script');
  script.src = chrome.runtime.getURL(src);

  script.onload = function () {
    this.remove();
    currentIndex++;
    loadNextScript(); // 加载下一个脚本
  };

  script.onerror = function () {
    this.remove();
    currentIndex++;
    loadNextScript(); // 即使失败也继续加载下一个
  };
  (document.head || document.documentElement).appendChild(script);
}

// 开始加载第一个脚本
loadNextScript();

// ==================== 悬浮一键绑卡按钮 ====================

const FLOATING_BTN_ID = 'atm-floating-bind-btn';

function isSupportedHost(hostname) {
  const host = (hostname || '').toLowerCase();
  return (
    host.endsWith('augmentcode.com') ||
    host.includes('cursor.com') ||
    host === 'checkout.stripe.com' ||
    host === 'pay.openai.com' ||
    host === 'chatgpt.com'
  );
}

function ensureFloatingButton() {
  if (window.top !== window) return;
  if (!isSupportedHost(window.location.hostname)) return;
  if (document.getElementById(FLOATING_BTN_ID)) return;

  const btn = document.createElement('button');
  btn.id = FLOATING_BTN_ID;
  btn.textContent = '一键绑卡';
  btn.style.position = 'fixed';
  btn.style.right = '16px';
  btn.style.bottom = '20px';
  btn.style.zIndex = '999999';
  btn.style.padding = '10px 14px';
  btn.style.background = '#2563eb';
  btn.style.color = '#fff';
  btn.style.border = 'none';
  btn.style.borderRadius = '999px';
  btn.style.boxShadow = '0 6px 16px rgba(0,0,0,0.2)';
  btn.style.fontSize = '14px';
  btn.style.fontWeight = '600';
  btn.style.cursor = 'pointer';
  btn.style.userSelect = 'none';

  btn.addEventListener('mouseenter', () => {
    btn.style.background = '#1d4ed8';
  });
  btn.addEventListener('mouseleave', () => {
    btn.style.background = '#2563eb';
  });

  btn.addEventListener('click', () => {
    chrome.runtime
      .sendMessage({ action: 'bindCard' })
      .catch(() => {});
  });

  const attach = () => {
    if (document.body) {
      document.body.appendChild(btn);
      return true;
    }
    return false;
  };

  if (!attach()) {
    const timer = setInterval(() => {
      if (attach()) clearInterval(timer);
    }, 200);
  }
}

function refreshFloatingButton() {
  if (window.top !== window) return;
  const supported = isSupportedHost(window.location.hostname);
  const existing = document.getElementById(FLOATING_BTN_ID);
  if (supported) {
    if (!existing) ensureFloatingButton();
  } else if (existing) {
    existing.remove();
  }
}

ensureFloatingButton();
setInterval(refreshFloatingButton, 1000);

// 监听来自 injected.js 的消息
window.addEventListener('message', (event) => {
  // 只接受来自同一窗口的消息
  if (event.source !== window) return;

  const message = event.data;

  // 检查消息类型
  if (message.type && message.type.startsWith('ATM_')) {
    // 转发到 background script
    chrome.runtime.sendMessage(message).catch((err) => {
      console.error(
        '[ATM Extension] Failed to send message to background:',
        err
      );
    });
  }
});

// 监听来自 background script 的消息
chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (message.type === 'FILL_EMAIL') {
    try {
      fillEmailInPage(message.email || '');
    } catch (error) {}
  }

  if (message.type === 'AUTO_IMPORT_PAGE_DETECTED') {
    // 通知页面
    window.postMessage(
      {
        type: 'ATM_AUTO_IMPORT_PAGE_DETECTED',
        url: message.url
      },
      '*'
    );
  }

  // 处理填充卡号数据的消息
  if (message.type === 'FILL_CARD_DATA') {
    // 转发给 injected.js（适配器管理器）
    window.postMessage(
      {
        type: 'ATM_FILL_CARD_DATA',
        cardData: message.cardData
      },
      '*'
    );
  }

  if (message.type === 'SHOW_STATUS_TOAST') {
    window.postMessage(
      {
        type: 'ATM_SHOW_STATUS_TOAST',
        state: message.state,
        icon: message.icon,
        text: message.text
      },
      '*'
    );
  }

  // 处理触发自动填充的消息（适配器自己生成数据）
  if (message.type === 'TRIGGER_AUTO_FILL') {
    const config = message.config || {};
    const mergedConfig = {
      bin: config.bin || '',
      patternInput: config.patternInput || '',
      addressRegion: config.addressRegion || 'US_TAX_FREE',
      maxRetries: config.maxRetries,
      binAddress: config.binAddress || '',
      onlyFill: !!config.onlyFill,
      onlyChangeCardNumber: !!config.onlyChangeCardNumber
    };

    // 转发给 injected.js（适配器管理器），包含配置数据
    window.postMessage(
      {
        type: 'ATM_TRIGGER_AUTO_FILL',
        config: mergedConfig
      },
      '*'
    );
  }

  sendResponse({ success: true });
  return true;
});

// ==================== Email 填充 ====================

function querySelectorDeep(selector) {
  const roots = [document];
  const visited = new Set();
  while (roots.length > 0) {
    const root = roots.shift();
    if (!root || visited.has(root)) continue;
    visited.add(root);
    try {
      const found = root.querySelector(selector);
      if (found) return found;
    } catch (error) {}

    let elements = [];
    try {
      elements = Array.from(root.querySelectorAll('*'));
    } catch (error) {
      elements = [];
    }
    for (const el of elements) {
      if (el && el.shadowRoot) {
        roots.push(el.shadowRoot);
      }
    }
  }
  return null;
}

function setInputValue(el, value) {
  if (!el) return false;
  try {
    el.focus();
    el.click();
    const setter = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      'value'
    )?.set;
    if (setter) {
      setter.call(el, String(value));
    } else {
      el.value = String(value);
    }
    el.dispatchEvent(new Event('input', { bubbles: true, cancelable: true }));
    el.dispatchEvent(new Event('change', { bubbles: true, cancelable: true }));
    el.dispatchEvent(new Event('blur', { bubbles: true }));
    return true;
  } catch (error) {
    return false;
  }
}

function findEmailInput() {
  const selectors = [
    'input[type="email"]',
    'input[autocomplete="email"]',
    'input[autocomplete="username"]',
    'input[name="email"]',
    'input[id="email"]',
    'input[name*="email" i]',
    'input[id*="email" i]',
    'input[aria-label*="email" i]',
    'input[data-testid*="email" i]'
  ];

  for (const selector of selectors) {
    const el = querySelectorDeep(selector);
    if (el) return el;
  }
  return null;
}

function fillEmailInPage(email, attempt = 0) {
  const value = String(email || '').trim();
  if (!value) return;
  const input = findEmailInput();
  if (!input) {
    if (attempt < 5) {
      setTimeout(() => fillEmailInPage(value, attempt + 1), 300);
    }
    return;
  }
  if (input.value && input.value.trim() === value) return;
  if (input.value && input.value.trim() !== '' && input.value.trim() !== value) {
    return;
  }
  setInputValue(input, value);
}
