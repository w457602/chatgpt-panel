#!/usr/bin/env python3
import base64
import json
import sys

def decode_jwt_payload(token):
    parts = token.split('.')
    if len(parts) < 2:
        return None
    payload = parts[1]
    padding = 4 - len(payload) % 4
    if padding != 4:
        payload += '=' * padding
    payload = payload.replace('-', '+').replace('_', '/')
    decoded = base64.b64decode(payload)
    return json.loads(decoded)

if len(sys.argv) < 2:
    print("Usage: python3 decode_token.py <access_token>")
    sys.exit(1)

token = sys.argv[1]
payload = decode_jwt_payload(token)

if payload:
    auth_info = payload.get("https://api.openai.com/auth", {})
    profile = payload.get("https://api.openai.com/profile", {})
    
    print("=" * 60)
    print("Account Subscription Info")
    print("=" * 60)
    print(f"Email: {profile.get('email')}")
    print(f"Plan Type: {auth_info.get('chatgpt_plan_type')}")
    print(f"Account ID: {auth_info.get('chatgpt_account_id')}")
    print(f"User ID: {auth_info.get('chatgpt_user_id')}")
else:
    print("Failed to decode token")

