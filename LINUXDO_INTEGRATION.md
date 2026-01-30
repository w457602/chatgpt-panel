# Linux DO 集成功能实现文档

## 📋 概述

已在 chatgpt-panel 项目中成功实现了 Linux DO OAuth 登录和 Linux DO Credit 订单支付功能。

## ✅ 已完成的功能

### 1. Linux DO OAuth 2.0 登录

允许用户使用 Linux DO 账号登录系统。

**功能特性：**
- 标准 OAuth 2.0 授权流程
- 自动创建/更新 Linux DO 用户信息
- JWT Session Token 签发（30天有效期）
- 用户信息同步（UID、用户名、信任等级等）

**API 端点：**
```
GET  /api/v1/linuxdo/authorize-url?redirectUri=xxx  # 获取授权 URL
POST /api/v1/linuxdo/exchange                       # 用授权码换取用户信息
GET  /api/v1/linuxdo/me                            # 获取当前用户信息
PUT  /api/v1/linuxdo/me/email                      # 更新用户邮箱
```

### 2. Linux DO Credit 支付订单

集成 Linux DO Credit 支付网关，支持创建订单、查询订单和支付回调。

**功能特性：**
- 创建支付订单并获取支付链接
- MD5 签名验证（防篡改）
- 支付回调处理和订单状态更新
- 订单列表查询
- 自动金额校验和退款标记

**API 端点：**

