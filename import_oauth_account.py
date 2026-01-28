#!/usr/bin/env python3
"""
OAuth 账号导入脚本
将本地获取的 RT 数据导入到 chatgpt-panel 线上项目
"""

import json
import requests
import jwt
from datetime import datetime
from typing import Optional, Dict

# 配置
CONFIG = {
    "api_base": "https://chatgptpanel.zeabur.app",
    "import_endpoint": "/api/v1/accounts/import",
    "local_token_file": "oauth_tokens.json",
    "timeout": 30,
}


def decode_jwt_payload(token: str) -> Optional[Dict]:
    """解码 JWT token 获取 payload (不验证签名)"""
    try:
        # 不验证签名，只解码
        payload = jwt.decode(token, options={"verify_signature": False})
        return payload
    except Exception as e:
        print(f"   JWT 解码失败: {e}")
        return None


def extract_account_info(access_token: str) -> Dict:
    """从 access_token 中提取账号信息"""
    info = {
        "account_id": "",
        "subscription_status": "free",
        "user_id": "",
    }
    
    payload = decode_jwt_payload(access_token)
    if payload:
        auth_info = payload.get("https://api.openai.com/auth", {})
        info["account_id"] = auth_info.get("chatgpt_account_id", "")
        info["subscription_status"] = auth_info.get("chatgpt_plan_type", "free")
        info["user_id"] = auth_info.get("chatgpt_user_id", "")
    
    return info


def load_local_tokens(file_path: str) -> Optional[Dict]:
    """加载本地 token 文件"""
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            data = json.load(f)
        print(f"✅ 已加载本地 token 文件: {file_path}")
        return data
    except FileNotFoundError:
        print(f"❌ 文件不存在: {file_path}")
        return None
    except json.JSONDecodeError as e:
        print(f"❌ JSON 解析错误: {e}")
        return None


def transform_to_import_format(local_data: Dict, password: str = "") -> Dict:
    """将本地数据转换为导入格式"""
    # 从 access_token 提取账号信息
    account_info = extract_account_info(local_data.get("access_token", ""))
    
    import_data = {
        "email": local_data.get("email", ""),
        "password": password,
        "access_token": local_data.get("access_token", ""),
        "refresh_token": local_data.get("refresh_token", ""),
        "account_id": account_info.get("account_id", ""),
        "status": "active",  # 有 token 就是 active
        "created_at": local_data.get("created_at", datetime.now().isoformat()),
    }
    
    return import_data


def import_account(api_base: str, data: Dict) -> bool:
    """调用导入 API"""
    url = f"{api_base}{CONFIG['import_endpoint']}"
    
    print(f"\n📤 正在导入账号到: {url}")
    print(f"   Email: {data.get('email')}")
    print(f"   Account ID: {data.get('account_id')}")
    print(f"   Status: {data.get('status')}")
    
    try:
        headers = {
            "Content-Type": "application/json",
            "User-Agent": "ChatGPT-Panel-Importer/1.0",
        }
        
        resp = requests.post(
            url,
            json=data,  # 发送单个对象
            headers=headers,
            timeout=CONFIG["timeout"]
        )
        
        print(f"   响应状态: {resp.status_code}")
        
        if resp.status_code == 200:
            result = resp.json()
            print(f"✅ 导入成功!")
            print(f"   响应: {json.dumps(result, ensure_ascii=False, indent=2)}")
            return True
        else:
            print(f"❌ 导入失败: {resp.text}")
            return False
            
    except requests.exceptions.RequestException as e:
        print(f"❌ 请求异常: {e}")
        return False


def main():
    print("=" * 60)
    print("ChatGPT OAuth 账号导入工具")
    print("=" * 60)
    
    # 1. 加载本地 token
    local_data = load_local_tokens(CONFIG["local_token_file"])
    if not local_data:
        return
    
    print(f"\n📋 本地数据:")
    print(f"   Email: {local_data.get('email')}")
    print(f"   Access Token: {local_data.get('access_token', '')[:50]}...")
    print(f"   Refresh Token: {local_data.get('refresh_token', '')[:50]}...")
    
    # 2. 输入密码 (可选)
    password = input("\n🔑 请输入账号密码 (可留空): ").strip()
    
    # 3. 转换格式
    import_data = transform_to_import_format(local_data, password)
    
    print(f"\n📦 转换后的导入数据:")
    display_data = {k: (v[:50] + "..." if isinstance(v, str) and len(v) > 50 else v) 
                    for k, v in import_data.items()}
    print(json.dumps(display_data, ensure_ascii=False, indent=2))
    
    # 4. 确认导入
    confirm = input("\n确认导入? (y/n): ").strip().lower()
    if confirm != 'y':
        print("已取消导入")
        return
    
    # 5. 执行导入
    success = import_account(CONFIG["api_base"], import_data)
    
    if success:
        print("\n" + "=" * 60)
        print("✅ 账号导入完成!")
        print("=" * 60)
    else:
        print("\n" + "=" * 60)
        print("❌ 账号导入失败")
        print("=" * 60)


if __name__ == "__main__":
    main()

