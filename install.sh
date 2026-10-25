#!/usr/bin/env bash
#
# PWFE - Arch Linux kurulum ve güncelleme betiği
#
# Kullanım:
#   sudo bash install.sh
#   sudo bash install.sh --branch main
#   sudo bash install.sh --update-password 'yeni-sifre'
#   sudo bash install.sh --uninstall [--purge]
#
set -euo pipefail

REPO_URL="https://github.com/kinderlinux/PWFE.git"
BRANCH="main"
UPDATE_REPO=""
DEFAULT_UPDATE_REPO="kinderlinux/PWFE"
PREFIX="/opt/pwfe"
CONFIG_DIR="/etc/pwfe"
UPDATE_PASSWORD=""
ASSUME_YES=0
FORCE=0
NO_DEPS=0
DO_UNINSTALL=0
PURGE=0
LOCAL_SOURCE=0

info() { printf '\033[1;34m[PWFE]\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m[PWFE]\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m[PWFE]\033[0m %s\n' "$*" >&2; exit 1; }

# GitHub URL'sinden "sahip/depo" biçimini çıkarır.
github_slug() {
    local url="$1"
    url="${url%/}"
    url="${url%.git}"
    case "$url" in
        https://github.com/*) printf '%s' "${url#https://github.com/}" ;;
        http://github.com/*)  printf '%s' "${url#http://github.com/}" ;;
        git@github.com:*)      printf '%s' "${url#git@github.com:}" ;;
        ssh://git@github.com/*) printf '%s' "${url#ssh://git@github.com/}" ;;
        *) return 1 ;;
    esac
}

valid_slug() {
    local LC_ALL=C
    [[ "$1" =~ ^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$ ]]
}

valid_branch() {
    local LC_ALL=C
    [[ "$1" =~ ^[A-Za-z0-9._/-]+$ ]]
}

usage() {
    cat <<'EOF'
PWFE kurulum betiği (Arch Linux)

Kullanım:
  sudo bash install.sh [seçenekler]

Seçenekler:
  --branch <dal>             Kurulacak git dalı (varsayılan: main)
  --repo <url>               Alternatif git deposu
  --local                    Betiğin yanındaki kaynak ağacından kur (test/geliştirme)
  --update-repo <sahip/depo> Güncellemelerin izleyeceği GitHub deposu
  --update-password <şifre>  Güncelleme şifresini belirle
  --no-deps                  Eksik paketleri pacman ile kurma
  -y, --yes                  Onay sormadan devam et
  --force                    Arch Linux kontrolünü atla
  --uninstall                PWFE'yi kaldır (config korunur)
  --purge                    --uninstall ile config'i de sil
  -h, --help                 Bu yardımı göster
EOF
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --branch)
            [[ $# -ge 2 ]] || die "--branch için değer gerekli"
            BRANCH="$2"; shift 2 ;;
        --branch=*)
            BRANCH="${1#*=}"; shift ;;
        --repo)
            [[ $# -ge 2 ]] || die "--repo için değer gerekli"
            REPO_URL="$2"; shift 2 ;;
        --repo=*)
            REPO_URL="${1#*=}"; shift ;;
        --local)
            LOCAL_SOURCE=1; shift ;;
        --update-repo)
            [[ $# -ge 2 ]] || die "--update-repo için değer gerekli"
            UPDATE_REPO="$2"; shift 2 ;;
        --update-repo=*)
            UPDATE_REPO="${1#*=}"; shift ;;
        --update-password)
            [[ $# -ge 2 ]] || die "--update-password için değer gerekli"
            UPDATE_PASSWORD="$2"; shift 2 ;;
        --update-password=*)
            UPDATE_PASSWORD="${1#*=}"; shift ;;
        --no-deps)
            NO_DEPS=1; shift ;;
        -y|--yes)
            ASSUME_YES=1; shift ;;
        --force)
            FORCE=1; shift ;;
        --uninstall)
            DO_UNINSTALL=1; shift ;;
        --purge)
            PURGE=1; shift ;;
        -h|--help)
            usage; exit 0 ;;
        *)
            die "Bilinmeyen parametre: $1 (--help)" ;;
    esac
done

[[ ${EUID:-$(id -u)} -eq 0 ]] || die "Bu betik root olarak çalıştırılmalı: sudo bash install.sh"

valid_branch "$BRANCH" || die "Geçersiz dal adı: $BRANCH"

if [[ -n "$UPDATE_REPO" ]]; then
    UPDATE_REPO="${UPDATE_REPO%.git}"
    valid_slug "$UPDATE_REPO" || die "Geçersiz --update-repo değeri: $UPDATE_REPO (örn. sahip/depo)"
elif UPDATE_REPO="$(github_slug "$REPO_URL")" && valid_slug "$UPDATE_REPO"; then
    :
else
    warn "GitHub deposu tanınamadı; güncellemeler varsayılan olarak $DEFAULT_UPDATE_REPO kullanacak. Değiştirmek için --update-repo kullanın."
    UPDATE_REPO="$DEFAULT_UPDATE_REPO"
fi

