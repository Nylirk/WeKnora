<p align="center">
  <picture>
    <img src="./docs/images/logo.png" alt="SelKnora Logo" height="120"/>
  </picture>
</p>

<p align="center">
    <a href="./LICENSE">
        <img src="https://img.shields.io/badge/License-MIT-ffffff?labelColor=d4eaf7&color=2e6cc4" alt="License">
    </a>
</p>

<p align="center">
  <h4 align="center">

  [项目介绍](#-项目介绍) • [上游来源](#-上游来源) • [核心特性](#-核心特性) • [快速开始](#-快速开始) • [开发指南](#-开发指南)

  </h4>
</p>

# SelKnora

## 项目介绍

**SelKnora** 是基于 [Tencent/WeKnora](https://github.com/Tencent/WeKnora) 派生的开源知识管理框架，延续 WeKnora 的 RAG 知识问答、Agent 推理、Wiki 模式等核心能力，并在此基础上进行定制化开发与维护。

框架围绕三大核心能力构建：**RAG 快速问答**适合日常知识查询，**ReAct Agent 智能推理**自主编排知识检索、MCP 工具与网络搜索完成复杂多步任务，全新的 **Wiki 模式**则让 Agent 从原始文档中自治生成相互链接的 Markdown 知识库与可视化知识图谱。结合多源数据接入（飞书 / Notion / 语雀，更多持续接入中）、二十余家主流模型厂商集成、Langfuse 全链路可观测性、**企业级多租户 RBAC（四级角色矩阵 + 资源归属 + 租户审计日志）**，以及完全可私有化部署的模块化架构，SelKnora 帮助团队把分散文档沉淀为可查询、可推理、可持续演进的专属知识资产。

## 上游来源

SelKnora 基于 [Tencent/WeKnora](https://github.com/Tencent/WeKnora) 派生开发。原项目遵循 MIT License，本项目继续遵循 MIT License 发布。相关版权与许可声明见仓库根目录 LICENSE 文件。

## ✨ 最新更新

- **v0.6.2** —— 按批次解析配置（`process_config`）+ 上传确认对话框；文档重新解析（reparse）支持覆盖配置；`weknora` CLI v0.9（内置 Agent Skills、`session stop`、auth/profile 统一）；知识库框选多选；pgvector 1024 维 HNSW 索引；对话资源 Store 重构；仅保留 Langfuse 追踪（移除 Jaeger）。详见 [`CHANGELOG.md`](./CHANGELOG.md)。
- **v0.6.1** —— 文档解析追踪时间线（Langfuse 风格 Span 树，逐阶段进度展示 + 解析中止）；OpenSearch 向量库驱动；YAML 声明式内置模型配置；系统管理员与统一平台设置 + 审计日志；新用户引导；设置页 UI 重构；`weknora` CLI v0.7 / v0.8（Agent 优先线协议、NDJSON、`--dry-run`）；OpenDataLoader 与 PaddleOCR-VL 解析引擎；MCP Server 多传输（stdio / SSE / HTTP）；按模型的思考模式配置；腾讯云 LKEAP 重排 + 原生 Gemini Embedding + MiniMax-M3。详见 [`CHANGELOG.md`](./CHANGELOG.md)。
- **v0.6.0** —— 租户 RBAC（四级角色矩阵 `Owner` / `Admin` / `Contributor` / `Viewer` + 按 KB 归属 + 每租户审计日志）、租户成员管理与多工作区 UX、自助创建工作区；`weknora` CLI v0.4 正式版 + `mcp serve`；KB 检索跨向量库扇出；MCP / 数据源凭据 AES-256-GCM 加密 + docreader gRPC TLS + Token；新增智谱 Embedding 与华为云 OBS；服务端用户偏好；Go 1.26.0。详见 [`docs/RBAC说明.md`](./docs/RBAC说明.md) 与 [`CHANGELOG.md`](./CHANGELOG.md)。
- **v0.5.2** —— Wiki 入库支撑万级文档知识库（任务队列 + 死信队列）；MCP 工具人机审批；Anthropic / Apache Doris / 腾讯云 VectorDB / 金山云 KS3 / SearXNG 后端；自适应三层分块 + 实时调试面板；全局 ⌘K 命令面板；语雀连接器 + 微信小程序；`weknora` CLI 早期版本。
- **v0.5.1** —— 知识库批量管理；租户级 IM 频道总览；会话搜索 + 用户级置顶；模型 / 网页搜索 / MCP 统一卡片化设置；按 Agent LLM 调用超时；桌面端租户切换。
- **v0.5.0** —— Wiki 模式正式版 —— Agent 从原始文档自治生成结构化、相互链接的 Markdown Wiki 页面及知识图谱；Wiki 浏览器 + 可视化图谱。
- **v0.4.0** —— WeKnora Cloud（托管模型 + 解析）；Chrome 插件；ClawHub Skill；微信 IM；附件处理；Azure OpenAI / 阿里云 OSS；Notion 连接器；百度 + Ollama 网页搜索；VectorStore 管理。
- **v0.3.6** —— ASR 语音；飞书数据源自动同步；OIDC；IM 引用回复 + 线程会话；文档自动摘要；Tavily 搜索；并行工具调用；Agent @提及范围限制。
- **v0.3.5** —— Telegram / 钉钉 / Mattermost IM；IM 斜杠命令 + QA 队列；推荐问题；VLM 自动描述 MCP 返回图片；Novita AI；来源频道标记。
- **v0.3.4** —— 企业微信 / 飞书 / Slack IM；多模态图片；NVIDIA 模型 API；Weaviate；AWS S3；AES-256-GCM API Key 加密；内置 MCP 服务；混合检索优化；`final_answer` 工具。
- **v0.3.3** —— 父子分块；知识库置顶；兜底回复；Rerank 段落清洗；存储桶自动创建；Milvus。
- **v0.3.2** —— 知识搜索入口；按来源配置解析与存储引擎；本地存储图片渲染；文档预览；火山引擎 TOS；Mermaid 渲染；对话批量管理；记忆图谱预览。
- **v0.3.0** —— 共享空间；Agent Skills + 沙盒执行；自定义 Agent；数据分析 Agent；思考模式；Bing / Google 搜索；API Key 认证；Helm Chart；韩语 i18n；Qdrant。
- **v0.2.0** —— Agent 模式（ReACT）；多类型知识库（FAQ + 文档）；对话策略配置；DuckDuckGo 网页搜索；MCP 工具集成；全新 UI + Agent 模式切换；MQ 异步任务管理。


## 📱 功能展示

<table>
  <tr>
    <td colspan="2" align="center"><b>💬 智能问答对话</b><br/><img src="./docs/images/qa.png" alt="智能问答对话" width="100%"></td>
  </tr>
  <tr>
    <td width="50%" align="center"><b>📖 Wiki 浏览器</b><br/><img src="./docs/images/wiki-browser.png" alt="Wiki 浏览器" width="100%"></td>
    <td width="50%" align="center"><b>🕸️ Wiki 知识图谱</b><br/><img src="./docs/images/wiki-graph.png" alt="Wiki 知识图谱" width="100%"></td>
  </tr>
  <tr>
    <td width="50%" align="center"><b>🤖 Agent 模式 · 工具调用过程</b><br/><img src="./docs/images/agent-qa.png" alt="Agent 模式工具调用过程" width="100%"></td>
    <td width="50%" align="center"><b>⚙️ 对话设置</b><br/><img src="./docs/images/settings.png" alt="对话设置" width="100%"></td>
  </tr>
  <tr>
    <td colspan="2" align="center"><b>🔭 监控可观测性 · Langfuse Tracing</b><br/><img src="./docs/images/langfuse.png" alt="Langfuse Tracing" width="100%"></td>
  </tr>
</table>

## 🏗️ 架构设计

![selknora-architecture.png](./docs/images/architecture.png)

从文档解析、向量化、检索到大模型推理，全流程模块化解耦，组件可灵活替换与扩展。支持本地 / 私有云部署，数据完全自主可控，零门槛 Web UI 快速上手。

## 🧩 核心特性

**智能对话**

| 能力 | 详情 |
|------|------|
| 智能推理 | ReACT 渐进式多步推理，自主编排知识检索、MCP 工具与网络搜索，支持自定义智能体 |
| 快速问答 | 基于知识库的 RAG 问答，快速准确地回答问题 |
| Wiki 模式 | Agent 驱动从原始文档中自动生成并维护结构化、相互链接的 Markdown Wiki 知识页面 |
| 工具调用 | 内置工具、MCP 工具、网络搜索 |
| 对话策略 | 在线 Prompt 编辑、检索阈值调节、多轮上下文感知 |
| 推荐问题 | 基于知识库内容自动生成推荐问题 |

**知识管理**

| 能力 | 详情 |
|------|------|
| 知识库类型 | FAQ / 文档 / Wiki / 题库，支持文件夹导入、URL 导入、标签管理、在线录入 |
| 按批次解析配置 | 上传确认对话框或 `process_config` API 覆盖解析引擎、分块、多模态（VLM / ASR）、图谱抽取与问题生成；支持 reparse 时调整配置 |
| 数据源导入 | 飞书 / Notion / 语雀 知识库自动同步（更多数据源开发中），支持增量与全量同步 |
| 文档格式 | PDF / Word / Txt / Markdown / HTML / 图片 / CSV / Excel / PPT / JSON |
| 检索策略 | BM25 稀疏召回 / Dense 稠密召回 / GraphRAG 图谱增强 / 父子分块 / pgvector HNSW 加速（1024 维）/ 多维度索引 |
| 批量选择 | 知识库文档列表支持框选（marquee）多选，便于批量操作 |
| 端到端测试 | 检索+生成全链路可视化，评估召回命中率、BLEU / ROUGE 等指标 |

**集成与扩展**

| 能力 | 详情 |
|------|------|
| 模型厂商 | OpenAI / Azure OpenAI / Anthropic（Claude）/ DeepSeek / Qwen（阿里云）/ 智谱 / 混元 / 豆包（火山引擎）/ Gemini / MiniMax / NVIDIA / Novita AI / SiliconFlow / OpenRouter / Ollama |
| 向量数据库 | PostgreSQL (pgvector) / Elasticsearch / OpenSearch / Milvus / Weaviate / Qdrant / Apache Doris / 腾讯云 VectorDB |
| Embedding | Ollama / BGE / GTE / 智谱 / OpenAI 兼容接口 |
| 对象存储 | 本地 / MinIO / AWS S3 / 火山引擎 TOS / 阿里云 OSS / 金山云 KS3 / 华为云 OBS |
| IM 集成 | 企业微信 / 飞书 / Slack / Telegram / 钉钉 / Mattermost / 微信 |
| 网络搜索 | DuckDuckGo / Bing / Google / Tavily / Baidu / Ollama / SearXNG |


**平台能力**

| 能力 | 详情 |
|------|------|
| 部署 | 本地 / Docker / Kubernetes (Helm)，支持私有化离线部署 |
| 界面 | Web UI / RESTful API / 命令行（`weknora`） |
| 权限控制 | 租户 RBAC 四级角色矩阵（Owner / Admin / Contributor / Viewer），按知识库的资源归属，每租户审计日志，invite-only 准入，自助创建工作区，跨租户超级管理员 |
| 安全 | API Key 与 MCP / 数据源凭据 AES-256-GCM 静态加密、支持平滑密钥轮换；app ↔ docreader gRPC TLS + Token；防 SSRF HTTP 客户端；Agent 技能沙箱隔离 |
| 可观测性 | 集成 Langfuse（唯一追踪后端）以追踪 ReAct 循环、Token 消耗、工具调用和任务流水线；内置 Langfuse 风格的文档解析追踪时间线，逐阶段展示解析进度 |
| 任务管理 | MQ 异步任务，版本升级自动数据库迁移 |
| 模型管理 | 集中配置，YAML 声明式内置模型配置，知识库级别模型选择，按模型思考模式配置，多租户共享内置模型 |

## 🚀 快速开始

### 🛠 环境要求

- [Docker](https://www.docker.com/) & [Docker Compose](https://docs.docker.com/compose/)
- [Git](https://git-scm.com/)

### 📦 安装与启动

```bash
git clone https://github.com/Nylirk/SelKnora.git
cd SelKnora
cp .env.example .env   # 按需编辑 .env，详见文件内注释
docker compose up -d   # 启动核心服务
```

启动成功后访问 **http://localhost** 即可使用。

> 如需使用本地 Ollama 模型，请先运行 `ollama serve > /dev/null 2>&1 &`

### 🔧 可选服务（Docker Compose Profile）

按需添加 `--profile` 启动额外组件，多个 profile 可叠加使用：

| Profile | 说明 | 启动命令 |
|---------|------|----------|
| _(默认)_ | 核心服务 | `docker compose up -d` |
| `full` | 全部功能 | `docker compose --profile full up -d` |
| `neo4j` | 知识图谱 (Neo4j) | `docker compose --profile neo4j up -d` |
| `minio` | 对象存储 (MinIO) | `docker compose --profile minio up -d` |
| `langfuse` | 链路追踪 (Langfuse) | `docker compose --profile langfuse up -d` |

组合示例：`docker compose --profile neo4j --profile minio up -d`

停止服务：`docker compose down`

### 🌐 服务地址

| 服务 | 地址 |
|------|------|
| Web UI | `http://localhost` |
| 后端 API | `http://localhost:8080` |
| 链路追踪 (Langfuse) | `http://localhost:3000` |

## 文档知识图谱

SelKnora 支持将文档转化为知识图谱，展示文档中不同段落之间的关联关系。开启知识图谱功能后，系统会分析并构建文档内部的语义关联网络，不仅帮助用户理解文档内容，还为索引和检索提供结构化支撑，提升检索结果的相关性和广度。

具体配置请参考 [知识图谱配置说明](./docs/KnowledgeGraph.md) 进行相关配置。

## 配套MCP服务器

请参考 [MCP配置说明](./mcp-server/MCP_CONFIG.md) 进行相关配置。

## 📘 文档

常见问题排查：[常见问题排查](./docs/QA.md)

详细接口说明请参考：[API 文档](./docs/api/README.md)

产品规划与计划：[路线图 (Roadmap)](./docs/ROADMAP.md)

## 🧭 开发指南

### ⚡ 快速开发模式（推荐）

如果你需要频繁修改代码，**不需要每次重新构建 Docker 镜像**！使用快速开发模式：

```bash
# 启动基础设施
make dev-start

# 启动后端（新终端）
make dev-app

# 启动前端（新终端）
make dev-frontend
```

**开发优势：**

- ✅ 前端修改自动热重载（无需重启）
- ✅ 后端修改快速重启（5-10秒，支持 Air 热重载）
- ✅ 无需重新构建 Docker 镜像
- ✅ 支持 IDE 断点调试

**详细文档：** [开发环境快速入门](./docs/开发指南.md)


## 🤝 贡献指南

欢迎通过 [Issue](https://github.com/Nylirk/SelKnora/issues) 反馈问题或提交 Pull Request。

**流程：** Fork → 新建分支 → 提交更改 → 创建 PR

**规范：** 使用 `gofmt` 格式化代码，遵循 [Conventional Commits](https://www.conventionalcommits.org/) 提交（`feat:` / `fix:` / `docs:` / `test:` / `refactor:`）

## 🔒 安全声明

**重要提示：** 从 v0.1.3 版本开始，系统提供登录鉴权功能，以增强安全性。在生产环境部署时，强烈建议：

- 将服务部署在内网/私有网络环境中，而非公网环境
- 避免将服务直接暴露在公网上，以防止重要信息泄露风险
- 为部署环境配置适当的防火墙规则和访问控制
- 定期更新到最新版本以获取安全补丁和改进

## 👥 贡献者

感谢上游项目及所有优秀的贡献者们：

[![Contributors](https://contrib.rocks/image?repo=Tencent/WeKnora)](https://github.com/Tencent/WeKnora/graphs/contributors)

## 📄 许可证

SelKnora 遵循 MIT License。详情见 [LICENSE](./LICENSE) 文件。

SelKnora 基于 Tencent/WeKnora 派生开发。原项目遵循 MIT License，本项目继续遵循 MIT License 发布。相关版权与许可声明见仓库根目录 LICENSE 文件。