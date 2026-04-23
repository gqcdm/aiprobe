# AGENTS.md

## 项目定位
- 这是一个**单一 Go CLI 仓库**，不是 monorepo。唯一可执行入口是 `cmd/aiprobe/main.go`。
- 真正的命令树、参数校验、退出码逻辑在 `internal/cli/app.go`；不要从 README 反推 CLI，优先以这里为准。

## 代码边界
- `internal/detect/`：provider 探测引擎与选择逻辑。
- `internal/providers/`：目前只注册 3 个适配器：`openai`、`anthropic`、`gemini`。
- `internal/diagnostics/`：`test` 命令的诊断与延迟采样。
- `internal/render/`：`text/json` 输出。
- `internal/schema/`：输出结构、错误码、脱敏规则；涉及输出格式时先看这里。
- `internal/httpx/`：URL 规范化、错误分类、HTTP 辅助逻辑。

## 开发与验证
- 固定验证入口是：`go test ./...`
- 本仓库当前没有独立 lint/typecheck 脚本；未来 agent 不要猜，先跑 `go test ./...`。
- 修改 CLI 行为后，除了测试，还要手动跑至少一个真实命令，例如：
  - `go run ./cmd/aiprobe --help`
  - `go run ./cmd/aiprobe detect --base-url <url> --api-key <key>`

## CLI 约束
- 根命令名是 `aiprobe`。
- 当前主子命令是：`detect`、`test`、`completion`。
- `detect` 必须带 `--base-url` 和 `--api-key`。
- `test` 必须带 `--base-url`、`--api-key`，且 `--samples > 0`。
- `completion` 现在依赖 **Cobra 内建补全生成**；如果补全行为异常，优先检查 `internal/cli/app.go`，不要再回退到手写脚本生成器。

## 输出与安全
- 输出结构和脱敏规则以 `internal/schema/schema.go` 为准。
- 这里明确做了 API key 脱敏；如果改 JSON/text 输出，必须保持脱敏行为不回退。
- `detect` 的策略是保守优先：证据冲突时返回 `unknown`/歧义，而不是强猜 provider。

## 发布与版本联动
- **版本变更不是只打 tag。** 目前至少要同步：Git tag、`PKGBUILD`、`.SRCINFO`。
- AUR 元数据不自动同步；`PKGBUILD` 和 `.SRCINFO` 必须人工保持一致。
- AUR source URL 绑定 Git tag，tag 形态必须是 `vX.Y.Z`。
- 默认使用中文发布：release 标题、release notes、发布说明、对外发布文案默认写中文；只有用户当次明确指定其他语言时才切换。

## GitHub Actions 分工
- `.github/workflows/release.yml`
  - 只在 `v*` tag push 时运行。
  - 先 `go test ./...`，再构建 `linux/darwin/windows` 二进制，并上传到 GitHub Release。
  - 这里上传的是**裸二进制**，不是压缩包。
- `.github/workflows/aur.yml`
  - 在 `v*` tag 或手动触发时运行。
  - 只校验 `PKGBUILD` / `.SRCINFO` 存在并打出 AUR 归档附件，**不会自动同步到 AUR 仓库**。
- `.github/workflows/package-distribution.yml`
  - 在 `v*` tag 或手动触发时运行。
  - 生成 `darwin/windows` 压缩包、`deb`、`rpm`、校验和，以及 `Homebrew / Scoop / winget` 模板。
  - 这里生成的是**发布附件和外部仓库模板**，不等于自动上架 Homebrew/Scoop/winget。

## 打包/分发 gotchas
- `release.yml` 和 `package-distribution.yml` 的产物形态不同：前者是裸二进制，后者是压缩包/包管理模板/系统包。不要混用预期。
- `package-distribution.yml` 当前 `checksums.txt` **不包含** Linux amd64 tar.gz；如果你改 release 资产或校验逻辑，先确认是否要补上。
- `packaging/homebrew/` 只引用 macOS tar.gz；`packaging/scoop/` 和 `packaging/winget/` 只引用 Windows zip。
- `packaging/nfpm.yaml.tpl` 是 `deb/rpm` 生成入口；涉及 Linux 包布局时优先改这里。

## 文档边界
- `README.md` 已经说明了：`Homebrew / Scoop / winget` 目前是**模板**，不是自动收录。
- 如果代码与 README 冲突，以代码和 workflow 为准；更新实现时顺手修 README，避免误导后续 agent。
