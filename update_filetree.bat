@echo off
chcp 65001 >nul
echo 正在更新 Filetree.json...
python -c "
import json, os, sys

root = 'Content'
tree = {'Content': {}}
if not os.path.isdir(root):
    print('请在 gfriends 仓库根目录运行')
    sys.exit(1)

for entry in sorted(os.listdir(root)):
    subdir = os.path.join(root, entry)
    if not os.path.isdir(subdir):
        continue
    files = {}
    for f in sorted(os.listdir(subdir)):
        if f.lower().endswith(('.jpg', '.jpeg', '.png')):
            name, _ = os.path.splitext(f)
            files[name] = f
    if files:
        tree['Content'][entry] = dict(sorted(files.items()))

with open('Filetree.json', 'w', encoding='utf-8') as f:
    json.dump(tree, f, ensure_ascii=False, indent=2)

total = sum(len(v) for v in tree['Content'].values())
print(f'完成，共 {total} 张图片')
"
pause