PackageIdentifier: gqcdm.aiprobe
PackageVersion: {{VERSION}}
InstallerType: zip
NestedInstallerType: portable
NestedInstallerFiles:
  - RelativeFilePath: aiprobe.exe
    PortableCommandAlias: aiprobe
Installers:
  - Architecture: x64
    InstallerUrl: https://github.com/gqcdm/aiprobe/releases/download/v{{VERSION}}/aiprobe-v{{VERSION}}-windows-amd64.zip
    InstallerSha256: {{SHA256_WINDOWS_AMD64}}
ManifestType: installer
ManifestVersion: 1.6.0
