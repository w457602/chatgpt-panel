#!/usr/bin/env python3
# -*- coding: utf-8 -*-

import argparse
import json
import os
import sys
import time
import threading
import urllib.parse
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed


API_BASE = "https://mail.chatgpt.org.uk/api/emails"
DEACTIVATED_SUBJECT = "OpenAI - Access Deactivated"


def build_opener_from_env():
    proxies = {}
    for key in ("http_proxy", "https_proxy", "HTTP_PROXY", "HTTPS_PROXY"):
        val = os.getenv(key)
        if val:
            if key.lower().startswith("http"):
                proxies["http"] = val
            if key.lower().startswith("https"):
                proxies["https"] = val
    if proxies:
        return urllib.request.build_opener(urllib.request.ProxyHandler(proxies))
    return urllib.request.build_opener()


def load_emails(path):
    emails = set()
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
            except json.JSONDecodeError:
                continue
            email = obj.get("email")
            if email:
                emails.add(email)
    return sorted(emails)


def fetch_inbox(opener, email, timeout=20):
    params = urllib.parse.urlencode({"email": email})
    url = f"{API_BASE}?{params}"
    req = urllib.request.Request(
        url,
        headers={
            "User-Agent": "Mozilla/5.0",
            "Origin": "https://mail.chatgpt.org.uk",
            "Referer": "https://mail.chatgpt.org.uk/",
        },
    )
    with opener.open(req, timeout=timeout) as resp:
        raw = resp.read().decode("utf-8")
    return json.loads(raw)


def find_deactivated(emails_list):
    for msg in emails_list:
        subject = (msg.get("subject") or "").strip()
        if DEACTIVATED_SUBJECT.lower() in subject.lower():
            return True
    return False


def extract_domain(email):
    if "@" not in email:
        return ""
    return email.split("@", 1)[1].lower().strip()


def load_banned_domains(path):
    domains = set()
    if not path or not os.path.exists(path):
        return domains
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            domains.add(line.lower())
    return domains


def append_banned_domains(path, domains):
    if not domains:
        return
    existing = load_banned_domains(path)
    new_items = [d for d in sorted(domains) if d and d not in existing]
    if not new_items:
        return
    with open(path, "a", encoding="utf-8") as f:
        for d in new_items:
            f.write(d + "\n")


