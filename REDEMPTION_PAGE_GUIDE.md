# 兑换页面使用指南

## 📄 文件位置

- **兑换页面**: `/templates/redeem.html`
- **访问路径**: `http://your-domain/redeem.html`

---

## 🎯 功能说明

兑换页面为普通用户提供了一个简洁的界面，用于兑换 ChatGPT Team 名额。

### 支持的渠道

1. **通用渠道（common）**
   - 适用于通用兑换码
   - 仅需输入：邮箱 + 兑换码

2. **Linux DO 渠道（linux-do）**
   - 适用于 Linux DO 论坛用户
   - 需要输入：邮箱 + 兑换码 + Linux DO UID

---

## 🔧 使用流程

### 用户兑换步骤

1. 访问兑换页面：`http://your-domain/redeem.html`
2. 选择兑换渠道（通用渠道 或 Linux DO）
3. 输入邮箱地址（用于登录 ChatGPT 的邮箱）
4. 输入兑换码（格式：XXXX-XXXX-XXXX，自动转大写）
5. 如果是 Linux DO 渠道，需要输入论坛 UID
6. 点击"立即兑换"按钮
7. 等待兑换结果（成功后显示 Team 账号邮箱）
8. 查收邮箱中的 ChatGPT Team 邀请邮件

---

## 📋 表单字段说明

| 字段 | 必填 | 说明 | 示例 |
|------|------|------|------|
| **兑换渠道** | ✅ | 选择兑换码所属的渠道 | 通用渠道 / Linux DO |
| **邮箱地址** | ✅ | 用于登录 ChatGPT 的邮箱 | name@example.com |
| **兑换码** | ✅ | 12位兑换码（自动格式化） | ABCD-1234-EFGH |
| **Linux DO UID** | ⚠️ | Linux DO 渠道必填 | 12345 |

---

## 🔄 后端 API

### 端点
```
POST /api/v1/redemption/redeem
```

### 请求体（通用渠道）
```json
{
  "email": "user@example.com",
  "code": "ABCD-1234-EFGH",
  "channel": "common"
}
```

### 请求体（Linux DO 渠道）
```json
{
  "email": "user@example.com",
  "code": "ABCD-1234-EFGH",
  "channel": "linux-do",
  "redeemer_uid": "12345"
}
```

### 成功响应
```json
{
  "message": "兑换成功",
  "data": {
    "success": true,
    "message": "兑换成功",
    "account_email": "team@example.com",
    "invite_id": "inv_xxxxx",
    "status_code": 200
  }
}
```

### 错误响应
```json
{
  "error": "兑换码不存在或已失效"
}
```

---

## 🎨 UI 特性

- ✅ **简洁实用**：使用 Tailwind CSS，界面清晰易用
- ✅ **实时验证**：邮箱、兑换码格式实时校验
- ✅ **自动格式化**：兑换码输入时自动添加分隔符
- ✅ **错误提示**：友好的错误消息，突出显示错误字段
- ✅ **成功反馈**：显示 Team 账号邮箱，10秒后自动清除
- ✅ **加载状态**：提交时显示加载动画，防止重复提交
- ✅ **响应式设计**：支持移动端和桌面端

---

## ⚠️ 常见错误处理

| 错误 | 原因 | 解决方法 |
|------|------|---------|
| `请输入邮箱地址` | 邮箱为空 | 填写有效的邮箱地址 |
| `请输入有效的邮箱地址` | 邮箱格式不正确 | 检查邮箱格式 |
| `请输入兑换码` | 兑换码为空 | 填写兑换码 |
| `兑换码格式不正确` | 格式不符合要求 | 使用 XXXX-XXXX-XXXX 格式 |
| `Linux DO 渠道需要填写论坛 UID` | Linux DO 渠道未填写 UID | 填写论坛 UID |
| `兑换码不存在或已失效` | 兑换码无效或已使用 | 联系管理员 |
| `暂无可用账号` | 系统无可用 Team 账号 | 联系管理员 |
| `网络错误` | 网络连接问题 | 检查网络后重试 |

---

## 🚀 部署说明

### 方式 1：直接访问（推荐）
兑换页面为独立的静态 HTML 文件，用户可以直接通过浏览器访问：
```
http://your-domain/redeem.html
```

### 方式 2：嵌入到现有系统
如果需要在管理后台添加兑换入口，可以在导航栏添加链接：
```html
<a href="/redeem.html" target="_blank">兑换 Team 名额</a>
```

---

## 📝 技术栈

- **前端框架**: Alpine.js 3.x
- **CSS 框架**: Tailwind CSS 3.x
- **图标库**: Font Awesome 6.4
- **后端 API**: Go + Gin（已实现）

---

## 🔐 安全说明

- ✅ 公开接口，无需登录
- ✅ 后端验证所有输入参数
- ✅ 兑换码一次性使用（兑换后自动失效）
- ✅ 支持渠道匹配验证
- ✅ 支持降级账号逻辑（warranty vs no-warranty）

---

## 📞 技术支持

如有问题，请联系系统管理员。

