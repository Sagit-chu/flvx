# 2026-10-22 运行时漏洞排查与升级

## 排查结论

| 漏洞 | 本项目涉及范围 | 修复措施 |
| --- | --- | --- |
| CVE-2026-109618、CVE-2026-109619（PHP RCE） | 仓库无 PHP 源码、Composer 依赖或 PHP 容器。项目后端为 Go，前端生产容器为 Nginx 静态服务。 | 本项目无 PHP 运行时需要更新；同机部署的其他 PHP 服务需单独排查。转发 PHP 网站的流量不会修复目标网站的 PHP。 |
| CVE-2026-111497（Node.js Brotli 越界写入） | 前端 Docker 构建使用 Node.js 22；CI 原使用已停止支持的 Node.js 20。Node.js 用于构建、开发及 skill 安装/发布，不是面板生产服务端。未发现项目源码直接调用 Brotli 解码，但构建依赖和下载工具仍运行于 Node.js。 | Docker、CI 和 `.node-version` 固定为 22.23.4；包声明仅接受已修复的 22.23.4+、24.22.1+、26.12.1+ 分支，前端启用 `engine-strict`。 |
| CVE-2026-111943（Go crypto/x509 中间证书信任） | 后端的版本检查、升级下载、授权验证等使用 HTTPS；agent 的更新下载、TLS 拨号和证书验证使用 Go 标准库。原 Go 1.25 工具链缺少此次拒绝信任修复。 | 三个 Go 模块最低版本升至 1.26.10，CI 从对应 `go.mod` 读取，后端 Docker 使用 1.26.10-bookworm。 |

Go 漏洞涉及一组特定中间证书，并非拒绝所有包含八层中间证书的证书链。官方修复在 `crypto/x509.Certificate.Verify` 中拒绝包含这些证书的链。Go 1.27.0–1.27.2 也未修复；手动选择 Go 1.27 时必须使用 1.27.3+。`go.mod` 的最低版本无法排除该较新分支里的旧补丁版本，因此发布必须使用仓库固定的工具链。

## 让已部署实例获得修复

1. 使用本次修改后的构建配置发布新版本，重新构建后端及前端镜像、Linux amd64/arm64 agent。Go 标准库编译进二进制，仅更新宿主机 Go 或 `ca-certificates` 不会替换已有二进制中的标准库。已有镜像标签也不会因源码更新自动修复。
2. 在面板实际部署目录，将 `.env` 中的 `FLUX_VERSION` 设置为包含本次修复的**已发布版本**，再执行 `docker compose pull backend frontend` 和 `docker compose up -d backend frontend`。如使用独立构建或私有仓库，应部署对应的新镜像。
3. 将所有节点的 agent 更新为同一安全版本并重启服务，包括离线节点。使用解压后的构建产物执行 `go version -m /path/to/gost` 或 `go version -m /path/to/paneld`，核对构建版本为 go1.26.10（或已修复的更新版本）；UPX 压缩可能需要先对副本解压以读取构建信息。
4. 开发机和自托管构建机也需升级 Node.js。本次排查时本机为 v26.0.0，属于受影响版本；应升级至 26.12.1+，或切换到 `.node-version` 的 22.23.4。无需在运行静态前端的 Nginx 容器内安装 Node.js。

本次代码修改不表示远端部署已经更新；须完成新版本发布、镜像替换与节点升级后，才能确认线上获得修复。更新系统 CA 信任包可作为额外措施，但不能替代 Go 二进制重建。

## 本次验证

- Go 1.26.10 下，`go-backend`、`go-gost`、`go-gost/x` 各自的 `go test ./...` 全部通过。PostgreSQL 外部数据库契约测试仍需配置 `FLVX_POSTGRES_TEST_DSN` 后单独执行。
- Node.js 22.23.4、pnpm 10.28.1 下，前端 `pnpm install --frozen-lockfile` 与 `pnpm run build` 通过，无需更改依赖锁文件。
- 本机 Node.js 26.0.0 执行前端构建时，被 `engine-strict` 拒绝。
- 使用 `CGO_ENABLED=0` 交叉编译后端和 agent 的 Linux amd64/arm64 共四个二进制成功；`go version -m` 确认均为 go1.26.10。
- Docker Hub 上两个固定构建镜像标签均存在，且包含 Linux amd64/arm64。未执行 Docker 镜像构建或线上部署。
- 工作流 YAML、包 JSON 解析及 `git diff --check` 通过。

## 官方依据

- [PHP 官方安全公告](https://www.php.net/archive/2026.php#2026-10-21-4)
- [Node.js 2026-10-21 安全更新及修复版本](https://nodejs.org/en/blog/vulnerability/october-2026-security-releases)
- [Go 官方问题 #82345](https://go.dev/issue/82345)
- [Go 漏洞库 GO-2026-6809](https://pkg.go.dev/vuln/GO-2026-6809)
- [Go 官方发布记录](https://go.dev/doc/devel/release)
