name: aiprobe
arch: amd64
platform: linux
version: {{VERSION}}
section: utils
priority: optional
maintainer: gqcdm
description: |
  CLI for probing AI API providers, models, and diagnostics.
vendor: gqcdm
homepage: https://github.com/gqcdm/aiprobe
license: MIT
contents:
  - src: ./dist/linux/aiprobe
    dst: /usr/bin/aiprobe
  - src: ./README.md
    dst: /usr/share/doc/aiprobe/README.md
