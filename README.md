> **维护分支（`calm`）：** 本 fork 维护 Claude 账号身份持久化与服务端统一客户端声明，初始基线为上游 0.2.21。**旧实例不能直接覆盖升级，须先核实并迁移现有身份。** 开发、构建和跟进上游以 [维护指南](docs/CALM_FORK.md) 为准。

# Sub2API · Calm

面向自用开发的 AI API 网关：统一管理上游账号，通过 API Key 提供模型访问，处理鉴权、请求转发、账号调度、并发限制和用量统计。

源自 [Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api)，原作者版权署名：Copyright (c) 2026 Wesley Liddick；保留原仓库 [LICENSE](LICENSE)。

## 业务能力

- 管理 OAuth、setup-token 和 API Key 等上游凭据，按平台、分组与模型权限选择账号。
- 提供 Anthropic Messages、OpenAI Chat Completions / Responses、Gemini 等协议入口；具体能力取决于账号类型与上游支持。
- 支持粘性会话、账号故障切换、用户级和账号级并发控制、请求与 Token 速率限制。
- 记录 Token 用量、成本和调用状态，通过 Web 后台管理账号、用户、API Key 与调度配置。
- 提供图片生成、编辑及异步图片任务；支付和订阅功能供需要时使用，自用可选择简易模式。

## 本 fork 的关键约束

- Claude OAuth / setup-token 的 ClientID 持久化在 PostgreSQL，不再以 Redis 指纹缓存作为身份权威。
- UA / SDK 声明由服务端统一生成，下游请求头不能覆盖。
- 新账号可并发安全地初始化身份；数据库错误、无效身份或既有 NULL 预留身份均拒绝继续请求，不随机补造身份。
- 旧实例升级前必须备份、暂停流量并核实迁移当前身份；不能靠清 Redis、删除身份行或直接换回上游镜像恢复。
- 已应用的迁移不改内容；回退必须另行制定保持当前身份的方案。详见 [维护指南](docs/CALM_FORK.md)。

## 本地开发与验证

技术栈：Go / Gin / Ent、Vue 3 / Vite / TailwindCSS、PostgreSQL 15+、Redis 7+。
Go 使用 [`backend/go.mod`](backend/go.mod) 要求的精确版本（当前为 1.27.4），前端使用 pnpm 和仓库锁文件，不为编译方便降级依赖。

前端开发（在 `frontend/` 执行）：

```bash
pnpm install --frozen-lockfile
pnpm run dev
```

后端开发（在 `backend/` 执行，需要独立开发用 PostgreSQL / Redis）：

```bash
go run ./cmd/server
```

首次初始化使用 setup 向导创建管理员；不要把配置模板里的默认管理员字段当作已创建的登录账号。
开发配置可参考 [`deploy/config.example.yaml`](deploy/config.example.yaml)，不要复用真实账号、身份或生产数据库。

相关单元与 race 检查（仓库根目录执行）：

```bash
bash scripts/calm/check.sh
```

真实 PostgreSQL 合约测试只使用显式创建的一次性本地数据库；回环地址也可能是生产隧道。测试入口、完整构建流程和已知测试限制见 [维护指南](docs/CALM_FORK.md)。

## 构建与部署边界

- 构建显式指定 fork 版本，记录源码 commit、工具链及产物哈希；内嵌前端的构建须先生成前端产物。
- [`deploy/`](deploy/) 中的安装脚本和 Compose 文件是继承的通用参考，不能当作本 fork 的直接升级入口。
- 不使用上游一键安装或后台自动更新覆盖本 fork，以免丢失身份补丁。
- 构建、提交和打标签不代表部署；实际部署配置、凭据、备份和恢复记录单独管理，不进入源码仓库。

## 自用配置与接入注意

- **简易模式：** `RUN_MODE=simple` 隐藏 SaaS 功能并跳过余额、订阅扣费；生产环境还需 `SIMPLE_MODE_CONFIRM=true`。
- **默认分组：** 简易模式默认在启动时补齐；自行管理时可设 `SIMPLE_MODE_AUTO_CREATE_DEFAULT_GROUPS=false`，不会删除已有分组。
- **密钥消费窗口：** 简易模式可用 `SIMPLE_MODE_KEY_RATE_LIMIT_ENABLED=true` 启用每个 API Key 的 5 小时、1 天、7 天窗口。请求完成后记账，并发请求可能超过上限，不能当作严格的费用硬上限。
- **Nginx：** 使用 Codex CLI 时在 `http` 块保留 `underscores_in_headers on;`，否则 `session_id` 等头可能被丢弃，影响粘性路由。
- **Codex Fast/Flex：** 网关策略处理请求体中的 `service_tier`，不控制客户端是否显示 `/fast`。`force_priority` 会影响实际计费；省略 tier 的请求需显式配置 `missing` 匹配规则。
- **Antigravity：** 专用入口为 `/antigravity/v1/messages` 和 `/antigravity/v1beta/`；Anthropic Claude 与 Antigravity Claude 不应混用同一对话上下文，使用分组隔离。
- **TypeSafe / Jev：** 使用原生非流式 `POST /v1/systemone`，不兼容 Messages、Chat Completions 或 Responses 客户端。
- **图片输出：** 上游不一定遵守请求的输出格式，保存前核验实际格式、扩展名和 MIME。成功响应即使格式不符也可能已计费，自动重试可能重复产生费用。

## 业务文档

| 主题 | 入口 |
|------|------|
| Fork 维护、身份迁移、测试与构建 | [CALM_FORK](docs/CALM_FORK.md) |
| OpenAI 图片接入 | [OPENAI_IMAGES_CODEX](docs/OPENAI_IMAGES_CODEX.md) |
| 异步图片任务 | [ASYNC_IMAGE_TASKS](docs/ASYNC_IMAGE_TASKS.md) |
| Seedance API | [seedance-api](docs/seedance-api.md) |
| 组合分组 | [COMPOSITE_GROUPS](docs/COMPOSITE_GROUPS.md) |
| 管理 API Key | [admin-api-keys](docs/admin-api-keys.md) |
| 支付配置（可选） | [PAYMENT_CN](docs/PAYMENT_CN.md) |
| 插件开发 | [PLUGIN_DEVELOPMENT](docs/PLUGIN_DEVELOPMENT.md) |

## 目录

```text
backend/          Go 后端、Ent 模型、迁移及测试
frontend/         Vue 管理界面
scripts/calm/     Fork 回归检查入口
deploy/           通用配置模板与部署参考
docs/             业务接入及维护文档
```
