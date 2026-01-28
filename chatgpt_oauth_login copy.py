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
from datetime import datetime
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Dict, Optional, Tuple
from urllib.parse import urlencode, urlparse, parse_qs
import pybase64

from curl_cffi import requests

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

    def step5_select_workspace(self, workspace_id: str = None) -> Tuple[bool, str]:
        """步骤5: 选择workspace (点击继续按钮)"""
        print(f"\n📍 步骤5: 选择Workspace (同意授权)")
        try:
            self._delay()

            # 如果没有提供workspace_id，尝试从cookies中获取
            if not workspace_id:
                workspace_id = self._get_workspace_id_from_cookies()

            if not workspace_id:
                print("❌ 无法获取workspace_id")
                return False, "error"

            print(f"   Workspace ID: {workspace_id}")

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

    def _get_workspace_id_from_cookies(self) -> Optional[str]:
        """从cookies中解析workspace_id"""
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
                if workspaces:
                    # 返回第一个workspace的id
                    return workspaces[0].get('id')
        except Exception as e:
            print(f"   解析workspace失败: {e}")
        return None



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
                continue_url = result.get('continue_url', '')
                if continue_url:
                    print(f"✅ 验证码验证成功")
                    return True, continue_url
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
                if 'callback' in resp.url and 'code=' in resp.url:
                    print(f"✅ 获取到回调URL")
                    return resp.url

                # 检查响应内容是否有下一步URL
                try:
                    result = resp.json()
                    continue_url = result.get('continue_url', '')
                    if continue_url:
                        print(f"   发现continue_url，继续处理...")
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


def main():
    """主函数"""
    print("\n请选择模式:")
    print("1. 交互式OAuth登录 (完整流程)")
    print("2. 仅处理回调URL (已有回调链接)")
    print("3. 生成授权URL (仅生成链接)")

    choice = input("\n请输入选项 (1/2/3): ").strip()

    if choice == "1":
        interactive_login()
    elif choice == "2":
        process_callback_only()
    elif choice == "3":
        client = ChatGPTOAuthClient()
        auth_url = client.step1_generate_auth_url()
        print(f"\n🔗 授权URL:\n{auth_url}")
        print(f"\n🔑 Code Verifier (保存好，换取token时需要):")
        print(client.code_verifier)
        print(f"\n📋 State:")
        print(client.state)
    else:
        print("❌ 无效的选项")


if __name__ == "__main__":
    main()