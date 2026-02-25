# OpenRouter Key Monitor

一个用于管理和监控 OpenRouter API 密钥余额的 Web 仪表盘，支持多渠道通知推送。

## 功能特性

- **密钥管理** - 添加、编辑、删除、归档 OpenRouter API 密钥
- **余额监控** - 实时查看各密钥的额度、消耗量与剩余余额
- **分组管理** - 对密钥进行自定义分组，方便分类管理
- **余额历史** - 记录最近 90 天的余额变化趋势
- **利润统计** - 根据自定义收购价/出售价计算总利润与今日利润
- **自动归档** - 余额低于耗尽阈值时自动归档密钥
- **定时通知** - 按设定间隔定时推送余额报告到多个渠道
- **多渠道推送** - 支持企业微信、钉钉、飞书、邮件（SMTP）
- **自定义模板** - 支持自定义通知消息模板和密钥详情模板
- **登录鉴权** - 内置账号密码登录，Session 有效期 24 小时

## 快速开始

### 方式一：本地运行

**环境要求：** Node.js 18+

```bash
# 安装依赖
npm install

# 启动服务
npm start
```

服务启动后访问：http://localhost:3000

### 方式二：Docker 部署（推荐）

```bash
# 使用 Docker Compose 一键启动
docker-compose up -d
```

或使用 Docker 命令：

```bash
# 构建镜像
docker build -t ormonitor:latest .

# 运行容器
docker run -d \
  --name ormonitor \
  -p 3000:3000 \
  -v $(pwd)/data:/app/data \
  --restart unless-stopped \
  ormonitor:latest
```

详细 Docker 部署说明请参考 [README-Docker.md](./README-Docker.md)。

## 默认登录信息

| 字段   | 值         |
|--------|------------|
| 用户名 | `admin`    |
| 密码   | `admin123` |

> 首次登录后请在设置页面修改默认密码。

## 配置说明

### 环境变量

| 变量名     | 默认值       | 说明         |
|------------|--------------|--------------|
| `PORT`     | `3000`       | 服务监听端口 |
| `NODE_ENV` | `production` | 运行环境     |

### 数据存储

数据以 JSON 格式存储在 `./data/db.json`，包含以下内容：

- 密钥列表（`keys`）
- 系统设置（`settings`）
- 登录凭证（`auth`）
- 余额历史（`balanceHistory`，最多保留 90 天）

### 通知渠道配置

在设置页面可配置以下推送渠道：

| 渠道     | 配置项            |
|----------|-------------------|
| 企业微信 | Webhook URL       |
| 钉钉     | Webhook URL       |
| 飞书     | Webhook URL       |
| 邮件     | SMTP 主机、端口、发件人、收件人、密码 |

### 通知模板变量

自定义通知模板时可使用以下变量：

| 变量                     | 说明               |
|--------------------------|--------------------|
| `{{date}}`               | 当前时间（MM-DD HH:mm） |
| `{{totalQuota}}`         | 总额度             |
| `{{totalUsage}}`         | 总消耗             |
| `{{totalRemaining}}`     | 总剩余余额         |
| `{{totalRemainingCNY}}`  | 总剩余余额（人民币）|
| `{{totalRemainingPercent}}` | 总剩余百分比    |
| `{{totalDaily}}`         | 今日总消耗         |
| `{{totalProfit}}`        | 总利润（人民币）   |
| `{{totalDailyProfit}}`   | 今日利润（人民币） |
| `{{purchaseRate}}`       | 收购价             |
| `{{sellRate}}`           | 出售价             |
| `{{keys}}`               | 密钥详情列表       |

密钥详情模板额外支持：`{{index}}`、`{{name}}`、`{{key}}`、`{{quota}}`、`{{usage}}`、`{{remaining}}`、`{{remainingCNY}}`、`{{remainingPercent}}`、`{{daily}}`、`{{profit}}`、`{{dailyProfit}}`、`{{status}}`

## 技术栈

| 组件       | 说明                         |
|------------|------------------------------|
| Node.js    | 运行时环境                   |
| Express    | Web 服务框架                 |
| LowDB      | 轻量级 JSON 文件数据库       |
| nanoid     | 唯一 ID 生成                 |
| Docker     | 容器化部署                   |

## API 接口

| 方法   | 路径                        | 说明               |
|--------|-----------------------------|--------------------|
| POST   | `/api/auth/login`           | 登录               |
| GET    | `/api/auth/check`           | 检查登录状态       |
| POST   | `/api/auth/logout`          | 登出               |
| POST   | `/api/auth/update`          | 修改账号密码       |
| GET    | `/api/keys`                 | 获取所有密钥       |
| POST   | `/api/keys`                 | 添加密钥           |
| PUT    | `/api/keys/:id`             | 更新密钥           |
| DELETE | `/api/keys/:id`             | 删除密钥           |
| POST   | `/api/keys/:id/archive`     | 归档密钥           |
| DELETE | `/api/keys/:id/archive`     | 取消归档           |
| GET    | `/api/balance-history`      | 获取余额历史       |
| POST   | `/api/balance-history`      | 保存余额历史       |
| GET    | `/api/settings`             | 获取设置           |
| POST   | `/api/settings`             | 保存设置           |
| POST   | `/api/settings/template`    | 保存通知模板       |
| POST   | `/api/settings/price`       | 保存价格配置       |
| POST   | `/api/test-channel`         | 测试通知渠道       |

## 许可证

MIT