uninstall_pwfe() {
    info "PWFE kaldırılıyor..."
    systemctl disable --now pwfe.service 2>/dev/null || true
    systemctl stop pwfe-update.service 2>/dev/null || true
    rm -f /etc/systemd/system/pwfe.service
    rm -f /etc/systemd/system/pwfe-update.service
    systemctl daemon-reload 2>/dev/null || true
    rm -rf "$PREFIX"
    if [[ $PURGE -eq 1 ]]; then
        rm -rf "$CONFIG_DIR"
        info "Config ve güncelleme şifresi silindi."
    else
        info "Config korundu: $CONFIG_DIR"
    fi
    info "PWFE kaldırıldı."
}

if [[ $DO_UNINSTALL -eq 1 ]]; then
    uninstall_pwfe
    exit 0
fi

# Arch Linux kontrolü
if [[ -r /etc/os-release ]]; then
    # shellcheck disable=SC1091
    . /etc/os-release
    if [[ "${ID:-}" != "arch" && " ${ID_LIKE:-} " != *" arch "* && $FORCE -eq 0 ]]; then
        die "Bu betik Arch Linux için hazırlanmıştır. Yine de denemek için --force kullanın."
    fi
elif [[ $FORCE -eq 0 ]]; then
    die "/etc/os-release bulunamadı. Arch Linux için --force kullanın."
fi

confirm() {
    local prompt="$1" answer=""
    if [[ $ASSUME_YES -eq 1 ]]; then
        return 0
    fi
    if [[ -r /dev/tty ]]; then
        read -r -p "$prompt [E/h] " answer </dev/tty || true
    elif [[ -t 0 ]]; then
        read -r -p "$prompt [E/h] " answer || true
    else
        return 1
    fi
    case "${answer:-e}" in
        e|E|y|Y|evet|yes) return 0 ;;
        *) return 1 ;;
    esac
}

# Gerekli araçlar: go, git, curl
missing=()
for cmd in go git curl; do
    command -v "$cmd" >/dev/null 2>&1 || missing+=("$cmd")
done

