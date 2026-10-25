#!/usr/bin/env bash
#
# PWFE - Arch Linux kurulum ve kaldırma betiği
#
# Kullanım:
#   sudo bash install.sh
#   sudo bash install.sh --uninstall
#
set -euo pipefail

REPO_URL="https://github.com/kinderlinux/PWFE.git"
BRANCH="main"
PREFIX="/opt/pwfe"
CONFIG_DIR="/etc/pwfe"
DO_UNINSTALL=0

info() { printf '\033[1;34m[PWFE]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[PWFE]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[PWFE]\033[0m %s\n' "$*" >&2; exit 1; }

usage() {
    cat <<'EOF'
PWFE kurulum betiği (Arch Linux)

Kullanım:
  sudo bash install.sh
  sudo bash install.sh --uninstall

Seçenekler:
  --uninstall   PWFE'yi ve ayarlarını kaldır
  -h, --help    Bu yardımı göster
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --uninstall) DO_UNINSTALL=1; shift ;;
        -h|--help)   usage; exit 0 ;;
        *)           die "Bilinmeyen parametre: $1 (--help)" ;;
    esac
done

[[ ${EUID:-$(id -u)} -eq 0 ]] || die "Bu betik root olarak çalıştırılmalı: sudo bash install.sh"

uninstall_pwfe() {
    info "PWFE kaldırılıyor..."
    systemctl disable --now pwfe.service 2>/dev/null || true
    systemctl stop pwfe-update.service 2>/dev/null || true
    rm -f /etc/systemd/system/pwfe.service
    rm -f /etc/systemd/system/pwfe-update.service
    systemctl daemon-reload 2>/dev/null || true
    rm -rf "$PREFIX" "$CONFIG_DIR"
    info "PWFE, config ve güncelleme şifresi silindi."
}

if [[ $DO_UNINSTALL -eq 1 ]]; then
    uninstall_pwfe
    exit 0
fi

# Arch Linux kontrolü
if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    if [[ "${ID:-}" != "arch" && " ${ID_LIKE:-} " != *" arch "* ]]; then
        die "Bu betik Arch Linux için hazırlanmıştır."
    fi
else
    die "/etc/os-release bulunamadı. Bu betik Arch Linux için hazırlanmıştır."
fi

confirm() {
    local answer=""
    if [[ -r /dev/tty ]]; then
        read -r -p "$1 [E/h] " answer </dev/tty || true
    elif [[ -t 0 ]]; then
        read -r -p "$1 [E/h] " answer || true
    else
        return 1
    fi
    case "${answer:-e}" in
        e|E|y|Y|evet|yes) return 0 ;;
        *) return 1 ;;
    esac
}

read_password() {
    local prompt="$1" var="$2" value=""
    if [[ -r /dev/tty ]]; then
        read -r -s -p "$prompt" value </dev/tty || true
    elif [[ -t 0 ]]; then
        read -r -s -p "$prompt" value || true
    else
        die "Güncelleme şifresi için terminal gerekli."
    fi
    printf '\n' >&2
    printf -v "$var" '%s' "$value"
}

# Gerekli araçlar: go, git, curl
missing=()
for cmd in go git curl; do
    command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
done

