pkgname=aiprobe
pkgver=0.1.3
pkgrel=1
pkgdesc="CLI for probing AI API providers, models, and diagnostics"
arch=('x86_64' 'aarch64')
url="https://github.com/gqcdm/aiprobe"
license=('MIT')
depends=()
makedepends=('go')
source=("${pkgname}-${pkgver}.tar.gz::${url}/archive/refs/tags/v${pkgver}.tar.gz")
sha256sums=('SKIP')

build() {
  cd "${srcdir}/${pkgname}-${pkgver}"
  go build -trimpath -ldflags="-s -w" -o "${pkgname}" ./cmd/aiprobe
}

check() {
  cd "${srcdir}/${pkgname}-${pkgver}"
  go test ./...
}

package() {
  cd "${srcdir}/${pkgname}-${pkgver}"
  install -Dm755 "${pkgname}" "${pkgdir}/usr/bin/${pkgname}"
  install -Dm644 README.md "${pkgdir}/usr/share/doc/${pkgname}/README.md"
}
