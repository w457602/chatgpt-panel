#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
ChatGPT OAuth 协议登录脚本
基于协议注册机改造，支持OAuth登录并获取refresh_token
"""

import hashlib
import json
import os
import random
import secrets
import threading
import time
import re
from concurrent.futures import ThreadPoolExecutor, as_completed
from datetime import datetime
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Dict, List, Optional, Tuple
from urllib.parse import urlencode, urlparse, parse_qs, quote as url_quote
import subprocess
try:
    import pybase64
except ModuleNotFoundError:
    print("❌ 缺少依赖 pybase64，请先运行: bash tools/oauth_login.sh")
    raise
import jwt

from curl_cffi import requests
import requests as std_requests  # 用于导入API调用

# ============================================================================
# 配置
# ============================================================================
class Config:
    """配置类"""
    # 代理
    PROXY = "http://127.0.0.1:7890"

    # OAuth配置
    AUTH_URL = "https://auth.openai.com/oauth/authorize"
    TOKEN_URL = "https://auth.openai.com/oauth/token"
    CLIENT_ID = "app_EMoamEEZ73f0CkXaXp7hrann"
    REDIRECT_URI = "http://localhost:1455/auth/callback"
    SCOPE = "openid email profile offline_access"
    CALLBACK_PORT = 1455

    # Auth基础URL
    AUTH_BASE = "https://auth.openai.com"
    CHATGPT_BASE = "https://chatgpt.com"
    SENTINEL_BASE = "https://sentinel.openai.com/backend-api/sentinel"

    # 请求超时
    TIMEOUT = 30

    # 浏览器指纹
    IMPERSONATE = "chrome120"
    USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
    SEC_CH_UA = '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"'

    # 线上项目导入配置
    PANEL_API_BASE = "https://openai.netpulsex.icu"
    PANEL_IMPORT_ENDPOINT = "/api/v1/accounts/import"
    PANEL_USERNAME = "admin"
    PANEL_PASSWORD = "admin123"
    USE_BASH_LAUNCHER = True
    BASH_LAUNCHER_PATH = "tools/oauth_login.sh"

    # 邮箱API (用于自动获取验证码)
    MAIL_API_BASE = "https://mail.chatgpt.org.uk/api"
    OTP_MAX_ATTEMPTS = 60
    OTP_INTERVAL = 3
    DEBUG_CONSENT = True
    DEBUG_CONSENT_DIR = "debug"

    # Bark 通知配置
    BARK_ENABLED = True
    BARK_URL = "https://api.day.app/sJdCVyNSgBrkoXrrFA3pTD"
    BARK_TITLE = "OAuth双RT刷新"

    # ClashX Meta API 配置
    CLASH_API_BASE = "http://127.0.0.1:9090"
    CLASH_PROXY_GROUP = "GLOBAL"  # 策略组名称
    # 只保留美国节点
    CLASH_INCLUDE_KEYWORDS = ["美国", "🇺🇸"]  # 只包含美国节点
    CLASH_EXCLUDE_KEYWORDS = [
        "剩余流量", "距离下次重置", "套餐到期", "建议",  # 排除信息节点
        "DIRECT", "REJECT"  # 排除系统节点
    ]
    CLASH_SWITCH_INTERVAL = 5  # 每处理多少账号切换一次节点

    # 多线程配置
    DEFAULT_WORKERS = 3  # 默认并发线程数


# ============================================================================
# ClashX Meta 节点切换器
# ============================================================================
class ClashProxySwitcher:
    """ClashX Meta 节点自动切换器（只保留美国节点）- 线程安全版"""

    def __init__(self, group_name: str = None, include_keywords: List[str] = None,
                 exclude_keywords: List[str] = None, switch_interval: int = None):
        self.api_base = Config.CLASH_API_BASE
        self.group_name = group_name or Config.CLASH_PROXY_GROUP
        self.include_keywords = include_keywords or Config.CLASH_INCLUDE_KEYWORDS
        self.exclude_keywords = exclude_keywords or Config.CLASH_EXCLUDE_KEYWORDS
        self.switch_interval = switch_interval or Config.CLASH_SWITCH_INTERVAL
        self.available_nodes: List[str] = []
        self.current_index: int = 0
        self.enabled: bool = False
        self._lock = threading.Lock()  # 线程锁
        self._processed_count: int = 0  # 已处理账号计数（多线程共享）
        self._load_nodes()

    def _load_nodes(self):
        """加载可用节点列表（只保留美国节点）"""
        try:
            resp = std_requests.get(f"{self.api_base}/proxies/{url_quote(self.group_name)}", timeout=5)
            if resp.status_code != 200:
                print(f"⚠️ ClashX API 连接失败: {resp.status_code}")
                return

            data = resp.json()
            all_nodes = data.get("all", [])
            current = data.get("now", "")

            # 筛选可用节点（只保留美国节点）
            self.available_nodes = []
            for node in all_nodes:
                # 必须包含 "丨" 才是有效代理节点
                if "丨" not in node:
                    continue
                # 必须包含美国关键词
                if not any(kw in node for kw in self.include_keywords):
                    continue
                # 跳过排除关键词中的节点
                if any(kw in node for kw in self.exclude_keywords):
                    continue
                self.available_nodes.append(node)

            if self.available_nodes:
                self.enabled = True
                # 找到当前节点的位置
                if current in self.available_nodes:
                    self.current_index = self.available_nodes.index(current)
                print(f"✅ ClashX 节点切换器已启用")
                print(f"   - 可用美国节点: {len(self.available_nodes)} 个")
                print(f"   - 当前节点: {current}")
                print(f"   - 切换频率: 每 {self.switch_interval} 个账号")
            else:
                print("⚠️ 未找到可用的美国节点")

        except Exception as e:
            print(f"⚠️ ClashX API 初始化失败: {e}")
            self.enabled = False

    def switch_next(self) -> bool:
        """切换到下一个节点（线程安全）"""
        if not self.enabled or not self.available_nodes:
            return False

        with self._lock:
            # 移动到下一个节点
            self.current_index = (self.current_index + 1) % len(self.available_nodes)
            next_node = self.available_nodes[self.current_index]

        try:
            resp = std_requests.put(
                f"{self.api_base}/proxies/{url_quote(self.group_name)}",
                headers={"Content-Type": "application/json"},
                json={"name": next_node},
                timeout=5
            )
            if resp.status_code == 204:
                print(f"\n🔄 节点切换成功: {next_node}")
                return True
            else:
                print(f"\n⚠️ 节点切换失败: {resp.status_code}")
                return False
        except Exception as e:
            print(f"\n⚠️ 节点切换异常: {e}")
            return False

    def check_and_switch(self) -> bool:
        """检查并切换节点（线程安全，多线程共享计数）

        Returns:
            bool: 是否执行了切换
        """
        if not self.enabled:
            return False

        with self._lock:
            self._processed_count += 1
            # 每 N 个账号切换一次
            if self._processed_count % self.switch_interval == 0:
                should_switch = True
            else:
                should_switch = False

        if should_switch:
            self.switch_next()
            time.sleep(2)  # 切换节点后等待 2 秒
            return True
        return False

    def should_switch(self, account_index: int) -> bool:
        """判断是否应该切换节点（每 N 个账号切换一次）"""
        if not self.enabled:
            return False
        # 在处理第 6, 11, 16, ... 个账号前切换
        return account_index > 1 and (account_index - 1) % self.switch_interval == 0

    def get_current_node(self) -> str:
        """获取当前节点名称"""
        if self.available_nodes and 0 <= self.current_index < len(self.available_nodes):
            return self.available_nodes[self.current_index]
        return "未知"


# ============================================================================
# Bark 通知
# ============================================================================
def send_bark_message(text: str, title: str = None) -> bool:
    """发送 Bark 通知消息"""
    if not Config.BARK_ENABLED:
        return False
    if not Config.BARK_URL:
        print("⚠️ Bark 未配置，跳过通知")
        return False
    try:
        url = Config.BARK_URL.rstrip("/")
        resp = std_requests.get(
            url,
            params={"title": title or Config.BARK_TITLE, "body": text},
            timeout=10,
        )
        if resp.status_code == 200:
            print("📨 Bark 通知已发送")
            return True
        print(f"⚠️ Bark 发送失败: {resp.status_code} - {resp.text[:200]}")
    except Exception as e:
        print(f"⚠️ Bark 发送异常: {e}")
    return False


# ============================================================================
# Panel API 客户端
# ============================================================================
class PanelAPIClient:
    """线上 Panel API 客户端"""

    def __init__(self):
        self.base_url = Config.PANEL_API_BASE
        self.token: Optional[str] = None

    def login(self) -> bool:
        """登录获取 JWT Token"""
        try:
            resp = std_requests.post(
                f"{self.base_url}/api/v1/auth/login",
                json={"username": Config.PANEL_USERNAME, "password": Config.PANEL_PASSWORD},
                timeout=30
            )
            if resp.status_code == 200:
                data = resp.json()
                self.token = data.get("token")
                print(f"✅ Panel API 登录成功")
                return True
            else:
                print(f"❌ Panel API 登录失败: {resp.status_code} - {resp.text[:200]}")
                return False
        except Exception as e:
            print(f"❌ Panel API 登录异常: {e}")
            return False

    def _get_headers(self) -> Dict:
        """获取带认证的请求头"""
        return {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json",
        }

    def fetch_accounts(self, page: int = 1, page_size: int = 100, status: str = "") -> Optional[Dict]:
        """获取账号列表（单页）"""
        if not self.token:
            if not self.login():
                return None

        try:
            params = {"page": page, "page_size": page_size}
            if status:
                params["status"] = status

            resp = std_requests.get(
                f"{self.base_url}/api/v1/accounts",
                params=params,
                headers=self._get_headers(),
                timeout=30
            )
            if resp.status_code == 200:
                return resp.json()
            else:
                print(f"❌ 获取账号列表失败: {resp.status_code} - {resp.text[:200]}")
                return None
        except Exception as e:
            print(f"❌ 获取账号列表异常: {e}")
            return None

    def fetch_all_accounts(self, page_size: int = 100, status: str = "") -> List[Dict]:
        """获取所有账号（自动分页）"""
        all_accounts = []
        page = 1

        while True:
            result = self.fetch_accounts(page=page, page_size=page_size, status=status)
            if not result:
                break

            accounts = result.get("accounts", result.get("data", []))
            if not accounts:
                break

            all_accounts.extend(accounts)

            # 检查是否还有更多页
            # 后端返回格式: {"data": [...], "total": 250, "page": 1, "page_size": 100, "total_pages": 3}
            # 或者: {"pagination": {"total_pages": 3, "page": 1}}
            pagination = result.get("pagination", {})
            total_pages = pagination.get("total_pages") or result.get("total_pages", 1)
            current_page = pagination.get("page") or result.get("page", page)

            print(f"   📄 已获取第 {current_page}/{total_pages} 页，累计 {len(all_accounts)} 个账号")

            if current_page >= total_pages:
                break

            page += 1

        return all_accounts

    def update_refresh_token(self, account_id: int, refresh_token: str) -> bool:
        """更新账号的 Refresh Token"""
        if not self.token:
            if not self.login():
                return False

        try:
            resp = std_requests.patch(
                f"{self.base_url}/api/v1/accounts/{account_id}/refresh-token",
                json={"refresh_token": refresh_token},
                headers=self._get_headers(),
                timeout=30
            )
            if resp.status_code == 200:
                print(f"✅ Refresh Token 更新成功 (账号ID: {account_id})")
                return True
            else:
                print(f"❌ 更新 Refresh Token 失败: {resp.status_code} - {resp.text[:200]}")
                return False
        except Exception as e:
            print(f"❌ 更新 Refresh Token 异常: {e}")
            return False

    def update_account(self, account_id: int, data: Dict) -> bool:
        """更新账号信息"""
        if not self.token:
            if not self.login():
                return False

        try:
            resp = std_requests.put(
                f"{self.base_url}/api/v1/accounts/{account_id}",
                json=data,
                headers=self._get_headers(),
                timeout=30
            )
            if resp.status_code == 200:
                print(f"✅ 账号信息更新成功 (账号ID: {account_id})")
                return True
            else:
                print(f"❌ 更新账号信息失败: {resp.status_code} - {resp.text[:200]}")
                return False
        except Exception as e:
            print(f"❌ 更新账号信息异常: {e}")
            return False

    def update_status(self, account_id: int, status: str) -> bool:
        """仅更新账号状态"""
        if not self.token:
            if not self.login():
                return False

        try:
            resp = std_requests.patch(
                f"{self.base_url}/api/v1/accounts/{account_id}/status",
                json={"status": status},
                headers=self._get_headers(),
                timeout=30
            )
            if resp.status_code == 200:
                print(f"✅ 账号状态已更新为 {status} (账号ID: {account_id})")
                return True
            else:
                print(f"❌ 更新账号状态失败: {resp.status_code} - {resp.text[:200]}")
                return False
        except Exception as e:
            print(f"❌ 更新账号状态异常: {e}")
            return False


# ============================================================================
# 账号导入工具
# ============================================================================
def extract_account_info(access_token: str) -> Dict:
    """从 access_token 中提取账号信息"""
    info = {
        "account_id": "",
        "subscription_status": "free",
        "user_id": "",
    }

    if not access_token:
        return info

    try:
        payload = jwt.decode(access_token, options={"verify_signature": False})
        auth_info = payload.get("https://api.openai.com/auth", {})
        info["account_id"] = auth_info.get("chatgpt_account_id", "")
        info["subscription_status"] = auth_info.get("chatgpt_plan_type", "free")
        info["user_id"] = auth_info.get("chatgpt_user_id", "")
    except Exception as e:
        print(f"   ⚠️ JWT 解码失败: {e}")

    return info


def import_to_panel(email: str, password: str, tokens: Dict) -> bool:
    """将账号导入到线上项目"""
    if not tokens:
        return False

    # 从 access_token 提取账号信息
    account_info = extract_account_info(tokens.get("access_token", ""))

    import_data = {
        "email": email,
        "password": password,
        "access_token": tokens.get("access_token", ""),
        "refresh_token": tokens.get("refresh_token", ""),
        "account_id": account_info.get("account_id", ""),
        "status": "active",
        "created_at": datetime.now().isoformat(),
    }

    url = f"{Config.PANEL_API_BASE}{Config.PANEL_IMPORT_ENDPOINT}"

    print(f"\n📤 正在导入账号到线上项目...")
    print(f"   URL: {url}")
    print(f"   Email: {email}")
    print(f"   Account ID: {account_info.get('account_id', 'N/A')}")

    try:
        headers = {
            "Content-Type": "application/json",
            "User-Agent": "ChatGPT-OAuth-Login/1.0",
        }

        resp = std_requests.post(
            url,
            json=import_data,
            headers=headers,
            timeout=30
        )

        if resp.status_code == 200:
            result = resp.json()
            print(f"✅ 导入成功! (ID: {result.get('id', 'N/A')})")
            return True
        else:
            print(f"❌ 导入失败: {resp.status_code} - {resp.text[:200]}")
            return False

    except Exception as e:
        print(f"❌ 导入异常: {e}")
        return False


# ============================================================================
# 邮箱验证码获取 (mail.chatgpt.org.uk)
# ============================================================================
def _fetch_mail_messages(email: str) -> list:
    try:
        resp = std_requests.get(
            f"{Config.MAIL_API_BASE}/emails",
            params={"email": email},
            headers={
                "User-Agent": "Mozilla/5.0",
                "Origin": "https://mail.chatgpt.org.uk",
                "Referer": "https://mail.chatgpt.org.uk/",
            },
            timeout=Config.TIMEOUT,
        )
        if resp.status_code == 200:
            data = resp.json()
            if data.get("success") and data.get("data", {}).get("emails"):
                return data["data"]["emails"]
    except Exception:
        pass
    return []


def get_email_verification_code(email: str) -> Optional[str]:
    """自动拉取邮箱验证码"""
    print(f"⏳ 自动获取 {email} 的验证码...")
    code_regex = re.compile(r'\b[A-Z0-9]{3}-[A-Z0-9]{3}\b|\b\d{6}\b')
    checked_msg_ids = set()

    for _ in range(Config.OTP_MAX_ATTEMPTS):
        msgs = _fetch_mail_messages(email)
        if msgs:
            for msg in msgs:
                msg_id = msg.get('id') or msg.get('subject', '') + str(msg.get('date', ''))
                if msg_id in checked_msg_ids:
                    continue
                checked_msg_ids.add(msg_id)

                content = " ".join([
                    str(msg.get('subject') or ''),
                    str(msg.get('html_content') or ''),
                    str(msg.get('text_content') or ''),
                    str(msg.get('body') or ''),
                    str(msg.get('content') or ''),
                ])

                matches = code_regex.findall(content)
                if matches:
                    code = matches[0].replace('-', '')
                    print(f"✅ 获取到验证码: {code}")
                    return code
        time.sleep(Config.OTP_INTERVAL)
    print("⚠️ 获取验证码超时")
    return None


# ============================================================================
# PKCE 工具
# ============================================================================
class PKCE:
    """PKCE (Proof Key for Code Exchange) 工具类"""

    @staticmethod
    def generate_code_verifier(length: int = 128) -> str:
        """生成 code_verifier (43-128字符的随机字符串)"""
        random_bytes = secrets.token_bytes(96)
        return pybase64.urlsafe_b64encode(random_bytes).decode('ascii').rstrip('=')

    @staticmethod
    def generate_code_challenge(code_verifier: str) -> str:
        """根据 code_verifier 生成 code_challenge (S256)"""
        digest = hashlib.sha256(code_verifier.encode('ascii')).digest()
        return pybase64.urlsafe_b64encode(digest).decode('ascii').rstrip('=')

    @staticmethod
    def generate() -> Tuple[str, str]:
        """生成 PKCE codes"""
        verifier = PKCE.generate_code_verifier()
        challenge = PKCE.generate_code_challenge(verifier)
        return verifier, challenge


# ============================================================================
# OAuth 回调服务器
# ============================================================================
class OAuthCallbackHandler(BaseHTTPRequestHandler):
    """OAuth回调处理器"""
    callback_result = None
    callback_event = None

    def log_message(self, format, *args):
        pass  # 静默日志

    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == '/auth/callback':
            params = parse_qs(parsed.query)
            code = params.get('code', [None])[0]
            state = params.get('state', [None])[0]
            error = params.get('error', [None])[0]

            OAuthCallbackHandler.callback_result = {
                'code': code,
                'state': state,
                'error': error
            }

            if code:
                self.send_response(200)
                self.send_header('Content-type', 'text/html; charset=utf-8')
                self.end_headers()
                self.wfile.write(b'''<!DOCTYPE html><html><head><meta charset="utf-8"><title>Success</title>
                <style>body{font-family:sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;background:#10b981;}
                .box{background:white;padding:2rem;border-radius:12px;text-align:center;}</style></head>
                <body><div class="box"><h1>&#10004; Authorization Success!</h1><p>You can close this window.</p></div></body></html>''')
            else:
                self.send_response(400)
                self.send_header('Content-type', 'text/html; charset=utf-8')
                self.end_headers()
                self.wfile.write(f'<html><body><h1>Error: {error}</h1></body></html>'.encode())

            if OAuthCallbackHandler.callback_event:
                OAuthCallbackHandler.callback_event.set()
        else:
            self.send_response(404)
            self.end_headers()


class OAuthCallbackServer:
    """OAuth回调服务器"""

    def __init__(self, port: int = Config.CALLBACK_PORT):
        self.port = port
        self.server = None
        self.thread = None
        self.event = threading.Event()
        OAuthCallbackHandler.callback_event = self.event
        OAuthCallbackHandler.callback_result = None

    def start(self):
        """启动服务器"""
        self.server = HTTPServer(('localhost', self.port), OAuthCallbackHandler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        print(f"✅ 回调服务器已启动: http://localhost:{self.port}/auth/callback")

    def wait_for_callback(self, timeout: int = 300) -> Optional[Dict]:
        """等待回调"""
        if self.event.wait(timeout=timeout):
            return OAuthCallbackHandler.callback_result
        return None

    def stop(self):
        """停止服务器"""
        if self.server:
            self.server.shutdown()
            print("✅ 回调服务器已停止")


# ============================================================================
# Sentinel Token 生成器 (从注册脚本复用完整版)
# ============================================================================
class SentinelTokenGenerator:
    """OpenAI Sentinel Token 生成器"""

    FNV_OFFSET = 2166136261
    FNV_PRIME = 16777619
    MAX_POW_ATTEMPTS = 500000

    def __init__(self, device_id: str, session: requests.Session):
        self.device_id = device_id
        self.session = session
        self._perf_origin: Optional[float] = None
        self._sentinel_cache: Optional[dict] = None
        self._sentinel_cache_time: float = 0

    def _get_perf_now(self) -> float:
        """获取 performance.now() 模拟值"""
        if self._perf_origin is None:
            self._perf_origin = time.time() * 1000
        return time.time() * 1000 - self._perf_origin

    def _get_fingerprint_config(self, nonce: int = 0, elapsed: int = 0) -> list:
        """生成浏览器指纹配置数组"""
        from datetime import timezone, timedelta
        now = datetime.now(timezone(timedelta(hours=0)))  # UTC时间
        date_str = now.strftime("%a %b %d %Y %H:%M:%S") + " GMT+0000 (Greenwich Mean Time)"

        navigator_props = [
            "mediaCapabilities−[object MediaCapabilities]",
            "permissions−[object Permissions]",
            "storage−[object StorageManager]",
            "cookieEnabled−true",
            "language−en-US",
        ]
        document_props = ["_reactListeningc5rfos7jrvl", "location", "body", "scripts"]
        window_props = ["DD_RUM", "window", "self", "document", "location"]

        return [
            3000,
            date_str,
            4294705152,
            nonce,
            Config.USER_AGENT,
            "https://sentinel.openai.com/sentinel/97790f37/sdk.js",
            None,
            "en-US",
            "en-US,en",
            elapsed if elapsed else random.randint(1, 30),
            random.choice(navigator_props),
            random.choice(document_props),
            random.choice(window_props),
            self._get_perf_now(),
            self.device_id,
            "",
            20,
            self._perf_origin or time.time() * 1000,
        ]

    def _encode_config(self, config: list) -> str:
        """编码配置为 Base64"""
        json_str = json.dumps(config, separators=(",", ":"), ensure_ascii=False)
        return pybase64.b64encode(json_str.encode('utf-8')).decode('ascii')

    def _fnv1a_hash(self, data: str) -> str:
        """FNV-1a 哈希算法 (OpenAI 变体)"""
        hash_val = self.FNV_OFFSET
        for char in data:
            hash_val ^= ord(char)
            hash_val = (hash_val * self.FNV_PRIME) & 0xFFFFFFFF
        hash_val ^= hash_val >> 16
        hash_val = (hash_val * 2246822507) & 0xFFFFFFFF
        hash_val ^= hash_val >> 13
        hash_val = (hash_val * 3266489909) & 0xFFFFFFFF
        hash_val ^= hash_val >> 16
        return format(hash_val, '08x')

    def _solve_pow(self, seed: str, difficulty: str) -> str:
        """解决 ProofOfWork 挑战"""
        start_time = self._get_perf_now()
        for i in range(self.MAX_POW_ATTEMPTS):
            elapsed = int(self._get_perf_now() - start_time)
            config = self._get_fingerprint_config(nonce=i, elapsed=elapsed)
            encoded = self._encode_config(config)
            hash_input = seed + encoded
            hash_result = self._fnv1a_hash(hash_input)
            if hash_result[:len(difficulty)] <= difficulty:
                return encoded + "~S"
        raise Exception("ProofOfWork 解决失败")

    def _generate_p_token(self) -> str:
        """生成 p token (指纹数据)"""
        config = self._get_fingerprint_config()
        encoded = self._encode_config(config)
        return "gAAAAAC" + encoded

    def _request_sentinel_token(self, flow: str) -> dict:
        """请求 Sentinel Token"""
        p_token = self._generate_p_token()
        payload = json.dumps({"p": p_token}, separators=(",", ":"))

        headers = {
            "User-Agent": Config.USER_AGENT,
            "Accept": "*/*",
            "Accept-Language": "en-US,en;q=0.9",
            "Content-Type": "text/plain;charset=UTF-8",
            "Origin": "https://sentinel.openai.com",
            "Referer": "https://sentinel.openai.com/backend-api/sentinel/frame.html",
            "sec-ch-ua": Config.SEC_CH_UA,
            "sec-ch-ua-mobile": "?0",
            "sec-ch-ua-platform": '"Windows"',
        }

        resp = self.session.post(
            f"{Config.SENTINEL_BASE}/req",
            headers=headers,
            data=payload,
            timeout=Config.TIMEOUT
        )

        if resp.status_code != 200:
            raise Exception(f"Sentinel API 错误: {resp.status_code}")
        return resp.json()

    def generate(self, flow: str = "authorize_continue") -> str:
        """生成 openai-sentinel-token"""
        now = time.time()
        if self._sentinel_cache and (now - self._sentinel_cache_time) < 500:
            response = self._sentinel_cache
        else:
            try:
                response = self._request_sentinel_token(flow)
                self._sentinel_cache = response
                self._sentinel_cache_time = now
                print(f"   [Sentinel] 获取新 token 成功")
            except Exception as e:
                print(f"   [Sentinel] 请求失败，使用本地生成: {e}")
                return self._generate_local(flow)

        base_token = response.get("token", "")

        pow_info = response.get("proofofwork", {})
        if pow_info.get("required"):
            seed = pow_info.get("seed", "")
            difficulty = pow_info.get("difficulty", "")
            try:
                self._solve_pow(seed, difficulty)
                print(f"   [Sentinel] PoW 解决成功")
            except Exception as e:
                print(f"   [Sentinel] PoW 解决失败: {e}")

        sentinel = {
            "p": self._generate_p_token(),
            "t": None,
            "c": base_token,
            "id": self.device_id,
            "flow": flow,
        }
        return json.dumps(sentinel, separators=(",", ":"))

    def _generate_local(self, flow: str) -> str:
        """本地生成 sentinel token (降级方案)"""
        p_token = self._generate_p_token()
        t_base = "SBMYGQ8GExQV"
        t_random_bytes = bytes([random.randint(0, 255) for _ in range(100)])
        t_token = t_base + pybase64.b64encode(t_random_bytes).decode()

        c_data = json.dumps({"seed": f"{random.random():.16f}", "difficulty": "0fffff"}, separators=(",", ":"))
        c_token = "gAAAAABp" + pybase64.b64encode(c_data.encode()).decode()

        sentinel = {"p": p_token, "t": t_token, "c": c_token, "id": self.device_id, "flow": flow}
        return json.dumps(sentinel, separators=(",", ":"))



# ============================================================================
# OAuth 登录客户端
# ============================================================================
class ChatGPTOAuthClient:
    """ChatGPT OAuth 登录客户端"""

    def __init__(self):
        self.session = requests.Session(impersonate=Config.IMPERSONATE, proxy=Config.PROXY)
        self.device_id = str(__import__('uuid').uuid4())
        self.sentinel_generator = SentinelTokenGenerator(self.device_id, self.session)
        self.code_verifier: Optional[str] = None
        self.code_challenge: Optional[str] = None
        self.state: Optional[str] = None
        self.consent_forbidden: bool = False

    def _delay(self, min_sec: float = 0.5, max_sec: float = 1.5):
        time.sleep(random.uniform(min_sec, max_sec))

    def _get_api_headers(self, referer: str, with_sentinel: bool = False, flow: str = "authorize_continue") -> Dict:
        headers = {
            "User-Agent": Config.USER_AGENT,
            "Accept": "application/json",
            "Accept-Language": "zh-CN,zh;q=0.9",
            "Content-Type": "application/json",
            "Origin": Config.AUTH_BASE,
            "Referer": referer,
            "sec-ch-ua": Config.SEC_CH_UA,
            "sec-ch-ua-mobile": "?0",
            "sec-ch-ua-platform": '"Windows"',
            "Sec-Fetch-Dest": "empty",
            "Sec-Fetch-Mode": "cors",
            "Sec-Fetch-Site": "same-origin",
        }
        if with_sentinel:
            headers["openai-sentinel-token"] = self.sentinel_generator.generate(flow)
        return headers

    def step1_generate_auth_url(self) -> str:
        """步骤1: 生成OAuth授权URL"""
        print("\n📍 步骤1: 生成OAuth授权URL")

        # 生成PKCE codes
        self.code_verifier, self.code_challenge = PKCE.generate()
        print(f"   Code Verifier: {self.code_verifier[:30]}...")
        print(f"   Code Challenge: {self.code_challenge[:30]}...")

        # 生成state
        self.state = pybase64.urlsafe_b64encode(secrets.token_bytes(32)).decode('ascii').rstrip('=')
        print(f"   State: {self.state[:30]}...")

        # 构建授权URL
        params = {
            "client_id": Config.CLIENT_ID,
            "response_type": "code",
            "redirect_uri": Config.REDIRECT_URI,
            "scope": Config.SCOPE,
            "state": self.state,
            "code_challenge": self.code_challenge,
            "code_challenge_method": "S256",
            "prompt": "login",
            "id_token_add_organizations": "true",
            "codex_cli_simplified_flow": "true",
        }

        auth_url = f"{Config.AUTH_URL}?{urlencode(params)}"
        print(f"✅ 授权URL已生成")
        return auth_url

    def step2_init_auth_session(self, auth_url: str) -> bool:
        """步骤2: 初始化认证会话"""
        print("\n📍 步骤2: 初始化认证会话")
        try:
            # 先访问chatgpt.com建立Cloudflare会话
            print("   建立Cloudflare会话...")
            self.session.get(Config.CHATGPT_BASE, timeout=Config.TIMEOUT)
            self._delay()

            # 再访问auth.openai.com建立会话
            print("   建立auth.openai.com会话...")
            self.session.get(f"{Config.AUTH_BASE}/", timeout=Config.TIMEOUT)
            self._delay()

            # 访问授权URL
            resp = self.session.get(auth_url, timeout=Config.TIMEOUT, allow_redirects=True)
            print(f"   响应状态: {resp.status_code}")
            print(f"   最终URL: {resp.url[:80]}...")

            if resp.status_code == 200:
                print(f"✅ 认证会话已建立")
                return True
            elif resp.status_code == 403:
                print(f"⚠️ 遇到403，尝试重试...")
                self._delay(2, 3)
                resp = self.session.get(auth_url, timeout=Config.TIMEOUT, allow_redirects=True)
                if resp.status_code == 200:
                    print(f"✅ 认证会话已建立 (重试成功)")
                    return True
            return False
        except Exception as e:
            print(f"❌ 初始化会话失败: {e}")
            return False

    def step3_submit_email(self, email: str) -> Tuple[bool, str]:
        """步骤3: 提交邮箱"""
        print(f"\n📍 步骤3: 提交邮箱 ({email})")
        try:
            self._delay()
            headers = self._get_api_headers(
                referer=f"{Config.AUTH_BASE}/log-in-or-create-account",
                with_sentinel=True, flow="authorize_continue"
            )

            resp = self.session.post(
                f"{Config.AUTH_BASE}/api/accounts/authorize/continue",
                json={"username": {"value": email, "kind": "email"}, "screen_hint": "login_or_signup"},
                headers=headers, timeout=Config.TIMEOUT
            )

            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                result = resp.json()
                page_type = result.get('page', {}).get('type', '')
                print(f"   页面类型: {page_type}")

                if 'password' in page_type:
                    print(f"✅ 邮箱已验证，进入密码页面")
                    return True, "password"
                elif 'create_account' in page_type:
                    print(f"⚠️ 账号不存在，需要注册")
                    return False, "not_registered"

            print(f"❌ 提交邮箱失败: {resp.text[:200]}")
            return False, "error"
        except Exception as e:
            print(f"❌ 提交邮箱异常: {e}")
            return False, "error"

    def step4_submit_password(self, email: str, password: str) -> Tuple[bool, str]:
        """步骤4: 提交密码 (OAuth授权流程中的密码验证)"""
        print(f"\n📍 步骤4: 提交密码")
        try:
            self._delay()
            headers = self._get_api_headers(
                referer=f"{Config.AUTH_BASE}/log-in/password",
                with_sentinel=True, flow="password_verify"
            )

            # 使用正确的密码验证端点 (通过抓包确认)
            resp = self.session.post(
                f"{Config.AUTH_BASE}/api/accounts/password/verify",
                json={"username": email, "password": password},
                headers=headers, timeout=Config.TIMEOUT
            )

            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                result = resp.json()
                print(f"   响应内容: {str(result)[:200]}")

                # 检查是否需要验证码
                page_type = result.get('page', {}).get('type', '')
                continue_url = result.get('continue_url', '')

                if 'otp' in page_type or 'verification' in page_type:
                    print(f"⚠️ 需要邮箱验证码")
                    return True, "otp_required"

                # 密码验证成功，尝试从cookies中获取workspace信息
                print(f"✅ 密码验证成功")

                # 检查是否需要选择workspace (consent页面)
                if 'consent' in str(result) or page_type == 'workspace_select':
                    return True, "workspace_select"
                elif continue_url:
                    return True, continue_url
                else:
                    # 默认需要workspace选择
                    return True, "workspace_select"

            print(f"❌ 密码验证失败: {resp.text[:200]}")
            return False, "error"
        except Exception as e:
            print(f"❌ 密码验证异常: {e}")
            return False, "error"

    def step5_select_workspace(self, workspace_id: str = None, workspace_type: str = None) -> Tuple[bool, str]:
        """步骤5: 选择workspace (点击继续按钮)

        Args:
            workspace_id: 直接指定 workspace ID
            workspace_type: 指定 workspace 类型 ("personal" 或 "team")
        """
        print(f"\n📍 步骤5: 选择Workspace (同意授权)")
        try:
            self._delay()

            # 如果没有提供workspace_id，根据类型或默认从cookies中获取
            if not workspace_id:
                workspace_id = self._get_workspace_id_from_cookies(workspace_type)

            if not workspace_id:
                print("❌ 无法获取workspace_id")
                return False, "error"

            print(f"   Workspace ID: {workspace_id}")
            if workspace_type:
                print(f"   Workspace Type: {workspace_type}")

            headers = self._get_api_headers(
                referer=f"{Config.AUTH_BASE}/sign-in-with-chatgpt/consent",
                with_sentinel=False
            )

            resp = self.session.post(
                f"{Config.AUTH_BASE}/api/accounts/workspace/select",
                json={"workspace_id": workspace_id},
                headers=headers, timeout=Config.TIMEOUT
            )

            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                result = resp.json()
                print(f"   响应内容: {str(result)[:200]}")
                continue_url = result.get('continue_url', '')
                if continue_url:
                    print(f"✅ Workspace选择成功")
                    print(f"   Continue URL: {continue_url[:80]}...")
                    return True, continue_url

            print(f"❌ Workspace选择失败: {resp.text[:200]}")
            return False, "error"
        except Exception as e:
            print(f"❌ Workspace选择异常: {e}")
            return False, "error"

    def _get_workspace_id_from_cookies(self, workspace_type: str = None) -> Optional[str]:
        """从cookies中解析workspace_id

        Args:
            workspace_type: 指定 workspace 类型
                - None: 返回第一个 workspace
                - "personal": 返回 Personal workspace (kind="personal")
                - "team": 返回 Team/Organization workspace (kind="organization")
        """
        import base64
        try:
            cookies = self.session.cookies
            # 直接通过名称获取cookie值
            cookie_value = cookies.get('oai-client-auth-session')
            if cookie_value:
                # 解码base64 (只取第一部分，去掉签名)
                value = cookie_value.split('.')[0]
                # 添加padding
                padding = 4 - len(value) % 4
                if padding != 4:
                    value += '=' * padding
                decoded = base64.b64decode(value).decode('utf-8')
                data = json.loads(decoded)
                workspaces = data.get('workspaces', [])

                if not workspaces:
                    return None

                # 如果没有指定类型，返回第一个
                if not workspace_type:
                    return workspaces[0].get('id')

                # 根据 kind 字段筛选
                for ws in workspaces:
                    kind = ws.get('kind', '').lower()
                    ws_id = ws.get('id', '')
                    ws_name = ws.get('name') or '个人帐户'

                    if workspace_type.lower() == 'personal':
                        # Personal workspace: kind="personal"
                        if kind == 'personal':
                            print(f"   找到 Personal workspace: {ws_name} ({ws_id})")
                            return ws_id
                    elif workspace_type.lower() == 'team':
                        # Team/Organization workspace: kind="organization"
                        if kind == 'organization':
                            print(f"   找到 Team workspace: {ws_name} ({ws_id})")
                            return ws_id

                # 如果没找到指定类型，打印可用的 workspaces
                print(f"   ⚠️ 未找到 {workspace_type} 类型的 workspace")
                available = [(ws.get('name') or '个人帐户', ws.get('kind')) for ws in workspaces]
                print(f"   可用 workspaces: {available}")
                return None

        except Exception as e:
            print(f"   解析workspace失败: {e}")
        return None

    def _get_all_workspaces_from_cookies(self) -> list:
        """获取所有 workspaces 列表"""
        import base64
        try:
            cookies = self.session.cookies
            cookie_value = cookies.get('oai-client-auth-session')
            if cookie_value:
                value = cookie_value.split('.')[0]
                padding = 4 - len(value) % 4
                if padding != 4:
                    value += '=' * padding
                decoded = base64.b64decode(value).decode('utf-8')
                data = json.loads(decoded)
                return data.get('workspaces', [])
        except Exception as e:
            print(f"   获取workspaces失败: {e}")
        return []



    def step5_submit_otp(self, code: str) -> Tuple[bool, str]:
        """步骤5: 提交邮箱验证码"""
        print(f"\n📍 步骤5: 提交验证码 ({code})")
        try:
            self._delay()
            headers = self._get_api_headers(
                referer=f"{Config.AUTH_BASE}/email-verification",
                with_sentinel=True, flow="email_otp_validate"
            )

            resp = self.session.post(
                f"{Config.AUTH_BASE}/api/accounts/email-otp/validate",
                json={"code": code},
                headers=headers, timeout=Config.TIMEOUT
            )

            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                result = resp.json()
                print(f"   响应内容: {str(result)[:200]}")

                print(f"✅ 验证码验证成功")

                # 验证码验证成功后，总是需要选择 workspace
                # 这和密码验证成功后的行为一致
                return True, "workspace_select"

            print(f"❌ 验证码验证失败: {resp.text[:200]}")
            return False, "error"
        except Exception as e:
            print(f"❌ 验证码验证异常: {e}")
            return False, "error"

    def step6_handle_consent(self, consent_url: str) -> Optional[str]:
        """步骤6: 处理consent页面，获取回调URL"""
        print(f"\n📍 步骤6: 处理授权同意页面")
        print(f"   URL: {consent_url[:100]}...")
        try:
            self._delay()

            # 设置 allow_redirects=False 以捕获重定向地址
            # 因为重定向到 localhost:1455 本地没有服务器会失败
            resp = self.session.get(consent_url, timeout=Config.TIMEOUT, allow_redirects=False)
            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 403:
                self.consent_forbidden = True
                return None

            # 处理 302 重定向
            if resp.status_code in (301, 302, 303, 307, 308):
                location = resp.headers.get('Location', '')
                print(f"   重定向到: {location[:100]}...")

                # 检查是否是回调URL
                if 'callback' in location and 'code=' in location:
                    print(f"✅ 获取到回调URL")
                    return location

                # 如果是相对路径，拼接完整URL
                if location.startswith('/'):
                    location = f"{Config.AUTH_BASE}{location}"

                # 如果重定向不是回调，继续跟随（但还是用 allow_redirects=False）
                if not location.startswith('http://localhost'):
                    return self.step6_handle_consent(location)
                else:
                    # 是 localhost 但没有 code，直接返回
                    return location if 'code=' in location else None

            # 如果响应是 200，检查是否已经是回调URL
            if resp.status_code == 200:
                if Config.DEBUG_CONSENT:
                    try:
                        content_type = resp.headers.get("content-type", "")
                        print(f"   [Debug] content-type: {content_type}")
                        print(f"   [Debug] final-url: {resp.url[:200]}")
                        os.makedirs(Config.DEBUG_CONSENT_DIR, exist_ok=True)
                        ts = datetime.now().strftime("%Y%m%d_%H%M%S")
                        debug_path = os.path.join(Config.DEBUG_CONSENT_DIR, f"consent_{ts}.html")
                        with open(debug_path, "w", encoding="utf-8") as f:
                            f.write(resp.text or "")
                        print(f"   [Debug] 保存 consent HTML: {debug_path}")
                    except Exception as e:
                        print(f"   [Debug] 保存 consent HTML 失败: {e}")

                if 'callback' in resp.url and 'code=' in resp.url:
                    print(f"✅ 获取到回调URL")
                    return resp.url

                # 检查响应内容是否有下一步URL
                try:
                    result = resp.json()
                    continue_url = result.get('continue_url', '')
                    if Config.DEBUG_CONSENT and result:
                        print(f"   [Debug] consent json keys: {list(result.keys())}")
                    if continue_url:
                        print(f"   发现continue_url，继续处理...")
                        return self.step6_handle_consent(continue_url)
                except:
                    pass

                # 处理 HTML consent 表单（自动点击同意）
                html = resp.text or ""
                form_match = re.search(r'<form[^>]+action="([^"]+)"[^>]*>', html, re.I)
                if Config.DEBUG_CONSENT:
                    if form_match:
                        method_match = re.search(r'<form[^>]+method="([^"]+)"', html, re.I)
                        method_dbg = (method_match.group(1) if method_match else "post").lower()
                        print(f"   [Debug] form action: {form_match.group(1)[:200]} | method: {method_dbg}")
                    else:
                        print("   [Debug] 未找到 consent 表单")
                if form_match:
                    action = form_match.group(1)
                    method_match = re.search(r'<form[^>]+method="([^"]+)"', html, re.I)
                    method = (method_match.group(1) if method_match else "post").lower()

                    inputs = {}
                    for m in re.finditer(r'<input[^>]+>', html, re.I):
                        tag = m.group(0)
                        name_m = re.search(r'name="([^"]+)"', tag, re.I)
                        if not name_m:
                            continue
                        name = name_m.group(1)
                        value_m = re.search(r'value="([^"]*)"', tag, re.I)
                        value = value_m.group(1) if value_m else ""
                        inputs[name] = value
                    if Config.DEBUG_CONSENT:
                        print(f"   [Debug] form inputs: {list(inputs.keys())[:20]}")

                    # 处理 submit 按钮
                    submit_m = re.search(r'<input[^>]+type="submit"[^>]*>', html, re.I)
                    if submit_m:
                        tag = submit_m.group(0)
                        name_m = re.search(r'name="([^"]+)"', tag, re.I)
                        value_m = re.search(r'value="([^"]*)"', tag, re.I)
                        if name_m and value_m:
                            inputs[name_m.group(1)] = value_m.group(1)
                    else:
                        btn_m = re.search(r'<button[^>]+name="([^"]+)"[^>]*value="([^"]+)"[^>]*>', html, re.I)
                        if btn_m:
                            inputs[btn_m.group(1)] = btn_m.group(2)

                    # 没有显式提交字段，尝试添加 accept
                    if not any(k in inputs for k in ("action", "accept", "consent")):
                        inputs["accept"] = "true"

                    if action.startswith('/'):
                        action = f"{Config.AUTH_BASE}{action}"

                    print("   自动提交同意表单...")
                    resp2 = self.session.request(
                        method.upper(),
                        action,
                        data=inputs if method.lower() != "get" else None,
                        params=inputs if method.lower() == "get" else None,
                        headers=self._get_api_headers(referer=consent_url, with_sentinel=True, flow="authorize_continue"),
                        timeout=Config.TIMEOUT,
                        allow_redirects=False,
                    )

                    if resp2.status_code in (301, 302, 303, 307, 308):
                        location = resp2.headers.get('Location', '')
                        print(f"   表单重定向到: {location[:100]}...")
                        if 'callback' in location and 'code=' in location:
                            print("✅ 获取到回调URL")
                            return location
                        if location.startswith('/'):
                            location = f"{Config.AUTH_BASE}{location}"
                        return self.step6_handle_consent(location)

                    if resp2.status_code == 200:
                        try:
                            result = resp2.json()
                            continue_url = result.get('continue_url', '')
                            if continue_url:
                                print("   表单返回continue_url，继续处理...")
                                return self.step6_handle_consent(continue_url)
                        except:
                            pass

            return None
        except Exception as e:
            print(f"❌ 处理consent页面异常: {e}")
            return None

    def step7_exchange_code(self, code: str) -> Optional[Dict]:
        """步骤7: 用授权码换取tokens"""
        print(f"\n📍 步骤7: 用授权码换取tokens")
        try:
            data = {
                "grant_type": "authorization_code",
                "client_id": Config.CLIENT_ID,
                "code": code,
                "redirect_uri": Config.REDIRECT_URI,
                "code_verifier": self.code_verifier,
            }

            headers = {
                "Content-Type": "application/x-www-form-urlencoded",
                "Accept": "application/json",
                "User-Agent": Config.USER_AGENT,
            }

            resp = self.session.post(
                Config.TOKEN_URL,
                data=data,
                headers=headers,
                timeout=Config.TIMEOUT
            )

            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                tokens = resp.json()
                print(f"✅ Token获取成功!")
                print(f"   Access Token: {tokens.get('access_token', '')[:50]}...")
                if tokens.get('refresh_token'):
                    print(f"   Refresh Token: {tokens.get('refresh_token', '')[:50]}...")
                if tokens.get('id_token'):
                    print(f"   ID Token: {tokens.get('id_token', '')[:50]}...")
                return tokens

            print(f"❌ Token换取失败: {resp.text[:300]}")
            return None
        except Exception as e:
            print(f"❌ Token换取异常: {e}")
            return None

    def process_callback_url(self, callback_url: str) -> Optional[Dict]:
        """处理回调URL，提取code并换取token"""
        print(f"\n📍 处理回调URL")
        try:
            parsed = urlparse(callback_url)
            params = parse_qs(parsed.query)

            code = params.get('code', [None])[0]
            state = params.get('state', [None])[0]

            if not code:
                print(f"❌ 回调URL中没有code参数")
                return None

            print(f"   Code: {code[:50]}...")
            print(f"   State: {state[:30]}..." if state else "   State: None")

            # 验证state
            if state and self.state and state != self.state:
                print(f"⚠️ State不匹配，但继续处理...")

            return self.step7_exchange_code(code)
        except Exception as e:
            print(f"❌ 处理回调URL异常: {e}")
            return None



# ============================================================================
# 主函数
# ============================================================================
def parse_selection(selection: str, max_count: int) -> list:
    """解析用户输入的选择，支持多种格式

    支持格式:
    - 单个: "5"
    - 范围: "3-20"
    - 多个: "1,3,5,7"
    - 混合: "1,3-5,8,10-12"
    """
    indices = set()
    parts = selection.replace(" ", "").split(",")

    for part in parts:
        if "-" in part:
            # 范围格式: "3-20"
            try:
                start, end = part.split("-", 1)
                start_idx = int(start)
                end_idx = int(end)
                if start_idx > end_idx:
                    start_idx, end_idx = end_idx, start_idx
                for i in range(start_idx, end_idx + 1):
                    if 1 <= i <= max_count:
                        indices.add(i)
            except ValueError:
                continue
        else:
            # 单个数字
            try:
                idx = int(part)
                if 1 <= idx <= max_count:
                    indices.add(idx)
            except ValueError:
                continue

    return sorted(list(indices))


def display_accounts_menu(accounts: list, batch_mode: bool = False) -> Optional[list]:
    """显示账号列表菜单并让用户选择

    Args:
        accounts: 账号列表
        batch_mode: 是否批量模式，批量模式返回账号列表

    Returns:
        batch_mode=False: 返回单个账号 dict 或 None
        batch_mode=True: 返回账号列表 list 或 None
    """
    if not accounts:
        print("❌ 没有可用的账号")
        return None

    print("\n" + "=" * 70)
    print("📋 账号列表")
    print("=" * 70)
    print(f"{'序号':<6}{'邮箱':<40}{'状态':<12}{'RT':<8}")
    print("-" * 70)

    for i, acc in enumerate(accounts, 1):
        email = acc.get("email", "N/A")[:38]
        status = acc.get("status", "N/A")
        has_rt = "✓" if acc.get("refresh_token") else "✗"
        print(f"{i:<6}{email:<40}{status:<12}{has_rt:<8}")

    print("-" * 70)
    print(f"共 {len(accounts)} 个账号")
    print("=" * 70)

    if batch_mode:
        print("\n💡 支持多选格式:")
        print("   单个: 5")
        print("   范围: 3-20")
        print("   多个: 1,3,5,7")
        print("   混合: 1,3-5,8,10-12")

    while True:
        prompt = "\n请输入账号序号 (输入 q 退出): " if not batch_mode else "\n请输入账号序号 (支持批量选择, q 退出): "
        choice = input(prompt).strip()

        if choice.lower() == 'q':
            return None

        if batch_mode:
            indices = parse_selection(choice, len(accounts))
            if indices:
                selected = [accounts[i - 1] for i in indices]
                print(f"\n✅ 已选择 {len(selected)} 个账号")
                return selected
            else:
                print(f"❌ 无效的输入，请输入 1-{len(accounts)} 之间的数字")
        else:
            try:
                idx = int(choice)
                if 1 <= idx <= len(accounts):
                    return [accounts[idx - 1]]
                else:
                    print(f"❌ 请输入 1-{len(accounts)} 之间的数字")
            except ValueError:
                print("❌ 请输入有效的数字")


def is_bound_account(account: Dict) -> bool:
    """判断账号是否为已绑卡状态

    检测条件（满足任一即可）：
    1. status == "bound"
    2. plus_bound == true
    3. team_bound == true
    """
    status = str(account.get("status", "")).lower()
    if status == "bound":
        return True
    if account.get("plus_bound"):
        return True
    if account.get("team_bound"):
        return True
    return False


def login_single_account(panel_client: PanelAPIClient, account: Dict, workspace_type: str = None) -> bool:
    """处理单个账号的 OAuth 登录流程

    Args:
        panel_client: Panel API 客户端
        account: 账号信息
        workspace_type: workspace 类型 ("personal" 或 "team")，None 表示使用默认

    Returns:
        bool: 是否成功获取并更新 RT
    """
    email = account.get("email")
    password = "testuser1314"  # 固定密码
    account_id = account.get("id")

    if not email or not password:
        print(f"❌ [{email}] 账号信息不完整 (缺少邮箱或密码)")
        return False

    workspace_label = f" [{workspace_type.upper()}]" if workspace_type else ""
    print(f"\n{'='*60}")
    print(f"🔄 正在处理: {email}{workspace_label}")
    print(f"{'='*60}")

    client = ChatGPTOAuthClient()

    # 步骤1: 生成授权URL
    auth_url = client.step1_generate_auth_url()
    print(f"   🔗 授权URL已生成")

    # 步骤2: 初始化会话
    if not client.step2_init_auth_session(auth_url):
        print(f"   ❌ 初始化会话失败")
        return False

    # 步骤3: 提交邮箱
    success, result = client.step3_submit_email(email)
    if not success:
        if result == "not_registered":
            print(f"   ❌ 该邮箱未注册")
        else:
            print(f"   ❌ 邮箱提交失败")
        return False

    # 步骤4: 提交密码
    success, result = client.step4_submit_password(email, password)
    if not success:
        print(f"   ❌ 密码验证失败")
        return False

    continue_url = result

    # 步骤5a: 如果需要验证码
    if result == "otp_required":
        print(f"   ⚠️ [{email}] 需要邮箱验证码，开始自动获取")
        code = get_email_verification_code(email)
        if not code:
            print(f"   ⏭️ 未获取到验证码，跳过此账号")
            return False
        success, result = client.step5_submit_otp(code)
        if not success:
            print(f"   ❌ 验证码验证失败")
            return False
        continue_url = result

    # 步骤5b: 选择workspace (根据指定的类型)
    if result == "workspace_select" or continue_url == "workspace_select":
        # 如果指定了 workspace_type，先显示可用的 workspaces
        if workspace_type:
            all_workspaces = client._get_all_workspaces_from_cookies()
            ws_info = [(ws.get('name') or '个人帐户', ws.get('kind')) for ws in all_workspaces]
            print(f"   📋 可用 Workspaces: {ws_info}")

        success, continue_url = client.step5_select_workspace(workspace_type=workspace_type)
        if not success:
            print(f"   ❌ Workspace选择失败")
            return False

    # 步骤6: 处理consent页面
    callback_url = None
    if continue_url and continue_url.startswith("http"):
        callback_url = client.step6_handle_consent(continue_url)

    # 如果自动处理失败，直接跳过
    if not callback_url:
        if client.consent_forbidden:
            print(f"   ⏭️ 授权同意页面 403，跳过此账号")
        else:
            print(f"   ⏭️ 无法自动获取回调URL，跳过此账号")
        return False

    # 步骤7: 换取token
    tokens = client.process_callback_url(callback_url)

    if tokens:
        refresh_token = tokens.get("refresh_token")
        access_token = tokens.get("access_token")

        if refresh_token:
            type_label = f" ({workspace_type})" if workspace_type else ""
            print(f"   🔐 RT{type_label}: {refresh_token[:40]}...")

            # 根据 workspace_type 决定保存到哪个字段
            panel_updated = False
            if workspace_type == "personal":
                # 保存到 Plus 字段
                update_data = {
                    "plus_access_token": access_token,
                    "plus_refresh_token": refresh_token,
                }
                if panel_client.update_account(account_id, update_data):
                    print(f"   ✅ Plus RT/AT 更新成功!")
                    panel_updated = True
            elif workspace_type == "team":
                # 保存到 Team 字段
                update_data = {
                    "team_access_token": access_token,
                    "team_refresh_token": refresh_token,
                }
                if panel_client.update_account(account_id, update_data):
                    print(f"   ✅ Team RT/AT 更新成功!")
                    panel_updated = True
            else:
                # 默认行为：更新主 RT 字段
                if panel_client.update_refresh_token(account_id, refresh_token):
                    print(f"   ✅ 线上 RT 更新成功!")
                    panel_updated = True

                # 同时更新 access_token
                if access_token:
                    account_info = extract_account_info(access_token)
                    update_data = {
                        "email": email,
                        "password": password,
                        "access_token": access_token,
                        "refresh_token": refresh_token,
                        "account_id": account_info.get("account_id", ""),
                        "status": "active",
                    }
                    if panel_client.update_account(account_id, update_data):
                        panel_updated = True

            # 获取RT成功后，清空绑卡状态（仅默认模式）
            if panel_updated and not workspace_type:
                panel_client.update_status(account_id, "active")

            # 保存到本地文件
            result_data = {
                "email": email,
                "account_id": account_id,
                "workspace_type": workspace_type or "default",
                "access_token": access_token,
                "refresh_token": refresh_token,
                "created_at": datetime.now().isoformat(),
            }
            with open("oauth_tokens.json", 'a', encoding='utf-8') as f:
                f.write(json.dumps(result_data, ensure_ascii=False) + '\n')

            return True
        else:
            print(f"   ⚠️ 未获取到 Refresh Token")
            return False
    else:
        print(f"   ❌ OAuth授权失败")
        return False


def login_account_dual_workspace(panel_client: PanelAPIClient, account: Dict) -> Tuple[bool, bool]:
    """对同一账号分别登录 Personal 和 Team workspace，保存两套 RT

    智能检测：
    1. 根据绑卡状态（plus_bound/team_bound）或订阅状态（is_plus/is_team）决定需要获取哪些 RT
    2. 跳过已存在的 RT，只获取缺失的

    Returns:
        Tuple[bool, bool]: (personal_success, team_success)
    """
    email = account.get("email")
    print(f"\n{'#'*60}")
    print(f"🔄 智能 Workspace 登录: {email}")
    print(f"{'#'*60}")

    # 检测绑卡状态（优先使用）和订阅状态（备用）
    plus_bound = bool(account.get("plus_bound"))
    team_bound = bool(account.get("team_bound"))
    is_plus = bool(account.get("is_plus"))
    is_team = bool(account.get("is_team"))

    # 综合判断：绑卡成功 或 订阅标记为 true 都视为已订阅
    has_plus_subscription = plus_bound or is_plus
    has_team_subscription = team_bound or is_team

    # 检测已有的 RT
    has_plus_rt = bool(account.get("plus_refresh_token"))
    has_team_rt = bool(account.get("team_refresh_token"))

    print(f"\n📋 绑卡/订阅状态检测:")
    print(f"   - Plus: 绑卡={'✅' if plus_bound else '❌'}, 订阅={'✅' if is_plus else '❌'} → {'✅ 需处理' if has_plus_subscription else '❌ 无订阅'}")
    print(f"   - Team: 绑卡={'✅' if team_bound else '❌'}, 订阅={'✅' if is_team else '❌'} → {'✅ 需处理' if has_team_subscription else '❌ 无订阅'}")

    print(f"\n📋 RT 状态检测:")
    print(f"   - Plus RT: {'✅ 已存在' if has_plus_rt else '❌ 缺失'}")
    print(f"   - Team RT: {'✅ 已存在' if has_team_rt else '❌ 缺失'}")

    # 根据绑卡/订阅状态和 RT 状态决定需要获取哪些
    # 只有绑卡成功或有订阅标记，且缺失 RT 时才需要获取
    need_personal = has_plus_subscription and not has_plus_rt
    need_team = has_team_subscription and not has_team_rt

    # 如果已有 RT 或未绑定订阅，视为成功（不需要处理）
    personal_success = has_plus_rt or not has_plus_subscription
    team_success = has_team_rt or not has_team_subscription

    # 显示需要获取的 RT
    if not need_personal and not need_team:
        if not has_plus_subscription and not has_team_subscription:
            print(f"\n⚠️ [{email}] 未绑卡且无订阅，跳过")
        else:
            reasons = []
            if has_plus_subscription:
                reasons.append("Plus RT 已存在" if has_plus_rt else "未绑定 Plus")
            if has_team_subscription:
                reasons.append("Team RT 已存在" if has_team_rt else "未绑定 Team")
            print(f"\n✅ [{email}] 无需获取 RT ({', '.join(reasons)})")
        return personal_success, team_success

    print(f"\n🎯 需要获取:")
    if need_personal:
        print(f"   - Plus RT（已绑卡/订阅，缺失 RT）")
    if need_team:
        print(f"   - Team RT（已绑卡/订阅，缺失 RT）")

    # 登录 Personal workspace（如果需要）
    if need_personal:
        print(f"\n--- 登录 Personal Workspace (获取 Plus RT) ---")
        personal_success = login_single_account(panel_client, account, workspace_type="personal")
        if personal_success:
            print(f"   ✅ Personal (Plus) RT 获取成功")
        else:
            print(f"   ⚠️ Personal workspace 登录失败或不存在")
    elif is_plus:
        print(f"\n--- Personal Workspace: 跳过（已有 Plus RT）---")

    # 等待一下再进行下一次登录（如果需要）
    if need_personal and need_team:
        time.sleep(2)

    # 登录 Team workspace（如果需要）
    if need_team:
        print(f"\n--- 登录 Team Workspace (获取 Team RT) ---")
        team_success = login_single_account(panel_client, account, workspace_type="team")
        if team_success:
            print(f"   ✅ Team RT 获取成功")
        else:
            print(f"   ⚠️ Team workspace 登录失败或不存在")
    elif is_team:
        print(f"\n--- Team Workspace: 跳过（已有 Team RT）---")

    # 汇总结果
    print(f"\n📊 [{email}] 登录结果:")
    if is_plus:
        status = "✅" if personal_success else "❌"
        note = " (已有)" if has_plus_rt else (" (新获取)" if personal_success else "")
        print(f"   - Plus: {status}{note}")
    else:
        print(f"   - Plus: ⏭️ 跳过（未绑定订阅）")

    if is_team:
        status = "✅" if team_success else "❌"
        note = " (已有)" if has_team_rt else (" (新获取)" if team_success else "")
        print(f"   - Team: {status}{note}")
    else:
        print(f"   - Team: ⏭️ 跳过（未绑定订阅）")

    return personal_success, team_success


def auto_login_from_panel():
    """从线上 Panel 获取已绑卡账号并自动登录获取 RT"""
    print("=" * 60)
    print("ChatGPT OAuth 自动登录 (仅从 Panel 获取已绑卡账号)")
    print("=" * 60)

    # 1. 连接 Panel API
    print("\n� 正在连接 Panel API...")
    panel_client = PanelAPIClient()
    if not panel_client.login():
        print("❌ 无法连接 Panel API")
        return

    # 2. 获取所有账号（自动分页）
    print("\n📥 正在获取所有账号...")
    accounts = panel_client.fetch_all_accounts(page_size=100)
    if not accounts:
        print("❌ 没有找到账号")
        return

    print(f"✅ 获取到全部 {len(accounts)} 个账号")

    # 3. 仅保留已绑卡账号
    selected_accounts = [acc for acc in accounts if is_bound_account(acc)]
    if not selected_accounts:
        print("❌ 未找到已绑卡账号")
        return
    print(f"✅ 已筛选到 {len(selected_accounts)} 个已绑卡账号，开始自动处理...")

    # 4. 批量处理选中的账号
    total = len(selected_accounts)
    success_count = 0
    failed_count = 0

    print(f"\n{'='*60}")
    print(f"📋 开始处理 {total} 个账号")
    print(f"{'='*60}")

    for i, account in enumerate(selected_accounts, 1):
        print(f"\n[{i}/{total}] 处理中...")

        if login_single_account(panel_client, account):
            success_count += 1
        else:
            failed_count += 1

        # 批量处理时增加延迟，避免请求过快
        if total > 1 and i < total:
            import time
            time.sleep(2)

    # 5. 输出统计结果
    print(f"\n{'='*60}")
    print(f"📊 批量处理完成")
    print(f"{'='*60}")
    print(f"   ✅ 成功: {success_count}")
    print(f"   ❌ 失败: {failed_count}")
    print(f"   📝 总计: {total}")
    print(f"{'='*60}")


def auto_login_dual_workspace_from_panel():
    """从 Panel 获取同时有 Plus 和 Team 的账号，分别登录两次获取两套 RT"""
    print("=" * 60)
    print("ChatGPT OAuth 双 Workspace 登录")
    print("同时获取 Personal (Plus) 和 Team 的 RT")
    print("=" * 60)

    # 1. 连接 Panel API
    print("\n🔌 正在连接 Panel API...")
    panel_client = PanelAPIClient()
    if not panel_client.login():
        print("❌ 无法连接 Panel API")
        return

    # 2. 获取所有账号（自动分页）
    print("\n📥 正在获取所有账号...")
    accounts = panel_client.fetch_all_accounts(page_size=100)
    if not accounts:
        print("❌ 没有找到账号")
        return

    print(f"✅ 获取到全部 {len(accounts)} 个账号")

    # 3. 筛选同时有 Plus 和 Team 的账号（is_plus=true 且 is_team=true）
    dual_accounts = [
        acc for acc in accounts
        if acc.get("is_plus") and acc.get("is_team")
    ]

    if not dual_accounts:
        print("❌ 未找到同时有 Plus 和 Team 订阅的账号")
        print("   提示: 需要 is_plus=true 且 is_team=true 的账号")
        return

    print(f"✅ 已筛选到 {len(dual_accounts)} 个双订阅账号")

    # 4. 初始化节点切换器
    print("\n🌐 初始化 ClashX 节点切换器...")
    proxy_switcher = ClashProxySwitcher()

    # 5. 批量处理
    total = len(dual_accounts)
    personal_success = 0
    team_success = 0

    for i, account in enumerate(dual_accounts, 1):
        # 检查是否需要切换节点（每 5 个账号切换一次）
        if proxy_switcher.should_switch(i):
            proxy_switcher.switch_next()
            time.sleep(2)  # 切换节点后等待 2 秒

        print(f"\n[{i}/{total}] 处理中...")
        p_ok, t_ok = login_account_dual_workspace(panel_client, account)
        if p_ok:
            personal_success += 1
        if t_ok:
            team_success += 1

        if i < total:
            time.sleep(3)

    # 6. 统计
    print(f"\n{'='*60}")
    print(f"📊 双 Workspace 登录完成")
    print(f"{'='*60}")
    print(f"   📝 总账号数: {total}")
    print(f"   ✅ Personal (Plus) 成功: {personal_success}")
    print(f"   ✅ Team 成功: {team_success}")
    print(f"{'='*60}")


def interactive_login():
    """交互式OAuth登录"""
    print("=" * 60)
    print("ChatGPT OAuth 协议登录")
    print("用于获取 refresh_token 进行API授权")
    print("=" * 60)

    client = ChatGPTOAuthClient()

    # 步骤1: 生成授权URL
    auth_url = client.step1_generate_auth_url()
    print(f"\n🔗 授权URL:\n{auth_url}\n")

    # 步骤2: 初始化会话
    if not client.step2_init_auth_session(auth_url):
        print("❌ 初始化会话失败")
        return

    # 步骤3: 输入邮箱
    email = input("\n📧 请输入邮箱: ").strip()
    if not email:
        print("❌ 邮箱不能为空")
        return

    success, result = client.step3_submit_email(email)
    if not success:
        if result == "not_registered":
            print("❌ 该邮箱未注册，请先注册账号")
        return

    # 步骤4: 输入密码
    password = input("\n🔑 请输入密码: ").strip()
    if not password:
        print("❌ 密码不能为空")
        return

    success, result = client.step4_submit_password(email, password)
    if not success:
        print("❌ 密码验证失败")
        return

    continue_url = result

    # 步骤5a: 如果需要验证码
    if result == "otp_required":
        code = input("\n🔢 请输入邮箱验证码: ").strip()
        if not code:
            print("❌ 验证码不能为空")
            return
        success, result = client.step5_submit_otp(code)
        if not success:
            print("❌ 验证码验证失败")
            return
        continue_url = result

    # 步骤5b: 选择workspace (点击继续按钮)
    if result == "workspace_select" or continue_url == "workspace_select":
        success, continue_url = client.step5_select_workspace()
        if not success:
            print("❌ Workspace选择失败")
            return

    # 步骤6: 处理consent页面 / 获取回调URL
    callback_url = None
    if continue_url and continue_url.startswith("http"):
        callback_url = client.step6_handle_consent(continue_url)

    # 如果自动处理失败，提示手动输入回调URL
    if not callback_url:
        print("\n" + "=" * 60)
        print("⚠️ 无法自动获取回调URL")
        print("请在浏览器中完成以下步骤:")
        print("1. 打开授权URL (上面已打印)")
        print("2. 完成登录和授权")
        print("3. 在consent页面点击继续")
        print("4. 复制浏览器地址栏中的完整回调URL")
        print("   (格式: http://localhost:1455/auth/callback?code=...&state=...)")
        print("=" * 60)

        callback_url = input("\n📋 请粘贴回调URL: ").strip()
        if not callback_url:
            print("❌ 回调URL不能为空")
            return

    # 步骤7: 换取token
    tokens = client.process_callback_url(callback_url)

    if tokens:
        print("\n" + "=" * 60)
        print("✅ OAuth授权成功!")
        print("=" * 60)

        # 保存结果
        result_data = {
            "email": email,
            "access_token": tokens.get("access_token"),
            "refresh_token": tokens.get("refresh_token"),
            "id_token": tokens.get("id_token"),
            "expires_in": tokens.get("expires_in"),
            "token_type": tokens.get("token_type"),
            "created_at": datetime.now().isoformat(),
        }

        # 保存到文件
        filename = "oauth_tokens.json"
        with open(filename, 'a', encoding='utf-8') as f:
            f.write(json.dumps(result_data, ensure_ascii=False) + '\n')
        print(f"\n💾 Token已保存到 {filename}")

        # 显示refresh_token
        if tokens.get("refresh_token"):
            print(f"\n🔐 Refresh Token (完整):")
            print(tokens.get("refresh_token"))

        # 自动导入到线上项目
        print("\n" + "-" * 60)
        import_to_panel(email, password, tokens)
    else:
        print("\n❌ OAuth授权失败")


def process_callback_only():
    """仅处理回调URL模式 - 用于已有授权链接的情况"""
    print("=" * 60)
    print("ChatGPT OAuth 回调处理模式")
    print("用于处理已完成登录的回调URL")
    print("=" * 60)

    # 输入PKCE verifier (如果有)
    code_verifier = input("\n🔑 请输入 code_verifier (如果没有直接回车): ").strip()

    # 输入回调URL
    callback_url = input("\n📋 请粘贴回调URL: ").strip()
    if not callback_url:
        print("❌ 回调URL不能为空")
        return

    # 解析回调URL
    parsed = urlparse(callback_url)
    params = parse_qs(parsed.query)
    code = params.get('code', [None])[0]

    if not code:
        print("❌ 回调URL中没有code参数")
        return

    print(f"\n   Code: {code[:50]}...")

    # 如果没有verifier，生成一个新的 (可能会失败)
    if not code_verifier:
        print("\n⚠️ 没有code_verifier，token换取可能会失败")
        print("   (PKCE要求code_verifier与生成授权URL时的一致)")
        proceed = input("   是否继续? (y/n): ").strip().lower()
        if proceed != 'y':
            return
        code_verifier = PKCE.generate_code_verifier()

    # 换取token
    client = ChatGPTOAuthClient()
    client.code_verifier = code_verifier
    tokens = client.step7_exchange_code(code)

    if tokens:
        print("\n" + "=" * 60)
        print("✅ Token获取成功!")
        print("=" * 60)
        if tokens.get("refresh_token"):
            print(f"\n🔐 Refresh Token:")
            print(tokens.get("refresh_token"))
    else:
        print("\n❌ Token换取失败")


def login_by_email(email: str, workspace_type: str = None, dual_mode: bool = False):
    """通过指定邮箱登录

    Args:
        email: 账号邮箱
        workspace_type: workspace 类型 ("personal" 或 "team")
        dual_mode: 是否双 workspace 模式
    """
    print("=" * 60)
    print(f"ChatGPT OAuth 指定账号登录: {email}")
    print("=" * 60)

    # 连接 Panel API
    print("\n🔌 正在连接 Panel API...")
    panel_client = PanelAPIClient()
    if not panel_client.login():
        print("❌ 无法连接 Panel API")
        return

    # 从 Panel 获取该账号信息（自动分页查找）
    print(f"\n📥 正在查找账号: {email}")
    accounts = panel_client.fetch_all_accounts(page_size=100)
    if not accounts:
        print("❌ 获取账号列表失败")
        return

    # 查找匹配的账号
    target_account = None
    for acc in accounts:
        if acc.get("email", "").lower() == email.lower():
            target_account = acc
            break

    if not target_account:
        print(f"❌ 未找到账号: {email}")
        return

    print(f"✅ 找到账号: {email} (ID: {target_account.get('id')})")

    # 根据模式登录
    if dual_mode:
        print("\n🔄 使用双 Workspace 模式...")
        login_account_dual_workspace(panel_client, target_account)
    elif workspace_type:
        print(f"\n🔄 使用 {workspace_type} Workspace 模式...")
        login_single_account(panel_client, target_account, workspace_type=workspace_type)
    else:
        print("\n🔄 使用默认模式...")
        login_single_account(panel_client, target_account)


def auto_refresh_dual_rt_from_panel(workers: int = None):
    """对所有已绑卡或已有RT的账号重新获取双 RT（Personal + Team）

    Args:
        workers: 并发线程数，默认使用 Config.DEFAULT_WORKERS
    """
    workers = workers or Config.DEFAULT_WORKERS

    print("=" * 60)
    print("ChatGPT OAuth 批量刷新双 RT（多线程版）")
    print("对所有已绑卡或已有RT的账号重新获取 Personal (Plus) 和 Team 的 RT")
    print(f"并发线程数: {workers}")
    print("=" * 60)

    # 1. 连接 Panel API
    print("\n🔌 正在连接 Panel API...")
    panel_client = PanelAPIClient()
    if not panel_client.login():
        print("❌ 无法连接 Panel API")
        return

    # 2. 获取所有账号
    print("\n📥 正在获取所有账号...")
    accounts = panel_client.fetch_all_accounts(page_size=100)
    if not accounts:
        print("❌ 没有找到账号")
        return

    print(f"✅ 获取到全部 {len(accounts)} 个账号")

    # 3. 筛选需要处理的账号
    def has_dual_rt(acc: Dict) -> bool:
        """检查是否已有双 RT"""
        return bool(acc.get("plus_refresh_token")) and bool(acc.get("team_refresh_token"))

    def should_process(acc: Dict) -> bool:
        """判断账号是否需要处理"""
        # 已有双 RT 的跳过
        if has_dual_rt(acc):
            return False
        # 已绑卡
        if is_bound_account(acc):
            return True
        # 已有任意 RT
        if acc.get("refresh_token"):
            return True
        if acc.get("plus_refresh_token"):
            return True
        if acc.get("team_refresh_token"):
            return True
        return False

    # 统计
    all_eligible = [acc for acc in accounts if is_bound_account(acc) or acc.get("refresh_token") or acc.get("plus_refresh_token") or acc.get("team_refresh_token")]
    already_has_dual = [acc for acc in all_eligible if has_dual_rt(acc)]
    eligible_accounts = [acc for acc in accounts if should_process(acc)]

    if not eligible_accounts:
        print(f"❌ 未找到需要处理的账号")
        print(f"   - 符合条件的账号: {len(all_eligible)} 个")
        print(f"   - 已有双RT（跳过）: {len(already_has_dual)} 个")
        return

    # 统计
    bound_count = sum(1 for acc in eligible_accounts if is_bound_account(acc))
    has_any_rt_count = sum(1 for acc in eligible_accounts if acc.get("refresh_token") or acc.get("plus_refresh_token") or acc.get("team_refresh_token"))
    print(f"✅ 已筛选到 {len(eligible_accounts)} 个需要处理的账号")
    print(f"   - 已绑卡: {bound_count} 个")
    print(f"   - 已有部分RT: {has_any_rt_count} 个")
    print(f"   - 已有双RT（跳过）: {len(already_has_dual)} 个")

    # 4. 初始化节点切换器
    print("\n🌐 初始化 ClashX 节点切换器...")
    proxy_switcher = ClashProxySwitcher()

    # 5. 线程安全的统计计数器
    total = len(eligible_accounts)
    stats_lock = threading.Lock()
    stats = {
        "personal_success": 0,
        "personal_fail": 0,
        "personal_skip": 0,
        "team_success": 0,
        "team_fail": 0,
        "team_skip": 0,
        "processed": 0,
    }
    failed_accounts = []  # 记录失败的账号

    def process_account(account: Dict) -> Tuple[str, bool, bool]:
        """处理单个账号（线程工作函数）"""
        email = account.get("email", "unknown")

        # 检查并切换节点（线程安全）
        proxy_switcher.check_and_switch()

        # 获取当前进度
        with stats_lock:
            stats["processed"] += 1
            current = stats["processed"]

        print(f"\n[{current}/{total}] 🔄 {email}")

        try:
            p_ok, t_ok = login_account_dual_workspace(panel_client, account)
            return email, p_ok, t_ok
        except Exception as e:
            print(f"   ❌ 异常: {e}")
            return email, False, False

    # 6. 使用线程池并发处理
    print(f"\n🚀 开始并发处理 ({workers} 线程)...")

    with ThreadPoolExecutor(max_workers=workers) as executor:
        # 提交所有任务
        future_to_account = {
            executor.submit(process_account, acc): acc
            for acc in eligible_accounts
        }

        # 收集结果
        for future in as_completed(future_to_account):
            account = future_to_account[future]
            email = account.get("email", "unknown")

            try:
                _, p_ok, t_ok = future.result()
            except Exception as e:
                print(f"   ❌ {email} 任务异常: {e}")
                p_ok, t_ok = False, False

            # 获取账号信息
            plus_bound = bool(account.get("plus_bound"))
            team_bound = bool(account.get("team_bound"))
            is_plus = bool(account.get("is_plus"))
            is_team = bool(account.get("is_team"))
            has_plus_subscription = plus_bound or is_plus
            has_team_subscription = team_bound or is_team
            has_plus_rt = bool(account.get("plus_refresh_token"))
            has_team_rt = bool(account.get("team_refresh_token"))

            # 线程安全地更新统计
            with stats_lock:
                # 统计 Personal/Plus RT
                if has_plus_subscription:
                    if has_plus_rt or p_ok:
                        stats["personal_success"] += 1
                    else:
                        stats["personal_fail"] += 1
                        if not any(email == fa[0] for fa in failed_accounts):
                            failed_accounts.append((email, "Plus RT 获取失败"))
                else:
                    stats["personal_skip"] += 1

                # 统计 Team RT
                if has_team_subscription:
                    if has_team_rt or t_ok:
                        stats["team_success"] += 1
                    else:
                        stats["team_fail"] += 1
                        if not any(email == fa[0] for fa in failed_accounts):
                            failed_accounts.append((email, "Team RT 获取失败"))
                else:
                    stats["team_skip"] += 1

    # 7. 输出统计结果
    print("\n" + "=" * 60)
    print("📊 批量刷新双 RT 完成（多线程）")
    print("=" * 60)
    print(f"   总计处理: {total} 个账号")
    print(f"   并发线程: {workers}")
    print(f"\n   📋 Plus RT 统计:")
    print(f"      - 成功: {stats['personal_success']}")
    print(f"      - 失败: {stats['personal_fail']}")
    print(f"      - 跳过(无订阅): {stats['personal_skip']}")
    print(f"\n   📋 Team RT 统计:")
    print(f"      - 成功: {stats['team_success']}")
    print(f"      - 失败: {stats['team_fail']}")
    print(f"      - 跳过(无订阅): {stats['team_skip']}")

    if failed_accounts:
        print(f"\n   ❌ 失败账号列表 ({len(failed_accounts)} 个):")
        for email, reason in failed_accounts[:10]:  # 只显示前10个
            print(f"      - {email}: {reason}")
        if len(failed_accounts) > 10:
            print(f"      ... 还有 {len(failed_accounts) - 10} 个未显示")

    print("=" * 60)

    # 8. 发送 Bark 通知
    bark_lines = [
        "✅ 批量刷新双 RT 完成",
        f"总计: {total} 个账号 ({workers}线程)",
        "",
        f"Plus RT: 成功 {stats['personal_success']} / 失败 {stats['personal_fail']}",
        f"Team RT: 成功 {stats['team_success']} / 失败 {stats['team_fail']}",
    ]
    if failed_accounts:
        bark_lines.append(f"\n❌ 失败: {len(failed_accounts)} 个账号")
    send_bark_message("\n".join(bark_lines))


def main():
    """主函数：支持多种登录模式"""
    import argparse

    parser = argparse.ArgumentParser(description="ChatGPT OAuth 登录工具")
    parser.add_argument("--email", type=str,
                       help="指定账号邮箱")
    parser.add_argument("--dual", action="store_true",
                       help="双 Workspace 模式：同时获取 Personal 和 Team 的 RT（仅处理已有双订阅的账号）")
    parser.add_argument("--refresh-dual", action="store_true",
                       help="刷新双 RT：对所有已绑卡账号重新获取 Personal 和 Team 的 RT")
    parser.add_argument("--workspace", choices=["personal", "team"],
                       help="指定 Workspace 类型 (personal 或 team)")
    parser.add_argument("--workers", type=int, default=Config.DEFAULT_WORKERS,
                       help=f"并发线程数 (默认: {Config.DEFAULT_WORKERS})")
    args = parser.parse_args()

    if Config.USE_BASH_LAUNCHER and os.getenv("OAUTH_LAUNCHED") != "1":
        script_path = os.path.join(os.path.dirname(__file__), Config.BASH_LAUNCHER_PATH)
        if os.path.exists(script_path):
            env = os.environ.copy()
            env["OAUTH_LAUNCHED"] = "1"
            # 传递命令行参数
            cmd = ["bash", script_path]
            if args.email:
                cmd.extend(["--email", args.email])
            if args.dual:
                cmd.append("--dual")
            if args.refresh_dual:
                cmd.append("--refresh-dual")
            if args.workspace:
                cmd.extend(["--workspace", args.workspace])
            if args.workers != Config.DEFAULT_WORKERS:
                cmd.extend(["--workers", str(args.workers)])
            try:
                subprocess.run(cmd, check=True, env=env)
                return
            except Exception as e:
                print(f"⚠️ 启动脚本失败，改用直接运行: {e}")
        else:
            print(f"⚠️ 未找到启动脚本: {script_path}，改用直接运行")

    # 如果指定了邮箱，使用指定账号登录
    if args.email:
        login_by_email(args.email, workspace_type=args.workspace, dual_mode=args.dual)
    elif args.refresh_dual:
        print("🔄 刷新双 RT 模式（所有已绑卡账号）...")
        auto_refresh_dual_rt_from_panel(workers=args.workers)
    elif args.dual:
        print("🔄 使用双 Workspace 模式（仅双订阅账号）...")
        auto_login_dual_workspace_from_panel()
    else:
        auto_login_from_panel()


if __name__ == "__main__":
    main()
