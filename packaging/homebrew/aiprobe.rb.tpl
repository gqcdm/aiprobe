class Aiprobe < Formula
  desc "CLI for probing AI API providers, models, and diagnostics"
  homepage "https://github.com/gqcdm/aiprobe"
  version "{{VERSION}}"
  license "MIT"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/gqcdm/aiprobe/releases/download/v{{VERSION}}/aiprobe-v{{VERSION}}-darwin-arm64.tar.gz"
      sha256 "{{SHA256_DARWIN_ARM64}}"
    else
      url "https://github.com/gqcdm/aiprobe/releases/download/v{{VERSION}}/aiprobe-v{{VERSION}}-darwin-amd64.tar.gz"
      sha256 "{{SHA256_DARWIN_AMD64}}"
    end
  end

  def install
    bin.install "aiprobe"
  end

  test do
    system "#{bin}/aiprobe", "--help"
  end
end
