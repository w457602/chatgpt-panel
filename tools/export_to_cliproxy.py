#!/usr/bin/env python3
"""
导出账号到 CLIProxyAPI 导入格式
从 Panel 数据库导出 Plus 和 Team 账号，生成 NDJSON 格式文件
"""

import json
import sys
import os
from datetime import datetime, timedelta
from typing import Dict, List, Optional
import base64

# 添加当前目录到路径
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

try:
    import requests as std_requests
except ImportError:
    print("❌ 请安装 requests: pip3 install requests")
    sys.exit(1)

# ============================================================================
# 配置
# ============================================================================
PANEL_BASE = os.environ.get("PANEL_BASE", "https://openai.netpulsex.icu")
PANEL_USERNAME = os.environ.get("PANEL_USERNAME", "admin")
PANEL_PASSWORD = os.environ.get("PANEL_PASSWORD", "admin123")

# ============================================================================
# Panel API Client
# ============================================================================
class PanelAPIClient:
    def __init__(self, base_url: str = PANEL_BASE):
        self.base_url = base_url.rstrip('/')
        self.token = None

    def login(self) -> bool:
        try:
            resp = std_requests.post(
                f"{self.base_url}/api/v1/auth/login",
                json={"username": PANEL_USERNAME, "password": PANEL_PASSWORD},
                timeout=30
            )
            if resp.status_code == 200:
                data = resp.json()
                self.token = data.get("token")
                return bool(self.token)
        except Exception as e:
            print(f"❌ Panel 登录异常: {e}")
        return False

    def _get_headers(self) -> Dict:
        return {"Authorization": f"Bearer {self.token}", "Content-Type": "application/json"}

    def get_accounts(self, page: int = 1, page_size: int = 100) -> Dict:
        try:
            resp = std_requests.get(
                f"{self.base_url}/api/v1/accounts",
                params={"page": page, "page_size": page_size},
                headers=self._get_headers(),
                timeout=30
            )
            if resp.status_code == 200:
                return resp.json()
        except Exception as e:
            print(f"❌ 获取账号异常: {e}")
        return {}

    def update_cliproxy_synced(self, account_id: int) -> bool:
        """更新账号的 cliproxy_synced_at 字段"""
        try:
            resp = std_requests.post(
                f"{self.base_url}/api/v1/accounts/{account_id}/cliproxy-sync",
                headers=self._get_headers(),
                timeout=30
            )
            return resp.status_code == 200
        except:
            return False


def extract_account_id_from_token(access_token: str) -> Optional[str]:
    """从 access_token 的 JWT payload 中提取 account_id"""
    try:
        parts = access_token.split('.')
        if len(parts) < 2:
            return None
        payload = parts[1]
        # 补充 base64 padding
        padding = 4 - len(payload) % 4
        if padding != 4:
            payload += '=' * padding
        decoded = base64.urlsafe_b64decode(payload)
        data = json.loads(decoded)
        # 从 auth claim 中获取 account_id
        auth_data = data.get("https://api.openai.com/auth", {})
        return auth_data.get("chatgpt_account_id")
    except:
        return None


def build_cliproxy_entry(email: str, access_token: str, refresh_token: str, account_id: str) -> Dict:
    """构建 CLIProxyAPI 导入格式的条目"""
    now = datetime.now()
    expired = now + timedelta(days=10)  # 假设 10 天后过期
    
    return {
        "email": email,
        "access_token": access_token,
        "refresh_token": refresh_token,  # 关键：CLIProxyAPI 使用 refresh_token 字段
        "account_id": account_id or "",
        "last_refresh": now.strftime("%Y-%m-%dT%H:%M:%S+08:00"),
        "type": "codex",
        "expired": expired.strftime("%Y-%m-%dT%H:%M:%S+08:00")
    }


def main():
    print("=" * 60)
    print("导出账号到 CLIProxyAPI 格式")
    print("=" * 60)
    
    # 1. 连接 Panel
    print("\n🔌 连接 Panel API...")
    client = PanelAPIClient()
    if not client.login():
        print("❌ Panel 登录失败")
        return
    print("✅ Panel 登录成功")
    
    # 2. 获取所有账号
    print("\n📥 获取账号列表...")
    all_accounts = []
    page = 1
    while True:
        data = client.get_accounts(page=page, page_size=100)
        accounts = data.get("accounts", [])
        if not accounts:
            break
        all_accounts.extend(accounts)
        total = data.get("total", 0)
        print(f"   已获取 {len(all_accounts)}/{total} 个账号")
        if len(all_accounts) >= total:
            break
        page += 1
    
    print(f"✅ 共获取 {len(all_accounts)} 个账号")
    
    # 3. 筛选有效账号（active 且有 RT）
    valid_accounts = []
    for acc in all_accounts:
        if acc.get("status") != "active":
            continue
        # 检查是否有 Plus RT 或 Team RT
        has_plus = bool(acc.get("plus_refresh_token"))
        has_team = bool(acc.get("team_refresh_token"))
        # 优先未同步的
        synced_at = acc.get("cliproxy_synced_at")
        if has_plus or has_team:
            valid_accounts.append({
                "account": acc,
                "has_plus": has_plus,
                "has_team": has_team,
                "synced_at": synced_at
            })
    
    # 按 synced_at 排序（未同步的优先）
    valid_accounts.sort(key=lambda x: (x["synced_at"] or "", x["account"]["id"]))
    
    print(f"✅ 筛选到 {len(valid_accounts)} 个有效账号")

    # 4. 取前 100 个账号
    target_count = 100
    selected = valid_accounts[:target_count]

    # 5. 生成导出数据
    export_entries = []
    plus_count = 0
    team_count = 0
    account_ids_to_mark = []

    for item in selected:
        acc = item["account"]
        email = acc.get("email", "")
        acc_id = acc.get("id")

        # 导出 Plus RT
        if item["has_plus"]:
            at = acc.get("plus_access_token", "")
            rt = acc.get("plus_refresh_token", "")
            account_id = extract_account_id_from_token(at) if at else acc.get("account_id", "")
            entry = build_cliproxy_entry(email, at, rt, account_id)
            export_entries.append(entry)
            plus_count += 1

        # 导出 Team RT
        if item["has_team"]:
            at = acc.get("team_access_token", "")
            rt = acc.get("team_refresh_token", "")
            account_id = extract_account_id_from_token(at) if at else ""
            entry = build_cliproxy_entry(email, at, rt, account_id)
            export_entries.append(entry)
            team_count += 1

        account_ids_to_mark.append(acc_id)

    print(f"\n📊 导出统计:")
    print(f"   - Plus 账号: {plus_count} 个")
    print(f"   - Team 账号: {team_count} 个")
    print(f"   - 总条目数: {len(export_entries)} 条")

    # 6. 生成文件
    timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
    output_dir = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
    output_file = os.path.join(output_dir, f"export_to_cliproxy_{timestamp}.json")

    with open(output_file, 'w', encoding='utf-8') as f:
        for entry in export_entries:
            f.write(json.dumps(entry, ensure_ascii=False) + '\n')

    print(f"\n✅ 文件已生成: {output_file}")

    # 7. 更新数据库同步状态
    print(f"\n🔄 更新数据库同步状态...")
    success_count = 0
    for acc_id in account_ids_to_mark:
        if client.update_cliproxy_synced(acc_id):
            success_count += 1

    print(f"✅ 已更新 {success_count}/{len(account_ids_to_mark)} 个账号的同步状态")

    print("\n" + "=" * 60)
    print("导出完成!")
    print("=" * 60)


if __name__ == "__main__":
    main()

