#!/bin/bash

# Word Collector 重启脚本
# 检查依赖、重新配置并重启 Hammerspoon

set -e

echo "📖 Word Collector - 重启脚本"
echo "================================"

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ========== 1. 检查 Hammerspoon ==========
echo ""
echo "📦 检查 Hammerspoon..."

if [ -d "/Applications/Hammerspoon.app" ]; then
    echo -e "${GREEN}✓ Hammerspoon 已安装${NC}"
else
    echo -e "${YELLOW}⚠️  Hammerspoon 未安装，正在安装...${NC}"

    if command -v brew &> /dev/null; then
        brew install --cask hammerspoon
        echo -e "${GREEN}✓ Hammerspoon 安装完成${NC}"
    else
        echo -e "${RED}❌ 未检测到 Homebrew${NC}"
        echo "请手动安装 Hammerspoon: https://www.hammerspoon.org/"
        exit 1
    fi
fi

# ========== 2. 检查 word-collector 可执行文件 ==========
echo ""
echo "📦 检查 word-collector..."

COLLECTOR_DIR="$HOME/word-collector"
COLLECTOR_BIN="$COLLECTOR_DIR/word-collector"

# 如果项目目录下有 word-collector，复制到 ~/word-collector/
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_BIN="$PROJECT_DIR/word-collector"

if [ -f "$PROJECT_BIN" ]; then
    echo "发现项目目录下的 word-collector，正在复制..."
    mkdir -p "$COLLECTOR_DIR"
    cp "$PROJECT_BIN" "$COLLECTOR_BIN"
    echo -e "${GREEN}✓ 已复制 word-collector 到 $COLLECTOR_DIR${NC}"
fi

if [ ! -f "$COLLECTOR_BIN" ]; then
    echo -e "${RED}❌ word-collector 不存在${NC}"
    echo "请先运行: cd $PROJECT_DIR && go build -o word-collector ./cmd/cli/"
    exit 1
fi

echo -e "${GREEN}✓ word-collector 存在: $COLLECTOR_BIN${NC}"

# ========== 3. 配置 Hammerspoon ==========
echo ""
echo "📦 配置 Hammerspoon..."

HAMMERSPOON_DIR="$HOME/.hammerspoon"
mkdir -p "$HAMMERSPOON_DIR"

# 复制 word_collector.lua
if [ -f "$PROJECT_DIR/hammerspoon/word_collector.lua" ]; then
    cp "$PROJECT_DIR/hammerspoon/word_collector.lua" "$HAMMERSPOON_DIR/"
    echo -e "${GREEN}✓ 已复制 word_collector.lua${NC}"
else
    echo -e "${RED}❌ 找不到 hammerspoon/word_collector.lua${NC}"
    exit 1
fi

# 更新 init.lua
INIT_FILE="$HAMMERSPOON_DIR/init.lua"

# 备份现有配置
if [ -f "$INIT_FILE" ]; then
    # 检查是否已包含 word_collector
    if grep -q "word_collector" "$INIT_FILE"; then
        echo -e "${GREEN}✓ init.lua 已包含 Word Collector${NC}"
    else
        # 备份并追加
        cp "$INIT_FILE" "$HAMMERSPOON_DIR/init_backup_$(date +%Y%m%d_%H%M%S).lua"
        echo "" >> "$INIT_FILE"
        echo "-- Word Collector" >> "$INIT_FILE"
        echo 'require("word_collector")' >> "$INIT_FILE"
        echo -e "${GREEN}✓ 已更新 init.lua${NC}"
    fi
else
    # 创建新的 init.lua
    echo "-- Word Collector" > "$INIT_FILE"
    echo 'require("word_collector")' >> "$INIT_FILE"
    echo -e "${GREEN}✓ 已创建 init.lua${NC}"
fi

# ========== 4. 检查辅助功能权限 ==========
echo ""
echo "📦 检查辅助功能权限..."

# 尝试通过 AppleScript 检查（不完美，但能给出提示）
echo -e "${YELLOW}重要：Hammerspoon 需要辅助功能权限才能监听快捷键${NC}"
echo ""
echo "请确认："
echo "  系统设置 → 隐私与安全性 → 辅助功能"
echo "  确保 Hammerspoon 已勾选 ✅"
echo ""
read -p "已完成授权？按 Enter 继续..."

# ========== 5. 重启 Hammerspoon ==========
echo ""
echo "📦 重启 Hammerspoon..."

# 先退出
pkill -x Hammerspoon 2>/dev/null || true
sleep 1

# 启动
open -a Hammerspoon
sleep 2

# 确认启动
if pgrep -x Hammerspoon > /dev/null; then
    echo -e "${GREEN}✓ Hammerspoon 已重启${NC}"
else
    echo -e "${RED}❌ Hammerspoon 启动失败${NC}"
    exit 1
fi

# ========== 6. 完成 ==========
echo ""
echo "================================"
echo -e "${GREEN}🎉 重启完成！${NC}"
echo ""
echo "使用方法："
echo "  1. 选中任意英文单词"
echo "  2. 按 Ctrl+Cmd+W (⌃⌘W)"
echo "  3. 看到 ✅ 提示即表示已添加"
echo ""
echo "调试："
echo "  - 点击菜单栏 📖 → 🧪 测试快捷键系统"
echo "  - 查看日志: open -a Console (搜索 WordCollector)"
echo "================================"
