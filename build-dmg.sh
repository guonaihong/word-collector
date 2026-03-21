#!/bin/bash

# Word Collector DMG 打包脚本
# 自动编译、创建 .app 并打包成 DMG

set -e

# 配置
APP_NAME="Word Collector"
VERSION="1.0.0"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$SCRIPT_DIR/build"
DIST_DIR="$SCRIPT_DIR/dist"
APP_BUNDLE="$DIST_DIR/$APP_NAME.app"
DMG_NAME="WordCollector-$VERSION.dmg"
DMG_PATH="$DIST_DIR/$DMG_NAME"

# 颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔══════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   Word Collector DMG Builder         ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════╝${NC}"
echo ""

# ========== 1. 清理旧的构建 ==========
echo -e "${YELLOW}📦 清理旧的构建...${NC}"
rm -rf "$BUILD_DIR" "$DIST_DIR"
mkdir -p "$BUILD_DIR" "$DIST_DIR"

# ========== 2. 编译 Go 程序 ==========
echo -e "${YELLOW}📦 编译 Go 程序...${NC}"
cd "$SCRIPT_DIR"

# 获取 Go 依赖
go mod tidy 2>/dev/null || true

# 检测当前架构
CURRENT_ARCH=$(uname -m)
echo "当前架构: $CURRENT_ARCH"

# 编译 CLI 版本 (通用二进制)
echo "编译 CLI arm64..."
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BUILD_DIR/word-collector-arm64" ./cmd/cli/

echo "编译 CLI amd64..."
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o "$BUILD_DIR/word-collector-amd64" ./cmd/cli/

echo "合并 CLI 通用二进制..."
lipo -create "$BUILD_DIR/word-collector-arm64" "$BUILD_DIR/word-collector-amd64" -output "$BUILD_DIR/word-collector"

# 编译 GUI 版本 (仅当前架构，因为 CGO 依赖)
echo "编译 GUI ($CURRENT_ARCH)..."
# Fyne 需要 CGO，不能交叉编译
if [ "$CURRENT_ARCH" = "arm64" ]; then
    GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -ldflags="-s -w" -o "$BUILD_DIR/word-collector-gui" ./cmd/gui/
else
    GOOS=darwin GOARCH=amd64 CGO_ENABLED=1 go build -ldflags="-s -w" -o "$BUILD_DIR/word-collector-gui" ./cmd/gui/
fi

echo -e "${GREEN}✓ 编译完成${NC}"

# ========== 3. 创建 .app 包结构 ==========
echo -e "${YELLOW}📦 创建 .app 包...${NC}"

mkdir -p "$APP_BUNDLE/Contents/MacOS"
mkdir -p "$APP_BUNDLE/Contents/Resources"

# 复制 GUI 可执行文件
cp "$BUILD_DIR/word-collector-gui" "$APP_BUNDLE/Contents/MacOS/Word Collector"
chmod +x "$APP_BUNDLE/Contents/MacOS/Word Collector"

# 复制 CLI 版本到 Resources（供 Hammerspoon 调用）
cp "$BUILD_DIR/word-collector" "$APP_BUNDLE/Contents/Resources/word-collector"
chmod +x "$APP_BUNDLE/Contents/Resources/word-collector"

# 创建 Info.plist
cat > "$APP_BUNDLE/Contents/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleDevelopmentRegion</key>
    <string>zh_CN</string>
    <key>CFBundleExecutable</key>
    <string>Word Collector</string>
    <key>CFBundleIconFile</key>
    <string>AppIcon</string>
    <key>CFBundleIdentifier</key>
    <string>com.wordcollector.app</string>
    <key>CFBundleInfoDictionaryVersion</key>
    <string>6.0</string>
    <key>CFBundleName</key>
    <string>Word Collector</string>
    <key>CFBundleDisplayName</key>
    <string>Word Collector</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>$VERSION</string>
    <key>CFBundleVersion</key>
    <string>$VERSION</string>
    <key>LSMinimumSystemVersion</key>
    <string>10.15</string>
    <key>NSHighResolutionCapable</key>
    <true/>
    <key>NSHumanReadableCopyright</key>
    <string>Copyright © 2024 Word Collector. All rights reserved.</string>
    <key>LSUIElement</key>
    <false/>
    <key>NSAppleEventsUsageDescription</key>
    <string>Word Collector needs to control other applications to collect words.</string>
</dict>
</plist>
EOF

# ========== 4. 创建应用图标 ==========
echo -e "${YELLOW}📦 创建应用图标...${NC}"

ICON_PATH="$APP_BUNDLE/Contents/Resources/AppIcon.icns"

# 使用系统默认图标
if [ -f "/System/Library/CoreServices/CoreTypes.bundle/Contents/Resources/GenericApplicationIcon.icns" ]; then
    cp /System/Library/CoreServices/CoreTypes.bundle/Contents/Resources/GenericApplicationIcon.icns "$ICON_PATH"
    echo -e "${GREEN}✓ 使用系统默认图标${NC}"
fi

# ========== 5. 创建 DMG ==========
echo -e "${YELLOW}📦 创建 DMG 安装包...${NC}"

# 创建临时 DMG 目录
DMG_TEMP="$BUILD_DIR/dmg_temp"
mkdir -p "$DMG_TEMP"

# 复制 .app
cp -R "$APP_BUNDLE" "$DMG_TEMP/"

# 创建 Applications 快捷方式
ln -sf /Applications "$DMG_TEMP/Applications"

# 创建 README
cat > "$DMG_TEMP/README.txt" << EOF
Word Collector $VERSION
======================

安装方法:
1. 将 Word Collector 拖到 Applications 文件夹
2. 打开 Applications 中的 Word Collector

使用方法:
- GUI 界面：直接打开 Word Collector 应用
- 快捷键取词：配合 Hammerspoon 使用

GUI 功能:
- 输入单词添加到 Anki
- 从剪贴板粘贴单词
- 启用/暂停切换
- 打开单词收集目录

Hammerspoon 快捷键（需单独配置）:
- ⌃⌘W：取词

GitHub: https://github.com/guonaihong/word-collector
EOF

# 删除旧的 DMG
rm -f "$DMG_PATH"

# 创建 DMG (使用 hdiutil)
if hdiutil create -volname "Word Collector" \
    -srcfolder "$DMG_TEMP" \
    -ov -format UDZO \
    -imagekey zlib-level=9 \
    "$DMG_PATH"; then
    echo -e "${GREEN}✓ DMG 创建成功${NC}"
else
    echo -e "${RED}❌ DMG 创建失败${NC}"
    exit 1
fi

# ========== 6. 复制 CLI 到 ~/word-collector/ ==========
echo -e "${YELLOW}📦 安装 CLI 工具...${NC}"
mkdir -p "$HOME/word-collector"
cp "$BUILD_DIR/word-collector" "$HOME/word-collector/"
echo -e "${GREEN}✓ CLI 已安装到 ~/word-collector/word-collector${NC}"

# ========== 7. 显示结果 ==========
echo ""
echo -e "${GREEN}╔══════════════════════════════════════╗${NC}"
echo -e "${GREEN}║          构建完成!                   ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════╝${NC}"
echo ""
echo "输出文件:"
echo "  .app: $APP_BUNDLE"
echo "  DMG:  $DMG_PATH"
echo ""
echo "文件大小:"
du -h "$DMG_PATH" 2>/dev/null || echo "  (未知)"
echo ""

# 可选: 打开 dist 目录
read -p "是否打开输出目录? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    open "$DIST_DIR"
fi
