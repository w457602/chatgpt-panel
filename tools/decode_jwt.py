#!/usr/bin/env python3
import base64
import json
import sys

def decode_jwt(token):
    """解码 JWT token 的 payload 部分"""
    parts = token.split('.')
    if len(parts) != 3:
        print("Invalid JWT format")
        return None
    
    payload = parts[1]
    # 添加 padding
    padding = 4 - len(payload) % 4
    if padding != 4:
        payload += '=' * padding
    # base64url 解码
    payload = payload.replace('-', '+').replace('_', '/')
    decoded = base64.b64decode(payload)
    return json.loads(decoded)

# 第一个账号的 access_token
token = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjE5MzQ0ZTY1LWJiYzktNDRkMS1hOWQwLWY5NTdiMDc5YmQwZSIsInR5cCI6IkpXVCJ9.eyJhdWQiOlsiaHR0cHM6Ly9hcGkub3BlbmFpLmNvbS92MSJdLCJjbGllbnRfaWQiOiJhcHBfRU1vYW1FRVo3M2YwQ2tYYVhwN2hyYW5uIiwiZXhwIjoxNzcwNjIyNTc2LCJodHRwczovL2FwaS5vcGVuYWkuY29tL2F1dGgiOnsiY2hhdGdwdF9hY2NvdW50X2lkIjoiYjBkZTNiNTEtZmIyOC00ODhlLWIyMTMtNWMxOWFmODgyZDJiIiwiY2hhdGdwdF9hY2NvdW50X3VzZXJfaWQiOiJ1c2VyLTNoYzdOOHU0UmRuT2tLcm41QmhuV0hsel9fYjBkZTNiNTEtZmIyOC00ODhlLWIyMTMtNWMxOWFmODgyZDJiIiwiY2hhdGdwdF9jb21wdXRlX3Jlc2lkZW5jeSI6Im5vX2NvbnN0cmFpbnQiLCJjaGF0Z3B0X3BsYW5fdHlwZSI6InRlYW0iLCJjaGF0Z3B0X3VzZXJfaWQiOiJ1c2VyLTNoYzdOOHU0UmRuT2tLcm41QmhuV0hseiIsInVzZXJfaWQiOiJ1c2VyLTNoYzdOOHU0UmRuT2tLcm41QmhuV0hseiJ9LCJodHRwczovL2FwaS5vcGVuYWkuY29tL3Byb2ZpbGUiOnsiZW1haWwiOiIyYTcyNzI2NEByZXZlcmVuZG5lZWRoYW0uY28udWsiLCJlbWFpbF92ZXJpZmllZCI6dHJ1ZX0sImlhdCI6MTc2OTc1ODU3NiwiaXNzIjoiaHR0cHM6Ly9hdXRoLm9wZW5haS5jb20iLCJqdGkiOiIxYTk1OGJkYy00ODVmLTRmMDMtOGM2OC1mYWJmY2FlZGZjOTciLCJuYmYiOjE3Njk3NTg1NzYsInB3ZF9hdXRoX3RpbWUiOjE3Njk3NTg1NzAyMzEsInNjcCI6WyJvcGVuaWQiLCJlbWFpbCIsInByb2ZpbGUiLCJvZmZsaW5lX2FjY2VzcyJdLCJzZXNzaW9uX2lkIjoiYXV0aHNlc3NfYXpQaUhPZ05FR0s4Yjc3UU9ZMUIyWnp5Iiwic3ViIjoiYXV0aDB8SmpkRVhDRXhRN01xa0E1Y1pOelZyZFRLIn0.JCZlSvdSLOL6utEKsjxiaxaYVxsnpUhX-iAChcwl28IrO298-WBONRO3hwKAZPCFRbOpdMTnjjuGiyvsA94Nowh3BF_bNw63rU-xaONpBI0lmKv-y7yrpo2A5H_QORLbV3-9M0mP9x2k6gAls5olEbujo2SM6yxZdHxZp2C7fsoiGwTIVq2G9KxpyZrmhQo3RhR3-bqr8lTGBS48CyuUipO6PTnB_FWc9OASMCgfgVixGAraDMLeu9Gpr4PBW-wfXyPEXQseLgHQI3khIJhQY8BaUSd0iuq2bbTmjO9duYAT413ywNVWRCXejUoHtidjdpwY3cQAquWQv6xAMXrZMdeSjg7ucoP5zKScsvusufGac0_fuOWx6F_-wpnBtIcybRr_PtcAlqwKfcXpdevufzSgluPwFSmPA0PJob996W0wh-yDRdnyVIqbWLx8f-XYgSs6kL4w9CErlLzVdSP00wUAIeSIBVcKMSgAZ0-skQC3Cb_1tZJoyOVR6R8FFkm01Q54757uW9BdRTelMf0EJo2En1ngKsNQeUrxOMisop3I4pUF2_M62qX16IOKHbBxnONLRgPwB4fJvIXmwe7S3rzX0HjI-9HbgMbkLkIUxx7KBqfIKm-MWxn9SIhxL6qy7PLk7dI7f45_ReNyCBmDt8vSezj15JYzELKPKqNVKR8"

data = decode_jwt(token)
if data:
    print("=== JWT Payload 解码结果 ===\n")
    print(json.dumps(data, indent=2, ensure_ascii=False))
    
    # 提取关键信息
    print("\n=== 订阅状态关键信息 ===")
    auth_info = data.get("https://api.openai.com/auth", {})
    print(f"Plan Type: {auth_info.get('chatgpt_plan_type', 'N/A')}")
    print(f"User ID: {auth_info.get('chatgpt_user_id', 'N/A')}")
    print(f"Account ID: {auth_info.get('chatgpt_account_id', 'N/A')}")
    
    profile_info = data.get("https://api.openai.com/profile", {})
    print(f"Email: {profile_info.get('email', 'N/A')}")

