{
  "version": "{{VERSION}}",
  "description": "CLI for probing AI API providers, models, and diagnostics",
  "homepage": "https://github.com/gqcdm/aiprobe",
  "license": "MIT",
  "url": "https://github.com/gqcdm/aiprobe/releases/download/v{{VERSION}}/aiprobe-v{{VERSION}}-windows-amd64.zip",
  "hash": "{{SHA256_WINDOWS_AMD64}}",
  "bin": "aiprobe.exe",
  "checkver": {
    "github": "https://github.com/gqcdm/aiprobe"
  },
  "autoupdate": {
    "url": "https://github.com/gqcdm/aiprobe/releases/download/v$version/aiprobe-v$version-windows-amd64.zip"
  }
}
