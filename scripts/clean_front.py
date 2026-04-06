#!/usr/bin/env python3
"""清理 Anki 卡片正面的脏数据，只保留纯单词。"""

import json, re, urllib.request

def anki_request(action, params=None):
    body = json.dumps({"action": action, "version": 6, "params": params or {}}).encode()
    req = urllib.request.Request("http://localhost:8765", data=body,
                                 headers={"Content-Type": "application/json"})
    resp = urllib.request.urlopen(req, timeout=30)
    data = json.loads(resp.read())
    if data.get("error"):
        raise Exception(f"AnkiConnect error: {data['error']}")
    return data["result"]

# 1. Get all note IDs
note_ids = anki_request("findNotes", {"query": ""})
print(f"总笔记数: {len(note_ids)}")

# 2. Get all notes info
notes = anki_request("notesInfo", {"notes": note_ids})

# 3. Clean front field
fixed = 0
for n in notes:
    front = n["fields"].get("正面", "")
    # Strip HTML
    word = re.sub(r"<[^>]+>", "", front).strip()
    # Take only the part before any phonetic /xxx/
    word = re.split(r"/", word)[0].strip()
    # Remove trailing whitespace or punctuation artifacts
    word = word.strip()

    if not word:
        continue

    if front != word:
        anki_request("updateNoteFields", {
            "note": {"id": n["noteId"], "fields": {"正面": word}}
        })
        fixed += 1
        print(f"  ✅ [{fixed}] \"{front[:50]}\" -> \"{word}\"")

print(f"\n完成！共修复 {fixed} 个笔记。")
