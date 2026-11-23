from pathlib import Path
import re

root = Path(__file__).resolve().parents[1]
assets = root / 'internal/uiassets'
css = (assets / 'ui.css').read_text(encoding='utf-8')
pages = []
for name in ('invite', 'control', 'merchant'):
    html = (assets / (name + '.html')).read_text(encoding='utf-8')
    body = re.search(r'<body>(.*)</body>', html, re.S).group(1)
    body = re.sub(r'<script.*?</script>', '', body, flags=re.S)
    body = body.replace('<button ', '<button disabled ')
    body = body.replace('正在读取校验码…', 'PEBBLE · MAPLE · ORBIT（示例）')
    body = body.replace('正在读取状态', '设计预览')
    pages.append(body)
out = '<!doctype html><meta charset="utf-8"><title>PairShell UI 设计预览</title><style>' + css + '</style>'
out += '<p class="frame">设计预览 · 两个页面按顺序展示。不会建立连接，按钮已禁用。实际界面来自同一份 HTML/CSS。</p>'
out += '<hr>'.join(pages)
(root / 'docs/UI-PREVIEW.html').write_text(out, encoding='utf-8')
