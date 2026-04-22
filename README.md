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

默认会先执行探测，再自动执行 diagnostics。你也可以自定义采样次数：

```bash
aiprobe test --base-url https://example.com/v1 --api-key YOUR_KEY --samples 5 --format json
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

## 设计原则

- 优先零配置体验
- 尽量保守识别，宁可返回 `unknown` 也不乱猜
- 统一输出结构，方便后续脚本处理
- 不打印完整 API key

## 免责声明

不同代理和兼容层对“OpenAI-compatible”的实现程度可能不同，因此自动识别结果是基于当前规则与响应形状得到的保守判断。
