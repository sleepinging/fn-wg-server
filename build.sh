#!/bin/bash
set -e
# 本地打包 fpk —— 复刻 fnpack build 的输出结构。
# 依赖：tar + md5sum（Git Bash / Linux 自带）。
# 产物：./wg-server_<version>_x86.fpk
#
# 用法：
#   bash build.sh            # 自动递增 patch 版本 (1.0.0 → 1.0.1)
#   bash build.sh --patch    # 同上，递增 patch
#   bash build.sh --minor    # 递增 minor (1.0.0 → 1.1.0)
#   bash build.sh --major    # 递增 major (1.0.0 → 2.0.0)
#   bash build.sh 1.2.3      # 指定具体版本

ROOT="$(cd "$(dirname "$0")" && pwd)"
APP="$ROOT/app"
PKG="$ROOT/pkg"
SKEL="$ROOT/.skel"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info(){ echo -e "${GREEN}[INFO]${NC} $1"; }
warn(){ echo -e "${YELLOW}[WARN]${NC} $1"; }
err(){ echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# ==================== 版本管理 ====================

MANIFEST_FILE="$PKG/manifest"
MAIN_GO_FILE="$APP/backend/main.go"
API_ROUTER_FILE="$APP/backend/api/router.go"

# 从 manifest 读取当前版本
get_current_version() {
    grep "^version" "$MANIFEST_FILE" | awk -F= '{print $2}' | tr -d ' '
}

# 写入 manifest 版本
set_manifest_version() {
    local ver="$1"
    sed -i.bak "s/^version.*/version               = ${ver}/" "$MANIFEST_FILE"
    rm -f "$MANIFEST_FILE.bak"
}

# 写入 Go 源码中的版本常量
set_go_version() {
    local ver="$1"
    # main.go
    sed -i.bak "s/const Version = \".*\"/const Version = \"${ver}\"/" "$MAIN_GO_FILE"
    rm -f "$MAIN_GO_FILE.bak"
    # api/router.go
    sed -i.bak "s/var Version = \".*\"/var Version = \"${ver}\"/" "$API_ROUTER_FILE"
    rm -f "$API_ROUTER_FILE.bak"
}

# 版本号自增
bump_version() {
    local current="$1"
    local part="$2"  # major, minor, patch
    IFS='.' read -r major minor patch <<< "$current"
    
    # Remove any non-numeric suffix (like -beta, etc.)
    patch="${patch%%[^0-9]*}"
    
    case "$part" in
        major)
            major=$((major + 1))
            minor=0
            patch=0
            ;;
        minor)
            minor=$((minor + 1))
            patch=0
            ;;
        patch|*)
            patch=$((patch + 1))
            ;;
    esac
    
    echo "${major}.${minor}.${patch}"
}

# ==================== 版本参数解析 ====================

CURRENT_VERSION=$(get_current_version)
BUMP_MODE="patch"

if [ $# -ge 1 ]; then
    case "$1" in
        --patch|--patch-version)
            BUMP_MODE="patch"
            ;;
        --minor|--minor-version)
            BUMP_MODE="minor"
            ;;
        --major|--major-version)
            BUMP_MODE="major"
            ;;
        --*)
            err "未知参数: $1\n用法: bash build.sh [--patch|--minor|--major|<版本号>]"
            ;;
        *)
            # 检查是否是有效的版本号格式 x.y.z
            if [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+ ]]; then
                NEW_VERSION="$1"
                BUMP_MODE=""
            else
                err "无效版本号: $1 (格式: x.y.z)"
            fi
            ;;
    esac
fi

if [ -n "$BUMP_MODE" ]; then
    NEW_VERSION=$(bump_version "$CURRENT_VERSION" "$BUMP_MODE")
fi

info "版本: ${CURRENT_VERSION} → ${NEW_VERSION}"

# 更新所有版本号
set_manifest_version "$NEW_VERSION"
set_go_version "$NEW_VERSION"

# ==================== 构建检查 ====================

[ -d "$APP" ] || err "缺少 app/"
[ -f "$MANIFEST_FILE" ] || err "缺少 $MANIFEST_FILE"
[ -d "$SKEL/cmd" ] || err "缺少 .skel/cmd（fnpack 骨架脚本存档）"

APPNAME=$(grep "^appname" "$MANIFEST_FILE" | awk -F= '{print $2}' | tr -d ' ')
VERSION=$(get_current_version)

W=$(mktemp -d); trap 'rm -rf "$W"' EXIT
P="$W/pkg"; mkdir -p "$P/cmd" "$P/config" "$P/wizard" "$P/ui/images"