if [[ ${#missing[@]} -gt 0 ]]; then
    warn "Eksik araçlar: ${missing[*]}"
    command -v pacman >/dev/null 2>&1 || die "pacman bulunamadı. Şunları kurun: ${missing[*]}"
    if ! confirm "Eksik paketler pacman ile kurulsun mu? (${missing[*]})"; then
        die "Kurulum iptal edildi. Elle kurmak için: sudo pacman -S --needed ${missing[*]}"
    fi
    info "Paketler kuruluyor: ${missing[*]}"
    pacman -S --needed --noconfirm "${missing[@]}" || die "pacman başarısız oldu."
fi

command -v systemctl >/dev/null 2>&1 || die "systemctl bulunamadı. Bu sistem systemd kullanmıyor."

# Şifre yalnızca ilk kurulumda sorulur; mevcut şifre güncellemelerde korunur.
UPDATE_PASSWORD=""
NEED_PASSWORD=0
if [[ ! -f "$CONFIG_DIR/update.passwd" ]]; then
    NEED_PASSWORD=1
    read_password "Güncelleme şifresi: " UPDATE_PASSWORD
    read_password "Güncelleme şifresi (tekrar): " UPDATE_PASSWORD_CONFIRM
    [[ -n "$UPDATE_PASSWORD" ]] || die "Güncelleme şifresi boş olamaz."
    [[ "$UPDATE_PASSWORD" == "$UPDATE_PASSWORD_CONFIRM" ]] || die "Güncelleme şifreleri eşleşmiyor."
    unset UPDATE_PASSWORD_CONFIRM
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/pwfe-install.XXXXXX")"
SERVICE_WAS_ACTIVE=0
INSTALL_OK=0

cleanup() {
    if [[ $SERVICE_WAS_ACTIVE -eq 1 && $INSTALL_OK -eq 0 ]]; then
        warn "Kurulum tamamlanmadı, eski servis başlatılmaya çalışılıyor..."
        systemctl restart pwfe.service 2>/dev/null || true
    fi
    rm -rf "$TMP_DIR"
}
trap cleanup EXIT

info "Kaynak kodu alınıyor: $REPO_URL ($BRANCH)"
git clone --depth 1 --single-branch --branch "$BRANCH" "$REPO_URL" "$TMP_DIR/src"
SRC_DIR="$TMP_DIR/src"

[[ -f "$SRC_DIR/version.txt" ]] || die "version.txt bulunamadı."
VERSION="$(tr -d '[:space:]' < "$SRC_DIR/version.txt")"
[[ "$VERSION" =~ ^[0-9]+$ ]] || die "Geçersiz sürüm: $VERSION"

info "PWFE v$VERSION derleniyor..."
export GOTOOLCHAIN=local GOPROXY=off CGO_ENABLED=0 GO111MODULE=on
(
    cd "$SRC_DIR"
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$TMP_DIR/pwfe" .
)
(
    cd "$SRC_DIR"
    go build -trimpath -ldflags "-s -w -X main.updaterVersion=$VERSION" -o "$TMP_DIR/pwfe-update" ./updater
)

[[ "$("$TMP_DIR/pwfe" --version)" == "$VERSION" ]] || die "Derlenen binary sürümü hatalı."
[[ "$("$TMP_DIR/pwfe-update" --version)" == "$VERSION" ]] || die "Derlenen güncelleyici sürümü hatalı."

if systemctl is-active --quiet pwfe.service 2>/dev/null; then
    SERVICE_WAS_ACTIVE=1
fi

info "Dosyalar kuruluyor: $PREFIX"
install -d -m 0755 "$PREFIX" "$PREFIX/static" "$PREFIX/.update" "$CONFIG_DIR"

if [[ $SERVICE_WAS_ACTIVE -eq 1 ]]; then
    systemctl stop pwfe.service
fi

cp -f "$TMP_DIR/pwfe" "$PREFIX/pwfe.new"
chmod 0755 "$PREFIX/pwfe.new"
mv -f "$PREFIX/pwfe.new" "$PREFIX/pwfe"

cp -f "$TMP_DIR/pwfe-update" "$PREFIX/pwfe-update.new"
chmod 0755 "$PREFIX/pwfe-update.new"
mv -f "$PREFIX/pwfe-update.new" "$PREFIX/pwfe-update"

cp -f "$SRC_DIR/static/index.html" "$PREFIX/static/index.html.new"
chmod 0644 "$PREFIX/static/index.html.new"
mv -f "$PREFIX/static/index.html.new" "$PREFIX/static/index.html"

printf '%s\n' "$VERSION" > "$PREFIX/version.txt.new"
mv -f "$PREFIX/version.txt.new" "$PREFIX/version.txt"

if [[ ! -f "$CONFIG_DIR/config.json" ]]; then
    cat > "$CONFIG_DIR/config.json" <<'JSON'
{
  "port": 7933,
  "directories": []
}
JSON
    chmod 0644 "$CONFIG_DIR/config.json"
    info "Varsayılan config oluşturuldu: $CONFIG_DIR/config.json"
fi

if [[ $NEED_PASSWORD -eq 1 ]]; then
    PWFE_CONFIG="$CONFIG_DIR/config.json" "$PREFIX/pwfe" --set-update-password "$UPDATE_PASSWORD" >/dev/null
    unset UPDATE_PASSWORD
    info "Güncelleme şifresi belirlendi."
else
    info "Mevcut güncelleme şifresi korunuyor."
fi

cat > /etc/systemd/system/pwfe.service <<EOF
[Unit]
Description=Panda Web File Explorer (PWFE)
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$PREFIX
ExecStart=$PREFIX/pwfe
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/pwfe-update.service <<EOF
[Unit]
Description=PWFE Guncelleyici
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
User=root
WorkingDirectory=$PREFIX
ExecStart=$PREFIX/pwfe-update --apply
EOF

systemctl daemon-reload
systemctl enable pwfe.service >/dev/null
systemctl restart pwfe.service

PORT="$(sed -n 's/.*"port"[[:space:]]*:[[:space:]]*\([0-9][0-9]*\).*/\1/p' "$CONFIG_DIR/config.json" | head -n1)"
PORT="${PORT:-7933}"

HEALTH_OK=0
for _ in {1..10}; do
    if systemctl is-active --quiet pwfe.service 2>/dev/null && \
       curl -fsS --max-time 2 "http://127.0.0.1:$PORT/api/version" >/dev/null 2>&1; then
        HEALTH_OK=1
        break
    fi
    sleep 1
done

if [[ $HEALTH_OK -ne 1 ]]; then
    warn "Servis sağlık kontrolü başarısız. Günlük: journalctl -u pwfe.service -n 50"
    die "Kurulum başarısız."
fi

INSTALL_OK=1

IP_ADDR="$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i == "src") { print $(i+1); exit }}' || true)"
if [[ -z "$IP_ADDR" ]]; then
    IP_ADDR="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
fi
[[ -n "$IP_ADDR" ]] || IP_ADDR="<sunucu-ip>"

echo
info "PWFE v$VERSION kuruldu."
echo "  Arayüz : http://$IP_ADDR:$PORT"
echo "  Ayarlar: $CONFIG_DIR/config.json"
echo
info "Servis açılışta otomatik başlar; kullanıcı oturumu gerekmez."
info "KinderLinux tarafından ❤️"