def http_json(opener, url, method="GET", headers=None, payload=None, timeout=20):
    body = None
    if payload is not None:
        body = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(url, data=body, method=method)
    # 添加默认 User-Agent 避免被 Cloudflare 拦截
    req.add_header("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    if payload is not None:
        req.add_header("Content-Type", "application/json")
    with opener.open(req, timeout=timeout) as resp:
        raw = resp.read().decode("utf-8")
    return json.loads(raw)


def panel_login(opener, base, username, password, timeout=20):
    url = base.rstrip("/") + "/api/v1/auth/login"
    data = http_json(
        opener,
        url,
        method="POST",
        payload={"username": username, "password": password},
        timeout=timeout,
    )
    return data.get("token")


def panel_list_accounts(opener, base, token, page=1, page_size=100, timeout=20):
    params = urllib.parse.urlencode({"page": page, "page_size": page_size})
    url = base.rstrip("/") + "/api/v1/accounts?" + params
    data = http_json(
        opener,
        url,
        method="GET",
        headers={"Authorization": f"Bearer {token}"},
        timeout=timeout,
    )
    items = data.get("data") or []
    total_pages = data.get("total_pages")
    total = data.get("total")
    return items, total, total_pages


def panel_fetch_all_accounts(opener, base, token, page_size=100, timeout=20):
    accounts = []
    page = 1
    total_pages = None
    while True:
        items, total, total_pages = panel_list_accounts(
            opener, base, token, page=page, page_size=page_size, timeout=timeout
        )
        if not items:
            break
        accounts.extend(items)
        if total_pages is not None and page >= int(total_pages):
            break
        page += 1
    return accounts


def panel_find_account_ids(opener, base, token, email, timeout=20):
    params = urllib.parse.urlencode({"search": email, "page_size": 50})
    url = base.rstrip("/") + "/api/v1/accounts?" + params
    data = http_json(
        opener,
        url,
        method="GET",
        headers={"Authorization": f"Bearer {token}"},
        timeout=timeout,
    )
    items = data.get("data") or []
    ids = []
    for item in items:
        if (item.get("email") or "").lower() == email.lower():
            if item.get("id") is not None:
                ids.append(int(item["id"]))
    return ids


def panel_batch_update_status(opener, base, token, ids, status="banned", batch_size=100, timeout=20):
    if not ids:
        return 0
    total_updated = 0
    url = base.rstrip("/") + "/api/v1/accounts/batch-status"
    for i in range(0, len(ids), batch_size):
        chunk = ids[i:i + batch_size]
        data = http_json(
            opener,
            url,
            method="POST",
            headers={"Authorization": f"Bearer {token}"},
            payload={"ids": chunk, "status": status},
            timeout=timeout,
        )
        total_updated += int(data.get("count", 0) or 0)
    return total_updated


def process_email(email, timeout, sleep_seconds):
    opener = build_opener_from_env()
    try:
        data = fetch_inbox(opener, email, timeout=timeout)
        ok = data.get("success")
        emails_list = (data.get("data") or {}).get("emails") or []
        is_deactivated = find_deactivated(emails_list)
        domain = extract_domain(email)
        preview = []
        for m in emails_list[:3]:
            subject = m.get("subject", "")
            date = m.get("date", "")
            preview.append(f"  - {date} | {subject}")
        result = {
            "email": email,
            "success": ok,
            "deactivated": is_deactivated,
            "emails": emails_list,
        }
    except Exception as e:
        result = {"email": email, "success": False, "error": str(e)}
        preview = []
        domain = ""
        is_deactivated = False
    if sleep_seconds > 0:
        time.sleep(sleep_seconds)
    return result, preview, is_deactivated, domain


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--input", default="chatgpt_accounts_api.json", help="input jsonl file")
    source_group = parser.add_mutually_exclusive_group()
    source_group.add_argument("--from-panel", action="store_true", help="load emails from panel")
    source_group.add_argument("--from-file", action="store_true", help="load emails from input file")
    parser.add_argument("--sleep", type=float, default=0.3, help="seconds between requests")
    parser.add_argument("--timeout", type=int, default=20, help="request timeout seconds")
    parser.add_argument("--out", default="", help="output json file")
    parser.add_argument("--ban-domain-file", default="banned_email_domains.txt", help="append banned domains to file")
    parser.add_argument("--no-update-panel", action="store_true", help="do not update panel status")
    parser.add_argument("--no-delete-panel", action="store_true", help="(alias) do not update panel status")
    parser.add_argument("--panel-base", default=os.getenv("PANEL_BASE", "https://openai.netpulsex.icu"))
    parser.add_argument("--panel-username", default=os.getenv("PANEL_USERNAME", "admin"))
    parser.add_argument("--panel-password", default=os.getenv("PANEL_PASSWORD", "admin123"))
    parser.add_argument("--panel-page-size", type=int, default=100, help="panel page size")
    parser.add_argument("--workers", type=int, default=8, help="concurrent workers")
    args = parser.parse_args()
    if not args.from_panel and not args.from_file:
        args.from_panel = True

    panel_opener = build_opener_from_env()
    panel_token = None
    panel_email_to_ids = {}
    panel_email_display = {}
    if args.from_panel:
        try:
            panel_token = panel_login(panel_opener, args.panel_base, args.panel_username, args.panel_password, timeout=args.timeout)
        except Exception as e:
            print(f"面板登录失败: {e}")
            sys.exit(1)
        if not panel_token:
            print("面板登录失败: token 为空")
            sys.exit(1)
        accounts = panel_fetch_all_accounts(
            panel_opener,
            args.panel_base,
            panel_token,
            page_size=args.panel_page_size,
            timeout=args.timeout,
        )
        for item in accounts:
            email = (item.get("email") or "").strip()
            if not email:
                continue
            # 跳过已标记为 banned 状态的账号
            status = (item.get("status") or "").lower()
            if status == "banned":
                continue
            key = email.lower()
            panel_email_to_ids.setdefault(key, []).append(int(item.get("id")))
            if key not in panel_email_display:
                panel_email_display[key] = email
        emails = [panel_email_display[k] for k in sorted(panel_email_display.keys())]
    else:
        if not os.path.exists(args.input):
            print(f"file not found: {args.input}")
            sys.exit(1)
        emails = load_emails(args.input)

    print(f"total emails: {len(emails)}")

    results = []
    deactivated_emails = []
    deactivated_domains = set()
    total = len(emails)
    completed = 0
    lock = threading.Lock()

    with ThreadPoolExecutor(max_workers=max(1, args.workers)) as executor:
        future_map = {
            executor.submit(process_email, email, args.timeout, args.sleep): email
            for email in emails
        }
        for future in as_completed(future_map):
            email = future_map[future]
            try:
                result, preview, is_deactivated, domain = future.result()
            except Exception as e:
                result = {"email": email, "success": False, "error": str(e)}
                preview = []
                is_deactivated = False
                domain = ""
            with lock:
                completed += 1
                idx = completed
                results.append(result)
                if is_deactivated:
                    deactivated_emails.append(email)
                    if domain:
                        deactivated_domains.add(domain)
            count = len(result.get("emails") or [])
            flag = " DEACTIVATED" if is_deactivated else ""
            print(f"[{idx}/{total}] {email} -> {count}{flag}")
            for line in preview:
                print(line)

    if deactivated_domains:
        append_banned_domains(args.ban_domain_file, deactivated_domains)
        print(f"已追加禁用域名到: {args.ban_domain_file}")
    if deactivated_emails:
        print("检测到已封禁邮箱:")
        for email in deactivated_emails:
            print(f"  - {email}")

    skip_update_panel = args.no_update_panel or args.no_delete_panel
    total_updated = 0
    if not skip_update_panel and deactivated_emails:
        if not panel_token:
            try:
                panel_token = panel_login(panel_opener, args.panel_base, args.panel_username, args.panel_password, timeout=args.timeout)
            except Exception as e:
                print(f"面板登录失败: {e}")
                panel_token = None
        if panel_token:
            ids_to_update = []
            for email in deactivated_emails:
                ids = panel_email_to_ids.get(email.lower())
                if ids:
                    ids_to_update.extend(ids)
                else:
                    try:
                        ids = panel_find_account_ids(panel_opener, args.panel_base, panel_token, email, timeout=args.timeout)
                        if ids:
                            ids_to_update.extend(ids)
                    except Exception as e:
                        print(f"查询失败: {email} -> {e}")
            if ids_to_update:
                total_updated = panel_batch_update_status(
                    panel_opener,
                    args.panel_base,
                    panel_token,
                    ids_to_update,
                    status="banned",
                    timeout=args.timeout,
                )
                print(f"面板标记封禁完成，数量: {total_updated}")
            else:
                print("未找到可标记的面板账号")

    if args.out:
        with open(args.out, "w", encoding="utf-8") as f:
            json.dump(results, f, ensure_ascii=False, indent=2)
        print(f"saved: {args.out}")

    failed_count = len([r for r in results if not r.get("success")])
    print("检测完成")
    print(f"总数: {total}")
    print(f"成功: {total - failed_count}")
    print(f"失败: {failed_count}")
    print(f"已封禁: {len(deactivated_emails)}")
    if not skip_update_panel:
        print(f"已标记封禁面板账号: {total_updated}")


if __name__ == "__main__":
    main()
