#!/usr/bin/env python3
import base64
import json
import sys

# 之前刷新时返回的 id_token
id_token = "eyJhbGciOiJSUzI1NiIsImtpZCI6IjE5MzQ0ZTY1LWJiYzktNDRkMS1hOWQwLWY5NTdiMDc5YmQwZSIsInR5cCI6IkpXVCJ9.eyJhdWQiOiJhcHBfRU1vYW1FRVo3M2YwQ2tYYVhwN2hyYW5uIiwiYXpwIjoiYXBwX0VNb2FtRUVaNzNmMENrWGFYcDdocmFubiIsImVtYWlsIjoiOWQ5NmRkODBAeG5udXhhbS5zaG9wIiwiZW1haWxfdmVyaWZpZWQiOnRydWUsImV4cCI6MTc2OTkwNDgzNSwiaHR0cHM6Ly9hcGkub3BlbmFpLmNvbS9hdXRoIjp7ImNoYXRncHRfYWNjb3VudF9pZCI6IjI5ODVjM2I3LWE4NGMtNDBhYy1iNTUzLWIyODVlODFjOTU3MCIsImNoYXRncHRfcGxhbl90eXBlIjoidGVhbSIsImNoYXRncHRfc3Vic2NyaXB0aW9uX2xhc3RfY2hlY2tlZCI6MTczODAyMzIwNywiY2hhdGdwdF91c2VyX2lkIjoidXNlci1MR0VhZW9wcHVFNk8wYXlESm1HSWMzUU0iLCJvcmdhbml6YXRpb25zIjpbeyJpZCI6Im9yZy1RalhiN0RQMFFTd05XWENERUNReUtpNWMiLCJpc19kZWZhdWx0IjpmYWxzZSwicm9sZSI6Im93bmVyIiwidGl0bGUiOiJQZXJzb25hbCJ9LHsiZGlzYWJsZWQiOmZhbHNlLCJpZCI6Im9yZy0xR05xcGloZkVRN0VMN3VuaVdYN1pUSkMiLCJpc19kZWZhdWx0Ijp0cnVlLCJwbGFuX3R5cGUiOiJ0ZWFtIiwicm9sZSI6Im1lbWJlciIsInRpdGxlIjoiU3Bpa2UifV19LCJpYXQiOjE3Njk4MTg4MzUsImlzcyI6Imh0dHBzOi8vYXV0aC5vcGVuYWkuY29tIiwibmFtZSI6IiIsInBpY3R1cmUiOiJodHRwczovL3MuZ3JhdmF0YXIuY29tL2F2YXRhci8zMzAwMGMxMWYzNDM5ZmI0MTQ2OGRlN2U1NjM3Y2EyOT9zPTQ4MCZyPXBnJmQ9aHR0cHMlM0ElMkYlMkZjZG4uYXV0aDAuY29tJTJGYXZhdGFycyUyRjlkLnBuZyIsInN1YiI6ImF1dGgwfHI3aGVQWUZEeUtwRFpsS25YaE56SmVLayJ9.cNWkFdLPx_e5Kpp2f8KIWOKYhJo6vxlz4pSFl5hyP7TaUiIrFwM2dYJj3QRz_VPKUzzybHRB8bBzzpKPc3xJQXxUHCNJHBSYxgCXgBdHUQPD3JGXX-bkYCt4-AvFyXCcxFuxdJyT6AxOA5fAl8fvbqr65nKjc8BQ5pXW_w6xrU-qBT93KMR3jJgKPf2qYBqhVbvhnUBFm2C2qHuOIFCDnNHW5_hhIRwOsqJvnRZH_LQKKQPZDRMRBWLjU75LWtJG_77qSjQjpSPrpuvnlfnP9Q5MBjdSQ5J4J9c7XEuEcBIgDjy4jLxuKVNvVoQTnKLuaEr-EcO5H3lMpGO4WrHGo9UqU0Dxvz1BRG4f3VlzY3YwDM-mFPqTUPmW3PFZR_ZiQPtq-XzHUPNnzXRTKCbLVPNDYz0gLNsPkVSZxQGvMlJSJK2aHf8eR2KAjQpIRsLVNBmB2LScY6Cg0OKf4YhQm1YYcxS3RQYxQ5VvXGH-8L3DvRQlHgCVDBphJzHqq6UhwB5n0jjRZZzqMKM-PXspNw7xTdZc8SHMnfE35xpkPvXU7M5xt0qGqn0I4LY8wWK7QTPv_RPgK7C_3PF1l5C0gWGBfTRLjCBWRLhqpXC9RYxYT-7m0_xLjMcF7vqzv8-xFxUTQj1GqvH7BpVNHC-9w3DPGbZA4xA"

# 解析 JWT payload
parts = id_token.split('.')
payload = parts[1] + '=' * (4 - len(parts[1]) % 4)
decoded = base64.urlsafe_b64decode(payload)
data = json.loads(decoded)

print("=" * 60)
print("ID Token 中的账户信息")
print("=" * 60)

auth_info = data.get('https://api.openai.com/auth', {})

print(f"Email: {data.get('email')}")
print(f"Current Account ID: {auth_info.get('chatgpt_account_id')}")
print(f"Current Plan Type: {auth_info.get('chatgpt_plan_type')}")
print(f"User ID: {auth_info.get('chatgpt_user_id')}")
print()
print("=" * 60)
print("Organizations (用户的所有账户)")
print("=" * 60)

organizations = auth_info.get('organizations', [])
for i, org in enumerate(organizations, 1):
    print(f"\nOrganization {i}:")
    print(f"   ID: {org.get('id')}")
    print(f"   Title: {org.get('title')}")
    print(f"   Role: {org.get('role')}")
    print(f"   Is Default: {org.get('is_default')}")
    print(f"   Plan Type: {org.get('plan_type', 'N/A')}")

