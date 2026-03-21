#!/bin/bash

# Hammerspoon 配置安装脚本
# 实现双击选中单词自动添加到 Anki

set -e

echo "📖 Word Collector - Hammerspoon 双击触发安装"
echo "================================================"
echo ""

# 检查是否已安装 Hammerspoon
if [ -d "/Applications/Hammerspoon.app" ]; then
    echo "✓ Hammerspoon 已安装"
else
    echo "⚠️  Hammerspoon 未安装"
    echo ""
    echo "正在通过 Homebrew 安装 Hammerspoon..."

    if command -v brew &> /dev/null; then
        brew install --cask hammerspoon
    else
        echo "❌ 未检测到 Homebrew"
        echo ""
        echo "请手动安装 Hammerspoon："
        echo "1. 访问 https://www.hammerspoon.org/"
        echo "2. 下载并安装"
        echo "3. 重新运行此脚本"
        exit 1
    fi
fi

# 检查 word-collector 是否已安装
if [ ! -f "$HOME/word-collector/word-collector" ]; then
    echo ""
    echo "⚠️  未检测到 word-collector"
    echo "请先运行项目根目录的 ./install.sh"
    exit 1
fi

echo ""
echo "📁 配置 Hammerspoon..."

# 创建 Hammerspoon 配置目录
mkdir -p "$HOME/.hammerspoon"

# 备份现有配置
if [ -f "$HOME/.hammerspoon/init.lua" ]; then
    backup_name="init_backup_$(date +%Y%m%d_%H%M%S).lua"
    cp "$HOME/.hammerspoon/init.lua" "$HOME/.hammerspoon/$backup_name"
    echo "✓ 已备份现有配置到 $backup_name"
fi

# 复制 Word Collector 配置
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

cp "$SCRIPT_DIR/word_collector.lua" "$HOME/.hammerspoon/word_collector.lua"
echo "✓ 已复制 word_collector.lua"

# 创建/更新 init.lua
INIT_FILE="$HOME/.hammerspoon/init.lua"

# 检查是否已包含 word_collector
if [ -f "$INIT_FILE" ] && grep -q "word_collector" "$INIT_FILE"; then
    echo "✓ init.lua 已包含 Word Collector 配置"
else
    # 追加到 init.lua
    cat >> "$INIT_FILE" << 'EOF'

-- ============================================
-- Word Collector 双击触发配置
-- ============================================
require("word_collector")

EOF
    echo "✓ 已更新 init.lua"
fi

echo ""
echo "🚀 启动 Hammerspoon..."

# 启动 Hammerspoon
open -a Hammerspoon

# 等待启动
sleep 1

# 检查 Accessibility 权限
if pgrep -x "Hammerspoon" > /dev/null; then
    echo "✓ Hammerspoon 已启动"
else
    echo "⚠️  请手动启动 Hammerspoon"
fi

echo ""
echo "========================================"
echo "🎉 安装完成！"
echo ""
echo "使用方法："
echo "1. 选中任意英文单词"
echo "2. 按 Ctrl+Cmd+W (⌃⌘W)"
echo "3. 看到 ✅ 提示即表示已添加"
echo ""
echo "注意："
echo "- 第一次运行时，macOS 会请求辅助功能权限"
echo "- 请前往 系统设置 → 隐私与安全性 → 辅助功能"
echo "  勾选 Hammerspoon"
echo "- 菜单栏会出现 📖 图标，点击可手动触发"
echo "- 点击菜单栏 📖 → Reload，可以重新加载配置"
echo ""
echo "调试："
echo "- 打开 Console.app: open -a Console"
echo "- 在 Console.app 中搜索 'WordCollector' 查看详细日志"
echo ""
echo "卸载方法："
echo "删除 ~/.hammerspoon/word_collector.lua"
echo "并从 ~/.hammerspoon/init.lua 中移除相关行"
echo "========================================"
