#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
ChatGPT 纯协议注册机
基于HAR文件逆向分析实现，使用curl_cffi绕过Cloudflare
"""

import json
import random
import re
import string
import time
import uuid
import pybase64
import threading
import os
import base64
from datetime import datetime, timezone, timedelta
from typing import Dict, List, Optional, Tuple
from urllib.parse import urlencode, urlparse, parse_qs

from curl_cffi import requests


# ============================================================================
# JWT 解析工具
# ============================================================================
def decode_jwt_payload(token: str) -> Optional[Dict]:
    """解码 JWT token 的 payload 部分（不验证签名）"""
    if not token:
        return None
    try:
        parts = token.split(".")
        if len(parts) != 3:
            return None
        # 解码 payload 部分
        payload_b64 = parts[1]
        # 添加 padding
        padding = (4 - len(payload_b64) % 4) % 4
        payload_b64 += "=" * padding
        payload_bytes = base64.urlsafe_b64decode(payload_b64)
        return json.loads(payload_bytes.decode("utf-8"))
    except Exception:
        return None


def extract_subscription_from_token(access_token: str) -> str:
    """从 access_token 中提取订阅状态"""
    payload = decode_jwt_payload(access_token)
    if not payload:
        return "free"

    # 尝试从 chatgpt_plan_type 字段获取
    plan_type = payload.get("chatgpt_plan_type")
    if plan_type:
        return normalize_subscription_status(plan_type)

    # 尝试从 https://api.openai.com/auth 字段获取
    auth_info = payload.get("https://api.openai.com/auth", {})
    if isinstance(auth_info, dict):
        plan_type = auth_info.get("chatgpt_plan_type") or auth_info.get("plan_type")
        if plan_type:
            return normalize_subscription_status(plan_type)

    return "free"


def normalize_subscription_status(raw: str) -> str:
    """标准化订阅状态"""
    if not raw:
        return "free"
    value = raw.lower().strip()
    if value == "chatgptteamplan":
        return "team"
    if value in ("free", "plus", "team", "business", "pro"):
        return value
    return value

# ============================================================================
# 配置
# ============================================================================
class Config:
    """配置类"""
    # 代理
    PROXY = "http://127.0.0.1:7890"

    # 邮箱API (mail.chatgpt.org.uk)
    MAIL_API_BASE = "https://mail.chatgpt.org.uk/api"

    # 默认密码
    DEFAULT_PASSWORD = "testuser1314"

    # ChatGPT相关URL
    CHATGPT_BASE = "https://chatgpt.com"
    AUTH_BASE = "https://auth.openai.com"
    SENTINEL_BASE = "https://sentinel.openai.com/backend-api/sentinel"

    # CLIENT_ID
    CLIENT_ID = "app_X8zY6vW2pQ9tR3dE7nK1jL5gH"

    # 请求超时
    TIMEOUT = 30

    # 面板导入（Zeabur 部署地址）
    PANEL_BASE = "xxxx"
    PANEL_USERNAME = ""
    PANEL_PASSWORD = ""
    PANEL_IMPORT_ENABLED = True

    # 浏览器指纹
    IMPERSONATE = "chrome120"
    USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
    SEC_CH_UA = '"Google Chrome";v="131", "Chromium";v="131", "Not_A Brand";v="24"'


# ============================================================================
# 工具类
# ============================================================================
class Utils:
    """工具类"""

    FIRST_NAMES = []
    LAST_NAMES = []
    _names_loaded = False

    @staticmethod
    def is_valid_name(name: str) -> bool:
        """检查名字是否合法（只包含英文字母，长度3-15）"""
        if not name or len(name) < 3 or len(name) > 15:
            return False
        # 只允许纯英文字母
        return name.isalpha() and name.isascii()

    @classmethod
    def load_names(cls):
        """从文件加载名字列表"""
        if cls._names_loaded:
            return

        # 获取脚本所在目录
        script_dir = os.path.dirname(os.path.abspath(__file__))

        # 尝试多个路径加载first-names.txt
        first_names_paths = [
            os.path.join(script_dir, 'first-names.txt'),
            os.path.join(script_dir, 'zhuceji_api', 'first-names.txt'),
            'first-names.txt',
        ]

        for path in first_names_paths:
            if os.path.exists(path):
                with open(path, 'r', encoding='utf-8') as f:
                    raw_names = [line.strip() for line in f if line.strip()]
                    # 过滤掉不合法的名字
                    cls.FIRST_NAMES = [n for n in raw_names if cls.is_valid_name(n)]
                print(f"✅ 加载 first-names.txt: {len(cls.FIRST_NAMES)} 个有效名字 (原始 {len(raw_names)} 个)")
                break

        if not cls.FIRST_NAMES:
            cls.FIRST_NAMES = ["James", "John", "Robert", "Michael", "William", "David",
                               "Richard", "Joseph", "Thomas", "Charles", "Mary", "Patricia",
                               "Jennifer", "Linda", "Elizabeth", "Emma", "Olivia", "Sophia"]
            print("⚠️ 未找到 first-names.txt，使用默认名字列表")

        # 尝试多个路径加载last-names.txt
        last_names_paths = [
            os.path.join(script_dir, 'last-names.txt'),
            os.path.join(script_dir, 'zhuceji_api', 'last-names.txt'),
            'last-names.txt',
        ]

        for path in last_names_paths:
            if os.path.exists(path):
                with open(path, 'r', encoding='utf-8') as f:
                    raw_names = [line.strip() for line in f if line.strip()]
                    # 过滤掉不合法的姓氏
                    cls.LAST_NAMES = [n for n in raw_names if cls.is_valid_name(n)]
                print(f"✅ 加载 last-names.txt: {len(cls.LAST_NAMES)} 个有效姓氏 (原始 {len(raw_names)} 个)")
                break

        if not cls.LAST_NAMES:
            cls.LAST_NAMES = ["Smith", "Johnson", "Williams", "Brown", "Jones", "Garcia",
                              "Miller", "Davis", "Rodriguez", "Martinez", "Wilson", "Anderson"]
            print("⚠️ 未找到 last-names.txt，使用默认姓氏列表")

        cls._names_loaded = True

    @staticmethod
    def generate_device_id() -> str:
        return str(uuid.uuid4())

    @staticmethod
    def generate_name() -> Dict[str, str]:
        Utils.load_names()
        first = random.choice(Utils.FIRST_NAMES).capitalize()
        last = random.choice(Utils.LAST_NAMES).capitalize()
        return {"firstName": first, "lastName": last, "fullName": f"{first} {last}"}

    @staticmethod
    def generate_email_prefix(name_info: Dict[str, str]) -> str:
        """生成邮箱前缀，不添加任何数字，只使用合法字符"""
        # 清理名字，只保留英文字母
        first = ''.join(c for c in name_info['firstName'].lower() if c.isalpha() and c.isascii())
        last = ''.join(c for c in name_info['lastName'].lower() if c.isalpha() and c.isascii())

        # 确保名字不为空
        if not first:
            first = "user"
        if not last:
            last = "name"

        # 随机选择格式，不添加数字
        formats = [f"{first}", f"{first}.{last}", f"{first}_{last}", f"{first}{last}"]
        return random.choice(formats)

    @staticmethod
    def generate_password(length: int = 14) -> str:
        chars = string.ascii_letters + string.digits
        password = list(random.choices(chars, k=length))
        password[0] = random.choice(string.ascii_uppercase)
        password[-1] = random.choice(string.digits)
        return ''.join(password)

    @staticmethod
    def generate_birthday() -> str:
        year = datetime.now().year - random.randint(18, 50)
        month = random.randint(1, 12)
        day = random.randint(1, 28)
        return f"{year:04d}-{month:02d}-{day:02d}"


# ============================================================================
# Sentinel Token 生成器
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
        now = datetime.now(timezone(timedelta(hours=8)))
        date_str = now.strftime("%a %b %d %Y %H:%M:%S") + " GMT+0800 (中国标准时间)"
        
        navigator_props = [
            "mediaCapabilities−[object MediaCapabilities]",
            "permissions−[object Permissions]",
            "storage−[object StorageManager]",
            "cookieEnabled−true",
            "language−zh-CN",
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
            "zh-CN",
            "zh-CN,zh",
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
            "Accept-Language": "zh-CN,zh;q=0.9",
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
# 邮箱API客户端 (mail.chatgpt.org.uk)
# ============================================================================
class MailClient:
    """临时邮箱API客户端 - 使用 mail.chatgpt.org.uk"""

    def __init__(self, username: str = None, password: str = None):
        """初始化邮箱客户端（无需登录）"""
        self.session = requests.Session(impersonate=Config.IMPERSONATE, proxy=Config.PROXY)
        self.headers = {
            "User-Agent": "Mozilla/5.0",
            "Origin": "https://mail.chatgpt.org.uk",
            "Referer": "https://mail.chatgpt.org.uk/"
        }
        self.domains: List[str] = ["chatgpt.org.uk"]  # 默认域名
        self.current_email: Optional[str] = None

    def login(self) -> bool:
        """无需登录，直接返回成功"""
        print(f"✅ 邮箱API就绪 (mail.chatgpt.org.uk)")
        return True

    def get_domains(self) -> List[str]:
        """返回可用域名"""
        print(f"✅ 获取到 {len(self.domains)} 个可用域名")
        return self.domains

    def create_email(self, prefix: str = None, domain_index: int = 0) -> Optional[str]:
        """从API获取新的临时邮箱地址"""
        try:
            resp = self.session.get(
                f"{Config.MAIL_API_BASE}/generate-email",
                headers={**self.headers, "content-type": "application/json"},
                timeout=Config.TIMEOUT
            )
            if resp.status_code == 200:
                result = resp.json()
                if result.get('success') and result.get('data', {}).get('email'):
                    email = result['data']['email']
                    self.current_email = email
                    print(f"✅ 获取邮箱: {email}")
                    return email
            print(f"❌ 获取邮箱失败: {resp.status_code} - {resp.text[:100]}")
            return None
        except Exception as e:
            print(f"❌ 获取邮箱异常: {e}")
            return None

    def _fetch_messages(self, email: str) -> List[dict]:
        """获取邮箱中的邮件列表"""
        try:
            resp = self.session.get(
                f"{Config.MAIL_API_BASE}/emails",
                params={"email": email},
                headers={**self.headers, "cache-control": "no-cache"},
                timeout=Config.TIMEOUT
            )
            if resp.status_code == 200:
                result = resp.json()
                if result.get('success') and result.get('data', {}).get('emails'):
                    return result['data']['emails']
            return []
        except Exception as e:
            print(f"⚠️ 获取邮件列表出错: {e}")
            return []

    def get_verification_code(self, email: str, max_attempts: int = 60, interval: int = 3) -> Optional[str]:
        """等待验证码邮件"""
        print(f"⏳ 等待验证码 (最多 {max_attempts * interval}s)...")
        # 支持两种验证码格式: XXX-XXX 或 6位数字
        code_regex = re.compile(r'\b[A-Z0-9]{3}-[A-Z0-9]{3}\b|\b\d{6}\b')
        checked_msg_ids = set()

        for attempt in range(max_attempts):
            try:
                msgs = self._fetch_messages(email)
                if msgs:
                    for msg in msgs:
                        msg_id = msg.get('id') or msg.get('subject', '') + str(msg.get('date', ''))
                        if msg_id in checked_msg_ids:
                            continue
                        checked_msg_ids.add(msg_id)

                        # 合并所有可能的内容
                        content = " ".join([
                            str(msg.get('subject') or ''),
                            str(msg.get('html_content') or ''),
                            str(msg.get('text_content') or ''),
                            str(msg.get('body') or ''),
                            str(msg.get('content') or ''),
                        ])

                        matches = code_regex.findall(content)
                        if matches:
                            # 取第一个匹配的验证码，去除连字符
                            code = matches[0].replace('-', '')
                            print(f"✅ 获取到验证码: {code}")
                            return code
            except Exception as e:
                print(f"⚠️ 获取邮件异常: {e}")

            print(f"⏳ 等待验证码... ({attempt + 1}/{max_attempts})")
            time.sleep(interval)

        print(f"❌ 获取验证码超时")
        return None


# ============================================================================
# ChatGPT注册客户端
# ============================================================================
class ChatGPTRegisterClient:
    """ChatGPT注册客户端 - 使用curl_cffi"""
    
    def __init__(self):
        self.session = requests.Session(impersonate=Config.IMPERSONATE, proxy=Config.PROXY)
        self.device_id = Utils.generate_device_id()
        self.csrf_token: Optional[str] = None
        self.state: Optional[str] = None
        self.sentinel_generator = SentinelTokenGenerator(self.device_id, self.session)
        
    def _delay(self, min_sec: float = 0.5, max_sec: float = 1.5):
        time.sleep(random.uniform(min_sec, max_sec))
    
    def _get_api_headers(self, referer: str, with_sentinel: bool = False, flow: str = "authorize_continue") -> Dict:
        """获取API请求头"""
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
    
    def step1_init_session(self) -> bool:
        """步骤1: 初始化会话，获取Cloudflare和ChatGPT Cookie"""
        print("\n📍 步骤1: 初始化会话")
        try:
            resp = self.session.get(Config.CHATGPT_BASE, timeout=Config.TIMEOUT)
            if resp.status_code == 200:
                cookies = list(self.session.cookies.keys())
                print(f"✅ 初始化成功，Cookies: {cookies}")
                return True
            print(f"❌ 初始化失败: {resp.status_code}")
            return False
        except Exception as e:
            print(f"❌ 初始化异常: {e}")
            return False
    
    def step2_get_csrf(self) -> bool:
        """步骤2: 获取CSRF Token"""
        print("\n📍 步骤2: 获取CSRF Token")
        try:
            resp = self.session.get(
                f"{Config.CHATGPT_BASE}/api/auth/csrf",
                headers={"Content-Type": "application/json"},
                timeout=Config.TIMEOUT
            )
            if resp.status_code == 200:
                self.csrf_token = resp.json().get('csrfToken')
                print(f"✅ CSRF Token: {self.csrf_token[:30]}...")
                return True
            print(f"❌ 获取CSRF失败: {resp.status_code}")
            return False
        except Exception as e:
            print(f"❌ 获取CSRF异常: {e}")
            return False
    
    def step3_start_oauth(self) -> Optional[str]:
        """步骤3: 发起OAuth，获取授权URL"""
        print("\n📍 步骤3: 发起OAuth")
        try:
            self._delay()
            resp = self.session.post(
                f"{Config.CHATGPT_BASE}/api/auth/signin/openai",
                params={
                    "prompt": "login",
                    "screen_hint": "login_or_signup",
                    "ext-oai-did": self.device_id,
                    "auth_session_logging_id": str(uuid.uuid4())
                },
                data=f"callbackUrl={Config.CHATGPT_BASE}/&csrfToken={self.csrf_token}&json=true",
                headers={"Content-Type": "application/x-www-form-urlencoded"},
                timeout=Config.TIMEOUT
            )
            if resp.status_code == 200:
                auth_url = resp.json().get('url')
                print(f"✅ 授权URL获取成功")
                return auth_url
            print(f"❌ OAuth失败: {resp.status_code}")
            return None
        except Exception as e:
            print(f"❌ OAuth异常: {e}")
            return None


    def step4_authorize(self, auth_url: str) -> bool:
        """步骤4: 访问授权URL，获取auth会话Cookie"""
        print("\n📍 步骤4: 访问授权URL")
        try:
            # 解析state
            parsed = urlparse(auth_url)
            params = parse_qs(parsed.query)
            self.state = params.get('state', [None])[0]
            
            # 先访问auth.openai.com首页建立CF会话
            print("   建立auth.openai.com会话...")
            self.session.get(f"{Config.AUTH_BASE}/", timeout=Config.TIMEOUT)
            
            self._delay()
            
            # 访问授权URL
            resp = self.session.get(auth_url, timeout=Config.TIMEOUT, allow_redirects=True)
            print(f"   响应状态: {resp.status_code}")
            print(f"   最终URL: {resp.url}")
            
            # 检查关键cookie
            cookies = list(self.session.cookies.keys())
            print(f"   Cookies: {cookies}")
            
            # 检查是否成功进入登录页面
            if resp.status_code == 200:
                # 检查是否有auth相关cookie或者页面内容
                if 'log-in' in resp.url or 'login' in resp.url or resp.status_code == 200:
                    print(f"✅ 授权会话建立成功")
                    return True
            
            print(f"⚠️ 授权可能失败，状态码: {resp.status_code}")
            return False
        except Exception as e:
            print(f"❌ 授权异常: {e}")
            return False
    
    def step5_submit_email(self, email: str) -> bool:
        """步骤5: 提交邮箱"""
        print(f"\n📍 步骤5: 提交邮箱 ({email})")
        try:
            self._delay()
            headers = self._get_api_headers(
                referer=f"{Config.AUTH_BASE}/log-in-or-create-account",
                with_sentinel=True,
                flow="authorize_continue"
            )
            
            resp = self.session.post(
                f"{Config.AUTH_BASE}/api/accounts/authorize/continue",
                json={
                    "username": {"value": email, "kind": "email"},
                    "screen_hint": "login_or_signup"
                },
                headers=headers,
                timeout=Config.TIMEOUT
            )
            
            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                result = resp.json()
                page_type = result.get('page', {}).get('type', '')
                print(f"   页面类型: {page_type}")
                
                if page_type == 'create_account_password':
                    print(f"✅ 新用户，进入密码设置")
                    return True
                elif 'login' in page_type:
                    print(f"⚠️ 邮箱已注册")
                    return False
                    
            print(f"❌ 提交邮箱失败: {resp.status_code} - {resp.text[:200]}")
            return False
        except Exception as e:
            print(f"❌ 提交邮箱异常: {e}")
            return False
    
    def step6_register(self, email: str, password: str) -> bool:
        """步骤6: 提交密码注册"""
        print(f"\n📍 步骤6: 提交密码注册")
        try:
            self._delay()
            headers = self._get_api_headers(
                referer=f"{Config.AUTH_BASE}/create-account/password",
                with_sentinel=True,
                flow="user_register"
            )
            
            resp = self.session.post(
                f"{Config.AUTH_BASE}/api/accounts/user/register",
                json={"password": password, "username": email},
                headers=headers,
                timeout=Config.TIMEOUT
            )
            
            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                try:
                    result = resp.json()
                    print(f"   响应内容: {result}")
                    continue_url = result.get('continue_url', '')
                    # 兼容多种 URL 格式
                    if 'email-otp' in continue_url or 'email-verification' in continue_url:
                        print(f"✅ 注册成功，等待邮箱验证")
                        return True
                except:
                    print(f"   响应内容: {resp.text[:200]}")
                    
            print(f"❌ 注册失败: {resp.status_code}")
            return False
        except Exception as e:
            print(f"❌ 注册异常: {e}")
            return False


    def step7_send_otp(self) -> bool:
        """步骤7: 发送验证码"""
        print(f"\n📍 步骤7: 发送验证码")
        try:
            resp = self.session.get(
                f"{Config.AUTH_BASE}/api/accounts/email-otp/send",
                headers={
                    "Referer": f"{Config.AUTH_BASE}/create-account/password",
                    "Accept": "application/json"
                },
                timeout=Config.TIMEOUT
            )
            print(f"   响应状态: {resp.status_code}")
            if resp.status_code in [200, 302]:
                try:
                    result = resp.json()
                    print(f"   响应内容: {result}")
                except:
                    pass
                print(f"✅ 验证码已发送")
                return True
            print(f"❌ 发送验证码失败: {resp.status_code} - {resp.text[:200]}")
            return False
        except Exception as e:
            print(f"❌ 发送验证码异常: {e}")
            return False
    
    def step8_verify_otp(self, code: str) -> bool:
        """步骤8: 验证邮箱验证码"""
        print(f"\n📍 步骤8: 验证验证码 ({code})")
        try:
            self._delay()
            headers = self._get_api_headers(
                referer=f"{Config.AUTH_BASE}/email-verification",
                with_sentinel=True,
                flow="email_otp_validate"
            )
            
            resp = self.session.post(
                f"{Config.AUTH_BASE}/api/accounts/email-otp/validate",
                json={"code": code},
                headers=headers,
                timeout=Config.TIMEOUT
            )
            
            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                result = resp.json()
                print(f"   响应内容: {result}")
                page_type = result.get('page', {}).get('type', '')
                if page_type == 'about_you' or 'continue_url' in result:
                    print(f"✅ 验证成功，进入个人信息页面")
                    return True
                    
            print(f"❌ 验证失败: {resp.status_code} - {resp.text[:200]}")
            return False
        except Exception as e:
            print(f"❌ 验证异常: {e}")
            return False
    
    def step9_fill_profile(self, name: str, birthday: str) -> Optional[str]:
        """步骤9: 填写个人信息，返回OAuth URL"""
        print(f"\n📍 步骤9: 填写个人信息 (name={name}, birthdate={birthday})")
        try:
            self._delay()
            headers = self._get_api_headers(
                referer=f"{Config.AUTH_BASE}/about-you",
                with_sentinel=True,
                flow="create_account"
            )
            
            resp = self.session.post(
                f"{Config.AUTH_BASE}/api/accounts/create_account",
                json={"name": name, "birthdate": birthday},
                headers=headers,
                timeout=Config.TIMEOUT
            )
            
            print(f"   响应状态: {resp.status_code}")
            if resp.status_code == 200:
                result = resp.json()
                print(f"   响应内容: {str(result)[:200]}...")
                continue_url = result.get('continue_url', '')
                if 'oauth2/auth' in continue_url:
                    print(f"✅ 个人信息填写成功，获取到OAuth URL")
                    return continue_url
            
            print(f"❌ 填写个人信息失败: {resp.status_code} - {resp.text[:200]}")
            return None
        except Exception as e:
            print(f"❌ 填写个人信息异常: {e}")
            return None


    def step10_complete_auth(self, oauth_url: Optional[str] = None) -> Optional[str]:
        """步骤10: 完成OAuth认证流程，获取session token"""
        print(f"\n📍 步骤10: 完成OAuth认证")
        try:
            # 如果有OAuth URL，先访问它完成认证流程
            if oauth_url:
                print(f"   访问OAuth URL...")
                resp = self.session.get(oauth_url, timeout=Config.TIMEOUT, allow_redirects=True)
                print(f"   OAuth响应状态: {resp.status_code}")
                print(f"   最终URL: {resp.url}")
            
            self._delay()
            
            # 获取session
            resp = self.session.get(
                f"{Config.CHATGPT_BASE}/api/auth/session",
                timeout=Config.TIMEOUT
            )
            
            print(f"   Session响应状态: {resp.status_code}")
            if resp.status_code == 200:
                data = resp.json()
                print(f"   Session数据: {str(data)[:200]}...")
                
                # 检查session token
                token = self.session.cookies.get('__Secure-next-auth.session-token')
                if token:
                    print(f"✅ 获取到session token: {token[:50]}...")
                    return token
                
                # 检查用户信息
                if data.get('user'):
                    user = data['user']
                    print(f"✅ 登录成功: {user.get('email', 'unknown')}")
                    # 尝试从其他cookie获取token
                    for key in self.session.cookies.keys():
                        if 'session' in key.lower() or 'token' in key.lower():
                            print(f"   Cookie: {key}")
                    return "session_active"
            
            print(f"⚠️ 未获取到session token，但注册可能已成功")
            return None
        except Exception as e:
            print(f"❌ 完成认证异常: {e}")
            return None
    
    def get_cookies(self) -> List[Dict]:
        """获取所有Cookie，格式与chatgpt_accounts.json一致"""
        cookies = []
        for cookie in self.session.cookies.jar:
            cookie_dict = {
                "name": cookie.name,
                "value": cookie.value,
                "domain": cookie.domain,
                "path": cookie.path,
                "expires": cookie.expires if cookie.expires else -1,
                "httpOnly": bool(cookie._rest.get("HttpOnly", False)) if hasattr(cookie, '_rest') else False,
                "secure": cookie.secure,
                "sameSite": "Lax"
            }
            cookies.append(cookie_dict)
        return cookies

    def get_access_token(self) -> Optional[dict]:
        """获取 Access Token 和完整 session 数据"""
        try:
            resp = self.session.get(
                f"{Config.CHATGPT_BASE}/api/auth/session",
                timeout=Config.TIMEOUT
            )
            if resp.status_code == 200:
                data = resp.json()
                token = data.get("accessToken")
                if token:
                    print(f"🔑 获取到 Access Token: {token[:50]}...")
                    return data
            return None
        except Exception as e:
            print(f"⚠️ 获取 Access Token 失败: {e}")
            return None

    def generate_checkout_url(self, access_token: str, workspace_name: str = "MyTeam") -> Optional[str]:
        """生成 Team 订阅支付链接（绑卡链接）"""
        print(f"\n📍 生成 Team 订阅支付链接...")
        try:
            headers = {
                "User-Agent": Config.USER_AGENT,
                "Accept": "application/json",
                "Content-Type": "application/json",
                "Authorization": f"Bearer {access_token}",
                "Origin": Config.CHATGPT_BASE,
                "Referer": f"{Config.CHATGPT_BASE}/",
            }

            payload = {
                "plan_name": "chatgptteamplan",
                "team_plan_data": {
                    "workspace_name": workspace_name,
                    "price_interval": "month",
                    "seat_quantity": 5
                },
                "billing_details": {"country": "US", "currency": "USD"},
                "cancel_url": "https://chatgpt.com/#pricing",
                "promo_campaign": {
                    "promo_campaign_id": "team-1-month-free",
                    "is_coupon_from_query_param": False
                },
                "checkout_ui_mode": "hosted"
            }

            resp = self.session.post(
                f"{Config.CHATGPT_BASE}/backend-api/payments/checkout",
                headers=headers,
                json=payload,
                timeout=Config.TIMEOUT
            )

            if resp.status_code == 200:
                data = resp.json()
                checkout_url = data.get("url")
                if checkout_url:
                    print(f"✅ 支付链接: {checkout_url[:80]}...")
                    return checkout_url
                print(f"❌ 响应中无 URL: {data}")
            else:
                print(f"❌ 生成支付链接失败: {resp.status_code} - {resp.text[:200]}")
            return None
        except Exception as e:
            print(f"❌ 生成支付链接异常: {e}")
            return None


# ============================================================================
# 注册器主类
# ============================================================================
class ChatGPTRegister:
    """ChatGPT注册器 - 支持并发"""

    def __init__(self):
        """初始化注册器（邮箱API无需登录）"""
        self.success_count = 0
        self.fail_count = 0
        self.lock = threading.Lock()
        self.stop_flag = False
        self.save_lock = threading.Lock()
        self.panel_token = None
        self.panel_lock = threading.Lock()

    def _create_mail_client(self) -> MailClient:
        """为每个线程创建独立的邮箱客户端"""
        client = MailClient()
        if client.login() and client.get_domains():
            return client
        return None

    def register_one(self, thread_id: int = 0, mail_client: MailClient = None) -> Optional[Dict]:
        """注册一个账号"""
        prefix = f"[线程{thread_id}]" if thread_id > 0 else ""
        print(f"\n{prefix} " + "=" * 50)
        print(f"{prefix} 开始注册新账号")
        print(f"{prefix} " + "=" * 50)

        # 使用传入的邮箱客户端或创建新的
        if mail_client is None:
            mail_client = self._create_mail_client()
            if mail_client is None:
                with self.lock:
                    self.fail_count += 1
                return None

        # 生成注册信息
        name_info = Utils.generate_name()
        email_prefix = Utils.generate_email_prefix(name_info)
        domain_index = random.randint(0, len(mail_client.domains) - 1)
        email = mail_client.create_email(email_prefix, domain_index)

        if not email:
            with self.lock:
                self.fail_count += 1
            return None

        password = Config.DEFAULT_PASSWORD
        birthday = Utils.generate_birthday()

        print(f"\n{prefix} 📋 注册信息:")
        print(f"{prefix}    邮箱: {email}")
        print(f"{prefix}    密码: {password}")
        print(f"{prefix}    姓名: {name_info['fullName']}")
        print(f"{prefix}    生日: {birthday}")

        # 创建注册客户端
        client = ChatGPTRegisterClient()

        try:
            # 执行注册流程
            steps = [
                (client.step1_init_session, "初始化会话"),
                (client.step2_get_csrf, "获取CSRF"),
            ]

            for step_func, step_name in steps:
                if not step_func():
                    raise Exception(f"{step_name}失败")

            auth_url = client.step3_start_oauth()
            if not auth_url:
                raise Exception("获取授权URL失败")

            if not client.step4_authorize(auth_url):
                raise Exception("授权失败")

            if not client.step5_submit_email(email):
                raise Exception("提交邮箱失败")

            if not client.step6_register(email, password):
                raise Exception("注册失败")

            if not client.step7_send_otp():
                raise Exception("发送验证码失败")

            # 获取验证码
            code = mail_client.get_verification_code(email)
            if not code:
                raise Exception("获取验证码超时")

            if not client.step8_verify_otp(code):
                raise Exception("验证码验证失败")

            oauth_url = client.step9_fill_profile(name_info['fullName'], birthday)
            if not oauth_url:
                raise Exception("填写个人信息失败")

            session_token = client.step10_complete_auth(oauth_url)

            # 获取 Access Token 和完整 session 数据
            session_data = client.get_access_token()
            access_token = session_data.get("accessToken") if session_data else None

            # 提取 account_id 和 expired
            account_id = None
            expired = None
            if session_data:
                account_info = session_data.get("account", {})
                account_id = account_info.get("id")
                expired = session_data.get("expires")

            # 从 access_token 中提取订阅状态
            subscription_status = extract_subscription_from_token(access_token) if access_token else "free"
            print(f"{prefix} 📊 订阅状态: {subscription_status}")

            # 获取绑卡链接（只保存到txt）
            checkout_url = None
            if access_token:
                checkout_url = client.generate_checkout_url(access_token)

            # 保存结果 - 兼容目标格式
            now_time = datetime.now().isoformat()
            account = {
                "access_token": access_token,
                "account_id": account_id,
                "email": email,
                "expired": expired,
                "last_refresh": now_time,
                "type": subscription_status,  # 使用从 token 中提取的订阅状态
                "subscription_status": subscription_status,  # 添加 subscription_status 字段
                "cookies": client.get_cookies(),
                "created_at": now_time
            }

            # 绑卡链接单独传递（不保存到json）
            account["_checkout_url"] = checkout_url

            with self.lock:
                self.success_count += 1
                current_success = self.success_count

            print(f"\n{prefix} ✅ 注册成功! (当前成功: {current_success})")
            if checkout_url:
                print(f"{prefix} 💳 支付链接: {checkout_url}")
            self._save_account(account)
            return account

        except Exception as e:
            with self.lock:
                self.fail_count += 1
            print(f"\n{prefix} ❌ 注册失败: {e}")
            return None

    def _save_account(self, account: Dict):
        """保存账号信息（线程安全）"""
        with self.save_lock:
            try:
                # 提取绑卡链接（不保存到json）
                checkout_url = account.pop('_checkout_url', None)

                with open('chatgpt_accounts_api.json', 'a', encoding='utf-8') as f:
                    f.write(json.dumps(account, ensure_ascii=False) + '\n')
                print(f"💾 账号已保存到 chatgpt_accounts_api.json")

                # 保存绑卡链接到单独文件
                if checkout_url:
                    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
                    with open('chatgpt_accounts_check_url.txt', 'a', encoding='utf-8') as f:
                        f.write(f"{timestamp}｜{account['email']}｜{checkout_url}\n")
                    print(f"💳 绑卡链接已保存到 chatgpt_accounts_check_url.txt")
            except Exception as e:
                print(f"⚠️ 保存账号失败: {e}")

        if Config.PANEL_IMPORT_ENABLED:
            self._import_to_panel(account, checkout_url)

    def _get_panel_token(self) -> Optional[str]:
        """获取面板登录 token（缓存）"""
        if self.panel_token:
            return self.panel_token

        with self.panel_lock:
            if self.panel_token:
                return self.panel_token
            try:
                resp = requests.post(
                    f"{Config.PANEL_BASE}/api/v1/auth/login",
                    json={"username": Config.PANEL_USERNAME, "password": Config.PANEL_PASSWORD},
                    timeout=Config.TIMEOUT,
                )
                if resp.status_code != 200:
                    print(f"⚠️ 面板登录失败: {resp.status_code} - {resp.text[:200]}")
                    return None
                data = resp.json()
                token = data.get("token")
                if not token:
                    print("⚠️ 面板登录未返回 token")
                    return None
                self.panel_token = token
                return token
            except Exception as e:
                print(f"⚠️ 面板登录异常: {e}")
                return None

    def _import_to_panel(self, account: Dict, checkout_url: Optional[str]):
        """导入账号到面板"""
        token = self._get_panel_token()
        if not token:
            return

        payload = {
            "email": account.get("email", ""),
            "password": Config.DEFAULT_PASSWORD,
            "access_token": account.get("access_token", ""),
            "refresh_token": account.get("refresh_token", ""),
            "checkout_url": checkout_url or "",
            "account_id": account.get("account_id", ""),
            "session_cookies": account.get("cookies", []),
            "status": "active" if account.get("access_token") else "pending",
            "subscription_status": account.get("subscription_status", account.get("type", "free")),  # 添加订阅状态
            "name": account.get("name", ""),
            "created_at": account.get("created_at", ""),
            "last_refresh": account.get("last_refresh", ""),
            "expired": account.get("expired", ""),
            "type": account.get("type", ""),
        }

        try:
            resp = requests.post(
                f"{Config.PANEL_BASE}/api/v1/accounts/import",
                headers={"Authorization": f"Bearer {token}"},
                json=payload,
                timeout=Config.TIMEOUT,
            )
            if resp.status_code != 200:
                print(f"⚠️ 面板导入失败: {resp.status_code} - {resp.text[:200]}")
                return
            print("✅ 已导入面板")
        except Exception as e:
            print(f"⚠️ 面板导入异常: {e}")

    def _worker_thread(self, thread_id: int, target_count: int):
        """工作线程函数"""
        # 每个线程创建自己的邮箱客户端
        mail_client = self._create_mail_client()
        if mail_client is None:
            print(f"[线程{thread_id}] ❌ 无法初始化邮箱客户端")
            return

        while True:
            # 检查是否已达到目标成功数
            with self.lock:
                if self.success_count >= target_count or self.stop_flag:
                    break

            # 尝试注册
            self.register_one(thread_id, mail_client)

            # 检查是否已达到目标
            with self.lock:
                if self.success_count >= target_count:
                    self.stop_flag = True
                    break

            # 短暂延迟避免请求过于频繁
            time.sleep(random.uniform(2, 5))

    def register_batch_concurrent(self, target_count: int, concurrency: int) -> Tuple[int, int]:
        """并发批量注册，直到成功数量达到目标"""
        print(f"\n开始并发注册，目标成功数量: {target_count}，并发数: {concurrency}")
        print(f"注意: 程序将持续运行直到成功注册 {target_count} 个账号\n")

        # 重置计数器
        self.success_count = 0
        self.fail_count = 0
        self.stop_flag = False

        threads = []

        # 启动工作线程，每个线程间隔1秒
        for i in range(concurrency):
            t = threading.Thread(target=self._worker_thread, args=(i + 1, target_count))
            t.daemon = True
            threads.append(t)
            t.start()
            print(f"🚀 线程 {i + 1} 已启动")

            # 启动间隔1秒
            if i < concurrency - 1:
                time.sleep(1)

        # 等待所有线程完成
        for t in threads:
            t.join()

        print(f"\n{'='*60}")
        print(f"并发注册完成!")
        print(f"目标数量: {target_count}")
        print(f"成功数量: {self.success_count}")
        print(f"失败数量: {self.fail_count}")
        print(f"{'='*60}")

        return self.success_count, self.fail_count

    def register_batch(self, count: int) -> Tuple[int, int]:
        """批量注册（单线程，保持向后兼容）"""
        print(f"\n开始批量注册，目标成功数量: {count} 个账号...")

        # 创建邮箱客户端
        mail_client = self._create_mail_client()
        if mail_client is None:
            print("❌ 无法初始化邮箱客户端")
            return 0, 1

        attempt = 0
        while self.success_count < count:
            attempt += 1
            print(f"\n{'='*60}")
            print(f"尝试 #{attempt} - 当前成功: {self.success_count}/{count}")
            self.register_one(0, mail_client)

            if self.success_count < count:
                delay = random.uniform(5, 10)
                print(f"\n⏳ 等待 {delay:.1f} 秒...")
                time.sleep(delay)

        print(f"\n{'='*60}")
        print(f"批量注册完成! 成功: {self.success_count}, 失败: {self.fail_count}")
        return self.success_count, self.fail_count


# ============================================================================
# 主函数
# ============================================================================
def main():
    """主函数"""
    import sys

    print("=" * 60)
    print("ChatGPT 纯协议注册机 (curl_cffi) - 并发版")
    print("邮箱API: mail.chatgpt.org.uk (无需登录)")
    print("=" * 60)

    try:
        if len(sys.argv) >= 3:
            count = int(sys.argv[1])
            concurrency = int(sys.argv[2])
        elif len(sys.argv) >= 2:
            count = int(sys.argv[1])
            concurrency = 1
        else:
            count = int(input("请输入目标成功数量: ").strip())
            concurrency = int(input("请输入并发数量 (1为单线程): ").strip() or "1")

        if count < 1:
            print("❌ 注册数量必须大于0")
            return

        if concurrency < 1:
            concurrency = 1

        # 预加载名字文件
        Utils.load_names()

        register = ChatGPTRegister()

        if concurrency == 1:
            if count == 1:
                register.register_one()
            else:
                register.register_batch(count)
        else:
            register.register_batch_concurrent(count, concurrency)

    except KeyboardInterrupt:
        print("\n\n⚠️ 用户中断程序运行")
        print("程序已停止")
    except Exception as e:
        print(f"\n❌ 程序出错: {e}")


if __name__ == "__main__":
    main()
