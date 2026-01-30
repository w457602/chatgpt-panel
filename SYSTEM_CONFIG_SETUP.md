# 系统配置管理功能 - 实施说明

## 📋 功能概述

为 `chatgpt-panel` 项目添加了系统配置管理功能，允许管理员通过 Web 界面配置 Linux DO OAuth 和 Credit 支付参数，配置存储在数据库中，优先级高于环境变量。

## ✅ 已完成的工作

### 1. 数据库模型 (`internal/models/system_config.go`)

- 创建 `SystemConfig` 模型（Key-Value 存储）
- 支持配置分类（`category`）
- 支持敏感字段标记（`is_secret`）
- 提供工具函数：
  - `GetConfigValue(db, key, envValue)`: 获取配置（优先数据库）
  - `UpsertConfig(...)`: 创建或更新配置

### 2. 数据库迁移 (`internal/models/db.go`)

- 在 AutoMigrate 中添加 `SystemConfig` 模型

### 3. 配置服务 (`internal/services/system_config_service.go`)

- `GetLinuxDoOAuthConfig()`: 获取 OAuth 配置
- `UpdateLinuxDoOAuthConfig()`: 更新 OAuth 配置
- `GetLinuxDoCreditConfig()`: 获取 Credit 配置
- `UpdateLinuxDoCreditConfig()`: 更新 Credit 配置
- `GetAllConfigs()`: 导出所有配置
- `DeleteConfig()`: 删除配置

### 4. 服务层数据库配置集成

**Linux DO OAuth Service** (`internal/services/linuxdo_oauth_service.go`):
- `GetAuthorizeURL()`: 使用数据库配置
- `ExchangeCode()`: 使用数据库配置

**Credit Gateway Service** (`internal/services/credit_gateway_service.go`):
- `CreateOrder()`: 使用数据库配置
- `QueryOrder()`: 使用数据库配置
- `HandleNotify()`: 使用数据库配置

### 5. HTTP Handler (`internal/handlers/system_config_handler.go`)

API 端点：
- `GET /api/v1/admin/config/linuxdo-oauth`: 获取 OAuth 配置
- `PUT /api/v1/admin/config/linuxdo-oauth`: 更新 OAuth 配置
- `GET /api/v1/admin/config/linuxdo-credit`: 获取 Credit 配置
- `PUT /api/v1/admin/config/linuxdo-credit`: 更新 Credit 配置
- `GET /api/v1/admin/config/all`: 获取所有配置
- `DELETE /api/v1/admin/config/:key`: 删除配置

### 6. 路由注册 (`cmd/main.go`)

在 `auth` 路由组中注册系统配置管理路由（需要登录）

### 7. 前端界面 (`templates/index.html`)

- 在快捷操作栏添加"系统设置"按钮
- 创建系统配置模态框，包含：
  - Linux DO OAuth 配置区块（Client ID、Client Secret）
  - Linux DO Credit 配置区块（PID、KEY）
  - 敏感字段显示/隐藏切换功能
  - 保存成功/失败提示
  - 已保存配置的状态显示
- JavaScript 方法：
  - `openSystemConfigModal()`: 打开配置模态框
  - `loadSystemConfig()`: 加载当前配置
  - `saveLinuxDoOAuthConfig()`: 保存 OAuth 配置
  - `saveLinuxDoCreditConfig()`: 保存 Credit 配置

## 🚀 使用说明

### 1. 启动服务

```bash
cd /Users/amesky/Documents/github/chatgpt-panel
go run cmd/main.go
```

### 2. 访问系统设置

1. 登录管理后台：`http://localhost:8080`
2. 点击顶部的"系统设置"按钮
3. 在弹出的模态框中配置：
   - **Linux DO OAuth**：Client ID 和 Client Secret
   - **Linux DO Credit**：PID 和 KEY
4. 点击各自的"保存"按钮

### 3. 配置优先级

```
数据库配置（优先） > 环境变量
```

- 如果数据库中有配置，优先使用数据库值
- 如果数据库中没有配置，使用环境变量值
- 环境变量配置方式不变（兼容旧版本）

### 4. 敏感字段处理

- `Client Secret` 和 `KEY` 被标记为敏感字段
- 前端不显示完整内容，仅提示是否已保存
- 支持点击眼睛图标显示/隐藏输入内容
- 留空表示不修改当前配置

## 📝 环境变量配置（可选）

如果不使用数据库配置，可以继续使用环境变量（`.env` 文件）：

```env
# Linux DO OAuth
LINUXDO_CLIENT_ID=your_client_id
LINUXDO_CLIENT_SECRET=your_client_secret

# Linux DO Credit
LINUXDO_CREDIT_PID=your_pid
LINUXDO_CREDIT_KEY=your_key
```

## 🔐 权限控制

- 所有系统配置 API 端点在 `auth` 路由组中
- 需要登录才能访问
- 建议：后续可添加管理员角色验证中间件

## 📦 数据库表结构

### `system_configs` 表

| 字段 | 类型 | 说明 |
|------|------|------|
| id | uint | 主键 |
| key | string | 配置键名（唯一） |
| value | text | 配置值 |
| label | string | 配置标签 |
| category | string | 配置分类 |
| is_secret | bool | 是否为敏感信息 |
| created_at | timestamp | 创建时间 |
| updated_at | timestamp | 更新时间 |
| deleted_at | timestamp | 软删除时间 |

### 配置键名规范

- `linuxdo_oauth_client_id`
- `linuxdo_oauth_client_secret`
- `linuxdo_credit_pid`
- `linuxdo_credit_key`

## 🧪 测试建议

1. **配置保存测试**：
   - 打开系统设置模态框
   - 输入测试配置
   - 点击保存，验证成功提示

2. **配置优先级测试**：
   - 先设置环境变量
   - 再通过系统设置保存数据库配置
   - 重启服务，验证使用的是数据库配置

3. **敏感字段测试**：
   - 验证密码输入框的显示/隐藏功能
   - 验证已保存配置的状态提示

4. **OAuth 流程测试**：
   - 配置 Linux DO OAuth
   - 测试授权 URL 生成
   - 测试 Code 换取 Token

5. **Credit 支付测试**：
   - 配置 Linux DO Credit
   - 测试创建订单
   - 测试支付回调

## 📄 相关文件清单

### 新建文件
- `internal/models/system_config.go`
- `internal/services/system_config_service.go`
- `internal/handlers/system_config_handler.go`

### 修改文件
- `internal/models/db.go`
- `internal/config/config.go`
- `internal/services/linuxdo_oauth_service.go`
- `internal/services/credit_gateway_service.go`
- `cmd/main.go`
- `templates/index.html`

## ✨ 功能亮点

1. **无缝兼容**：现有环境变量配置方式完全保留
2. **优先级清晰**：数据库配置优先于环境变量
3. **用户友好**：Web 界面配置，无需修改文件和重启服务
4. **安全可靠**：敏感字段脱敏显示，支持显示/隐藏切换
5. **状态提示**：清晰显示配置是否已保存

