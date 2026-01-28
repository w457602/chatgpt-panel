# ChatGPT 账号管理面板

一个用于管理 ChatGPT 账号的 Web 管理面板，支持账号导入、OAuth 授权、批量操作等功能。

## 功能特性

- ✅ 账号列表展示（邮箱、密码、Access Token、绑卡链接、状态等）
- ✅ 账号搜索、筛选、分页
- ✅ 批量操作（删除、状态更新）
- ✅ OAuth 授权按钮
- ✅ 账号详情查看
- ✅ 统计面板（总数、状态分布、今日新增等）
- ✅ JWT 用户认证
- ✅ RESTful API 支持外部导入

## 技术栈

- 后端：Go + Gin + GORM
- 数据库：PostgreSQL
- 前端：HTML5 + Tailwind CSS + Alpine.js

## 快速开始

### 1. 配置环境

```bash
cp .env.example .env
# 编辑 .env 配置数据库连接等信息
```

### 2. 创建数据库

```sql
CREATE DATABASE chatgpt_panel;
```

### 3. 运行服务

```bash
go run ./cmd/main.go
```

或编译后运行：

```bash
go build -o chatgpt-panel ./cmd/main.go
./chatgpt-panel
```

### 4. 访问面板

打开浏览器访问 http://localhost:8080

默认管理员账号：
- 用户名：admin
- 密码：admin123

## API 接口

### 认证

- `POST /api/v1/auth/login` - 登录获取 Token
- `GET /api/v1/auth/me` - 获取当前用户信息

### 账号管理

- `GET /api/v1/accounts` - 获取账号列表（支持分页、筛选）
- `GET /api/v1/accounts/stats` - 获取统计信息
- `GET /api/v1/accounts/:id` - 获取账号详情
- `POST /api/v1/accounts` - 创建账号
- `PUT /api/v1/accounts/:id` - 更新账号
- `DELETE /api/v1/accounts/:id` - 删除账号
- `POST /api/v1/accounts/batch-delete` - 批量删除
- `PATCH /api/v1/accounts/:id/status` - 更新状态
- `POST /api/v1/accounts/batch-status` - 批量更新状态
- `PATCH /api/v1/accounts/:id/refresh-token` - 更新 Refresh Token

### 账号导入（无需认证）

- `POST /api/v1/accounts/import` - 导入账号

导入请求示例：
```json
{
  "email": "test@example.com",
  "password": "password123",
  "access_token": "...",
  "checkout_url": "https://pay.openai.com/...",
  "status": "pending"
}
```

### 浏览器插件对接（可选）

用于根据绑卡页面 URL 查找账号邮箱，并在绑卡成功后更新状态。

- `GET /api/v1/extension/account?url=...` - 根据 checkout_url 查找账号（返回 id/email/status）
- `POST /api/v1/extension/billing-success` - 绑卡成功回传（body: `{"url":"...","account_id":123,"status":"active"}`)

如需开启扩展 Token 校验，请在 `.env` 中设置：

```
EXTENSION_TOKEN=your-extension-token
```

## 集成到注册脚本

在注册脚本中，注册成功后调用导入 API：

```python
import requests

def upload_to_panel(account_data):
    response = requests.post(
        "http://localhost:8080/api/v1/accounts/import",
        json=account_data
    )
    return response.json()
```

## 目录结构

```
chatgpt-panel/
├── cmd/
│   └── main.go              # 入口文件
├── internal/
│   ├── config/              # 配置
│   ├── handlers/            # HTTP 处理器
│   ├── middleware/          # 中间件（JWT认证等）
│   ├── models/              # 数据模型
│   └── services/            # 业务逻辑
├── templates/               # HTML 模板
├── static/                  # 静态资源
├── .env.example             # 环境变量示例
├── go.mod
└── README.md
```