if [[ ${#missing[@]} -gt 0 ]]; then
    warn "Eksik araçlar: ${missing[*]}"
    if [[ $NO_DEPS -eq 1 ]]; then
        die "--no-deps verildi. Önce şunları kurun: sudo pacman -S --needed ${missing[*]}"
    fi
    command -v pacman >/dev/null 2>&1 || die "pacman bulunamadı. Şunları kurun: ${missing[*]}"
    if ! confirm "Eksik paketler pacman ile kurulsun mu? (${missing[*]})"; then
        die "Kurulum iptal edildi. Elle kurmak için: sudo pacman -S --needed ${missing[*]}"
    fi
    info "Paketler kuruluyor: ${missing[*]}"
    if ! pacman -S --needed --noconfirm "${missing[@]}"; then
        die "pacman başarısız oldu. Önce 'sudo pacman -Syu' çalıştırıp tekrar deneyin."
    fi
fi

command -v systemctl >/dev/null 2>&1 || die "systemctl bulunamadı. Bu sistem systemd kullanmıyor."

GO_VERSION="$(go env GOVERSION 2>/dev/null | sed 's/^go//' || true)"
if [[ -z "$GO_VERSION" ]]; then
    GO_VERSION="$(go version | awk '{print $3}' | sed 's/^go//')"
fi
if [[ "$(printf '%s\n%s\n' "1.18" "$GO_VERSION" | sort -V | head -n1)" != "1.18" ]]; then
    die "Go 1.18 veya üzeri gerekli. Bulunan: $GO_VERSION"
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

if [[ $LOCAL_SOURCE -eq 1 ]]; then
    SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-}")" && pwd)"
    SRC_DIR="$SCRIPT_DIR"
    [[ -f "$SRC_DIR/main.go" && -f "$SRC_DIR/updater/main.go" ]] || die "--local: betiğin yanında main.go ve updater/main.go bulunamadı."
    info "Yerel kaynak kullanılıyor: $SRC_DIR"
else
    info "Kaynak kodu alınıyor: $REPO_URL ($BRANCH)"
    git clone --depth 1 --single-branch --branch "$BRANCH" "$REPO_URL" "$TMP_DIR/src"
    SRC_DIR="$TMP_DIR/src"
fi

[[ -f "$SRC_DIR/version.txt" ]] || die "version.txt bulunamadı: $SRC_DIR"
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

OLD_VERSION=""
if [[ -f "$PREFIX/version.txt" ]]; then
    OLD_VERSION="$(tr -d '[:space:]' < "$PREFIX/version.txt" || true)"
fi
if systemctl is-active --quiet pwfe.service 2>/dev/null; then
    SERVICE_WAS_ACTIVE=1
fi

backup_current() {
    local version="$1"
    [[ -n "$version" ]] || return 0
    [[ -f "$PREFIX/pwfe" ]] || return 0
    local backup_dir="$PREFIX/backups/$version"
    mkdir -p "$backup_dir"
    cp -f "$PREFIX/pwfe" "$backup_dir/pwfe"
    [[ -f "$PREFIX/static/index.html" ]] && cp -f "$PREFIX/static/index.html" "$backup_dir/index.html"
    [[ -f "$PREFIX/version.txt" ]] && cp -f "$PREFIX/version.txt" "$backup_dir/version.txt"
    info "Mevcut sürüm yedeklendi: $backup_dir"
}

restore_current() {
    local version="$1"
    local backup_dir="$PREFIX/backups/$version"
    [[ -f "$backup_dir/pwfe" ]] || return 1
    [[ $SERVICE_WAS_ACTIVE -eq 1 ]] && systemctl stop pwfe.service 2>/dev/null || true
    cp -f "$backup_dir/pwfe" "$PREFIX/pwfe"
    if [[ -f "$backup_dir/index.html" ]]; then
        cp -f "$backup_dir/index.html" "$PREFIX/static/index.html"
    fi
    if [[ -f "$backup_dir/version.txt" ]]; then
        cp -f "$backup_dir/version.txt" "$PREFIX/version.txt"
    fi
    systemctl restart pwfe.service 2>/dev/null || true
    return 0
}

# En yeni 3 yedeği tutar.
prune_backups() {
    local dir="$PREFIX/backups" versions=() name
    [[ -d "$dir" ]] || return 0
    while IFS= read -r name; do
        versions+=("$name")
    done < <(find "$dir" -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | grep -E '^[0-9]+$' | sort -n)
    local i=0
    while (( i < ${#versions[@]} - 3 )); do
        rm -rf "$dir/${versions[$i]}"
        i=$((i + 1))
    done
}

if [[ -n "$OLD_VERSION" ]]; then
    backup_current "$OLD_VERSION"
fi

info "Dosyalar kuruluyor: $PREFIX"
install -d -m 0755 "$PREFIX" "$PREFIX/static" "$PREFIX/backups" "$PREFIX/.update" "$CONFIG_DIR"

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

GENERATED_PASSWORD=""
if [[ -n "$UPDATE_PASSWORD" ]]; then
    PWFE_CONFIG="$CONFIG_DIR/config.json" "$PREFIX/pwfe" --set-update-password "$UPDATE_PASSWORD" >/dev/null
    info "Güncelleme şifresi belirlendi."
elif [[ ! -f "$CONFIG_DIR/update.passwd" ]]; then
    # Şifreyi komut satırında göstermemek için üretimi pwfe'ye bırak.
    PASSWORD_OUTPUT="$(PWFE_CONFIG="$CONFIG_DIR/config.json" "$PREFIX/pwfe" --set-update-password)"
    GENERATED_PASSWORD="$(printf '%s\n' "$PASSWORD_OUTPUT" | sed -n 's/^Güncelleme şifresi: //p')"
    [[ ${#GENERATED_PASSWORD} -eq 16 ]] || die "Güncelleme şifresi üretilemedi."
    info "Güncelleme şifresi oluşturuldu."
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
Environment=PWFE_ROOT=$PREFIX
Environment=PWFE_CONFIG=$CONFIG_DIR/config.json
Environment=PWFE_UPDATE_REPO=$UPDATE_REPO
Environment=PWFE_UPDATE_BRANCH=$BRANCH
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
Environment=PWFE_ROOT=$PREFIX
Environment=PWFE_UPDATE_REPO=$UPDATE_REPO
Environment=PWFE_UPDATE_BRANCH=$BRANCH
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
    if [[ -n "$OLD_VERSION" ]] && restore_current "$OLD_VERSION"; then
        die "Yeni sürüm başlatılamadı, eski sürüme dönüldü."
    fi
    die "Kurulum başarısız."
fi

INSTALL_OK=1
prune_backups

IP_ADDR="$(ip route get 1.1.1.1 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i == "src") { print $(i+1); exit }}' || true)"
if [[ -z "$IP_ADDR" ]]; then
    IP_ADDR="$(hostname -I 2>/dev/null | awk '{print $1}' || true)"
fi
[[ -n "$IP_ADDR" ]] || IP_ADDR="<sunucu-ip>"

echo
info "PWFE v$VERSION kuruldu."
echo "  Arayüz      : http://$IP_ADDR:$PORT"
echo "  Ayarlar     : $CONFIG_DIR/config.json"
echo "  Güncelleyici: $PREFIX/pwfe-update"
echo "  Yedekler    : $PREFIX/backups"
echo "  Günlük      : $PREFIX/update.log"
if [[ -n "$GENERATED_PASSWORD" ]]; then
    echo "  Güncelleme şifresi: $GENERATED_PASSWORD"
    echo "  (Bu şifreyi not edin. Değiştirmek için: sudo $PREFIX/pwfe --set-update-password 'yeni')"
fi
echo
echo "  Güncelleme kontrolü : sudo $PREFIX/pwfe-update --check"
echo "  Güncelleme          : sudo systemctl start pwfe-update.service"
echo "  Geri dönüş          : sudo $PREFIX/pwfe-update --rollback"
echo
info "Servis açılışta otomatik başlar; kullanıcı oturumu gerekmez."