# 1. 构建 Go 后端
info "构建 Go 后端..."
cd "$APP/backend"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o wg-server .
cd "$ROOT"

# 2. 构建前端
info "构建前端..."
if command -v npm &> /dev/null; then
    if [ -f "$APP/ui-src/package.json" ]; then
        cd "$APP/ui-src"
        npm install --registry=https://registry.npmmirror.com 2>/dev/null || npm install
        npm run build
        cd "$ROOT"
        # npm build 会清空 ui/ 目录（emptyOutDir: true），确保 index.cgi 存在
        if [ ! -f "$APP/ui/index.cgi" ]; then
            cat > "$APP/ui/index.cgi" << 'CGIEOF'
#!/bin/bash
APP_DEST="${TRIM_APPDEST:-/var/apps/wg-server/target}"
PKG_VAR="${TRIM_PKGVAR:-/var/apps/wg-server/var}"
export UI_DIR="${APP_DEST}/ui"
export TRIM_PKGVAR="${PKG_VAR}"
exec "${APP_DEST}/backend/wg-server"
CGIEOF
            chmod +x "$APP/ui/index.cgi"
        fi
    fi
else
    warn "npm 未安装，跳过前端构建（使用已编译的前端）"
    if [ ! -d "$APP/ui" ] || [ ! -f "$APP/ui/index.html" ]; then
        mkdir -p "$APP/ui"
        cp "$ROOT/build-fallback.html" "$APP/ui/index.html" 2>/dev/null || true
    fi
fi

# 3. app.tgz —— 在临时目录组装完整 app/
info "打包 app.tgz ..."
ATMP="$W/app"; mkdir -p "$ATMP/ui/images" "$ATMP/backend"
cp -a "$APP/ui/." "$ATMP/ui/"
cp "$PKG/ui/config" "$ATMP/ui/config"
cp "$PKG/ui/images/"* "$ATMP/ui/images/" 2>/dev/null || true
cp "$APP/backend/wg-server" "$ATMP/backend/wg-server"
rm -rf "$ATMP/ui/__pycache__" "$ATMP/ui-src"
chmod +x "$ATMP/ui/index.cgi" "$ATMP/backend/wg-server" 2>/dev/null || true
( cd "$ATMP" && tar --exclude='__pycache__' --exclude='*.pyc' --exclude='*.go' --exclude='go.mod' --exclude='go.sum' -czf "$P/app.tgz" . )

# 4. cmd：先铺 .skel 默认 9 脚本，再用 pkg/cmd 覆盖
cp "$SKEL/cmd/"* "$P/cmd/"
[ -d "$PKG/cmd" ] && cp "$PKG/cmd/"* "$P/cmd/" 2>/dev/null || true
chmod +x "$P/cmd/"* 2>/dev/null || true

# 5. config
cp "$PKG/config/privilege" "$P/config/"
cp "$PKG/config/resource"  "$P/config/"

# 6. 图标
cp "$PKG/ICON.PNG" "$P/" 2>/dev/null || true
cp "$PKG/ICON_256.PNG" "$P/" 2>/dev/null || true
cp "$PKG/ICON.PNG" "$P/ui/images/icon_64.png" 2>/dev/null || true
cp "$PKG/ICON_256.PNG" "$P/ui/images/icon_256.png" 2>/dev/null || true

# 7. manifest + checksum
CK=$(md5sum "$P/app.tgz" | cut -d' ' -f1)
cp "$MANIFEST_FILE" "$P/manifest"
grep -q "^checksum" "$P/manifest" \
  && sed -i.bak "s/^checksum.*/checksum              = ${CK}/" "$P/manifest" \
  || printf "checksum              = %s\n" "$CK" >> "$P/manifest"
rm -f "$P/manifest.bak"

# 8. 校验 + 打包
for f in app.tgz manifest cmd config; do
  [ -e "$P/$f" ] || err "校验失败：缺 $f"
done

# 清理旧的 fpk 文件（只保留最近 5 个）
OLD_FPKS=$(ls -t "$ROOT"/*.fpk 2>/dev/null | tail -n +6)
if [ -n "$OLD_FPKS" ]; then
    info "清理旧 fpk 文件..."
    rm -f $OLD_FPKS
fi

FPK="${APPNAME}_${VERSION}_x86.fpk"
rm -f "$ROOT/$FPK"
( cd "$P" && tar -czf "$ROOT/$FPK" * )

info "构建完成：${FPK} ($(du -h "$ROOT/$FPK" | cut -f1))，checksum=${CK}"
echo ""
echo "  📦 ${ROOT}/${FPK}"
echo "  📌 ${CURRENT_VERSION} → ${VERSION}"
echo ""
