#!/bin/bash
# 在你的 gfriends fork 仓库根目录执行，更新 Filetree.json
# 用法: bash update_filetree.sh

ROOT="Content"
FILE="Filetree.json"

if [ ! -d "$ROOT" ]; then
    echo "请在含有 Content 目录的 gfriends fork 仓库根目录执行"
    exit 1
fi

# 使用 Python 生成 JSON，避免 shell 处理 JSON 的坑
python3 << EOF
import json, os, sys, time

tree = {"Content": {}}
content_dir = os.path.join(os.getcwd(), "$ROOT")

if not os.path.isdir(content_dir):
    print(f"目录 {content_dir} 不存在")
    sys.exit(1)

for entry in os.listdir(content_dir):
    subdir = os.path.join(content_dir, entry)
    if not os.path.isdir(subdir):
        continue
    now = int(time.time())
    files = {}
    for f in sorted(os.listdir(subdir)):
        if f.lower().endswith(('.jpg', '.jpeg', '.png')):
            name, _ = os.path.splitext(f)
            files[name] = f"{f}?t={now}"
    if files:
        tree["Content"][entry] = dict(sorted(files.items()))

with open("$FILE", "w", encoding="utf-8") as f:
    json.dump(tree, f, ensure_ascii=False, indent=2)

print(f"Filetree.json 已更新，共 {sum(len(v) for v in tree['Content'].values())} 张图片")
EOF