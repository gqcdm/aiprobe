# aiprobe

`aiprobe` 是一个用 Go 编写的命令行工具，用来快速探测 AI API 接口类型、列出模型，并测试接口是否可用以及大致延迟。

它的默认目标很简单：**用户只需要提供 `base URL` 和 `API key`**，工具会尽量自动识别接口风格，而不是要求手动选择 provider。

## 当前支持

- `OpenAI-compatible`
- `Anthropic`
- `Gemini`
- 无法确定时返回 `unknown`

## 安装

### 方式一：从源码运行

```bash
go run ./cmd/aiprobe --help
```

### 方式二：本地编译

```bash
go build -o aiprobe ./cmd/aiprobe
./aiprobe --help
```

### 方式三：通过 `yay` / AUR 安装

当 AUR 包发布后，可以直接：

```bash
yay -S aiprobe
```

仓库里已经包含 `PKGBUILD`、`.SRCINFO` 和对应的 GitHub Actions workflow，用来校验并产出 AUR 源码包描述文件。

### 方式四：主流系统包管理

项目现在会额外生成这些分发产物或清单模板：

- `Homebrew` formula 模板
- `Scoop` manifest 模板
- `winget` manifest 模板
- `deb` 包
- `rpm` 包

其中 `Homebrew / Scoop / winget` 默认产出的是可提交到各自外部仓库的清单模板，不代表这些源已经自动收录；`deb / rpm` 会直接生成包文件并作为 release 附件上传。

## 用法

### 1. 自动探测接口

```bash
aiprobe detect --base-url https://example.com/v1 --api-key YOUR_KEY
```

输出会展示：

- 推测出的 provider
- API type
- 置信度
- 规范化后的 URL
- 探测到的模型列表或模型数量

如果你想拿到结构化输出：

```bash
aiprobe detect --base-url https://example.com/v1 --api-key YOUR_KEY --format json
```

### 2. 测试可用性和延迟

```bash
aiprobe test --base-url https://example.com/v1 --api-key YOUR_KEY
```

如果你更想一条命令直接跑完整探测，也可以用快捷写法：

```bash
aiprobe -t https://example.com/v1 YOUR_KEY
```

默认会先执行探测，再自动执行 diagnostics。你也可以自定义采样次数：

```bash
aiprobe test --base-url https://example.com/v1 --api-key YOUR_KEY --samples 5 --format json
```

JSON 输出现在还会包含 `model_diagnostics`，用于展示每个探测到的模型是否可用，以及对应的首 token 延迟采样汇总（`ttft_ms`）。

### 3. 生成 shell 补全

支持这些主流 shell：

- `bash`
- `zsh`
- `fish`
- `powershell`

例如生成 `fish` 补全脚本：

```bash
aiprobe completion fish > ~/.config/fish/completions/aiprobe.fish
```

生成 `bash` 补全脚本：

```bash
aiprobe completion bash > ~/.local/share/bash-completion/completions/aiprobe
```

生成 `zsh` 补全脚本：

```bash
aiprobe completion zsh > ~/.zfunc/_aiprobe
```

生成 `PowerShell` 补全脚本：

```powershell
aiprobe completion powershell > aiprobe.ps1
```

## 输出说明

`detect` 和 `test` 都支持两种输出：

- `text`：适合终端直接看
- `json`：适合脚本和自动化

核心输出字段包括：

- `input`
- `normalized_base_url`
- `detection`
- `models`
- `diagnostics`
- `errors`
- `warnings`

## 验证

本项目的固定验证入口是：

```bash
go test ./...
```

## 自动发布

仓库已经包含 GitHub Actions 自动发布流程。

当你 push 一个形如 `v0.1.2` 的 tag 到 GitHub 时，workflow 会自动：

- 执行 `go test ./...`
- 交叉编译 `linux / darwin / windows`
- 创建对应的 GitHub Release
- 上传编译产物附件

另外还包含一个 AUR 打包 workflow，会在 tag 或手动触发时校验 `PKGBUILD` / `.SRCINFO`，并产出可用于 AUR 发布的源码包描述附件。

此外还包含一个主流分发 workflow，会在 tag 或手动触发时生成：

- `darwin` 压缩包（用于 `Homebrew`）
- `windows` 压缩包（用于 `Scoop` / `winget`）
- `deb`
- `rpm`
- 对应 `sha256` 校验和
- `Homebrew / Scoop / winget` 清单模板

这些模板和包文件都会作为 GitHub Release 附件上传，方便后续分发。

## 设计原则

- 优先零配置体验
- 尽量保守识别，宁可返回 `unknown` 也不乱猜
- 统一输出结构，方便后续脚本处理
- 不打印完整 API key

## 免责声明

不同代理和兼容层对“OpenAI-compatible”的实现程度可能不同，因此自动识别结果是基于当前规则与响应形状得到的保守判断。
