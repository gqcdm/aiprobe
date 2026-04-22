package cli

import "github.com/gqcdm/aiprobe/internal/app"

var rootHelp = app.Name + ` auto-detects AI API providers and diagnostics.

Usage:
  aiprobe <command> [flags]

Commands:
  detect    Detect provider, API type, and model list metadata
  test      Run detection plus diagnostics and latency checks
  help      Show help for aiprobe or a subcommand

Flags:
  -h, --help   Show help

Use "aiprobe <command> --help" for more information about a command.
`

const detectHelp = `Detect provider, API type, and model list metadata.

Usage:
  aiprobe detect --base-url <url> --api-key <key> [flags]

Flags:
  --format <text|json>   Output format
  --provider <name>      Optional provider hint
  --type <name>          Optional API type hint
  -h, --help   Show help for detect
`

const testHelp = `Run detection plus diagnostics and latency checks.

Usage:
  aiprobe test --base-url <url> --api-key <key> [flags]

Flags:
  --format <text|json>   Output format
  --samples <n>          Latency sample count, default 3
  -h, --help   Show help for test
`
