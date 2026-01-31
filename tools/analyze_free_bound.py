#!/usr/bin/env python3
"""
分析显示 free 但已绑卡的账号
"""
import requests

# Panel API 配置
PANEL_API_BASE = "https://openai.netpulsex.icu"
PANEL_USERNAME = "admin"
PANEL_PASSWORD = "admin123"

def main():
    # 登录获取 token
    resp = requests.post(
        f"{PANEL_API_BASE}/api/v1/auth/login",
        json={"username": PANEL_USERNAME, "password": PANEL_PASSWORD},
        timeout=30
    )
    token = resp.json().get("token")
    print(f"✅ 登录成功\n")

    # 获取所有账号（分页）
    headers = {"Authorization": f"Bearer {token}"}
    all_accounts = []
    page = 1
    while True:
        resp = requests.get(
            f"{PANEL_API_BASE}/api/v1/accounts",
            params={"page": page, "page_size": 100},
            headers=headers,
            timeout=30
        )
        data = resp.json()
        accounts = data.get("data", [])
        if not accounts:
            break
        all_accounts.extend(accounts)
        total_pages = data.get("total_pages", 1)
        print(f"获取第 {page}/{total_pages} 页，共 {len(accounts)} 条")
        if page >= total_pages:
            break
        page += 1

    print(f"\n📊 总共获取 {len(all_accounts)} 个账号\n")

    # 分析：找出显示 free 但已绑卡的账号
    problem_accounts = []
    for acc in all_accounts:
        subscription_status = (acc.get("subscription_status") or "").lower().strip()
        is_free = subscription_status in ["", "free"]
        
        plus_bound = acc.get("plus_bound", False)
        team_bound = acc.get("team_bound", False)
        is_plus = acc.get("is_plus", False)
        is_team = acc.get("is_team", False)
        checkout_url = acc.get("checkout_url", "") or ""
        team_checkout_url = acc.get("team_checkout_url", "") or ""
        
        # 如果显示 free，但已绑卡或有订阅标记
        if is_free and (plus_bound or team_bound or is_plus or is_team):
            problem_accounts.append({
                "id": acc.get("id"),
                "email": acc.get("email"),
                "subscription_status": subscription_status or "(空)",
                "plus_bound": plus_bound,
                "team_bound": team_bound,
                "is_plus": is_plus,
                "is_team": is_team,
                "has_plus_url": bool(checkout_url),
                "has_team_url": bool(team_checkout_url),
            })

    print(f"🔍 分析结果：显示 free 但已绑卡/有订阅的账号: {len(problem_accounts)} 个\n")

    if problem_accounts:
        print("=" * 110)
        print(f"{'ID':>5} | {'邮箱':<40} | {'状态':<8} | Plus绑卡 | Team绑卡 | is_plus | is_team")
        print("=" * 110)
        for acc in problem_accounts[:100]:  # 显示前100个
            plus_mark = "✅" if acc['plus_bound'] else "❌"
            team_mark = "✅" if acc['team_bound'] else "❌"
            is_plus_mark = "✅" if acc['is_plus'] else "❌"
            is_team_mark = "✅" if acc['is_team'] else "❌"
            print(f"{acc['id']:>5} | {acc['email']:<40} | {acc['subscription_status']:<8} | "
                  f"{plus_mark:^8} | {team_mark:^8} | {is_plus_mark:^7} | {is_team_mark:^7}")
        
        if len(problem_accounts) > 100:
            print(f"\n... 还有 {len(problem_accounts) - 100} 个账号未显示")
    
    # 统计
    print("\n" + "=" * 60)
    print("📈 统计汇总")
    print("=" * 60)
    plus_bound_count = sum(1 for a in problem_accounts if a['plus_bound'])
    team_bound_count = sum(1 for a in problem_accounts if a['team_bound'])
    is_plus_count = sum(1 for a in problem_accounts if a['is_plus'])
    is_team_count = sum(1 for a in problem_accounts if a['is_team'])
    
    print(f"Plus 已绑卡 (plus_bound=true): {plus_bound_count} 个")
    print(f"Team 已绑卡 (team_bound=true): {team_bound_count} 个")
    print(f"Plus 订阅标记 (is_plus=true): {is_plus_count} 个")
    print(f"Team 订阅标记 (is_team=true): {is_team_count} 个")

if __name__ == "__main__":
    main()

