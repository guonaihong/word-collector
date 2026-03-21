-- Word Collector - ⌃⌘W 取词
local config = {
    hotkeyMod = {"ctrl", "cmd"},
    hotkeyKey = "w",
    collectorPath = os.getenv("HOME") .. "/word-collector/word-collector",
}

local function collect()
    hs.eventtap.keyStroke({"cmd"}, "c")
    hs.timer.usleep(200000)
    local text = hs.pasteboard.getContents()
    if not text or text == "" then return end
    text = text:gsub("^%s+", ""):gsub("%s+$", "")

    local n = 0
    for _ in text:gmatch("%S+") do n = n + 1 end
    if n > 3 then return end

    hs.task.new(config.collectorPath, function(code)
        if code == 0 then hs.alert.show("✅ " .. text, 1.5) end
    end, nil, {text}):start()
end

hs.hotkey.bind(config.hotkeyMod, config.hotkeyKey, collect)

hs.menubar.new():setTitle("📖"):setMenu({
    {title = "Anki", fn = function() hs.application.launchOrFocus("Anki") end},
    {title = "Reload", fn = function() hs.reload() end},
})

hs.alert.show("📖 ⌃⌘W", 2)
