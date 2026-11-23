from pathlib import Path
import re

css = (Path(__file__).resolve().parents[1] / 'internal/uiassets/ui.css').read_text(encoding='utf-8')
tokens = dict(re.findall(r'--([a-z]+):#([0-9a-fA-F]{6})', css))

def luminance(hex_color):
    channels = [int(hex_color[i:i+2], 16) / 255 for i in (0, 2, 4)]
    linear = [v / 12.92 if v <= .04045 else ((v + .055) / 1.055) ** 2.4 for v in channels]
    return sum(v * w for v, w in zip(linear, (.2126, .7152, .0722)))

for label, foreground, background in [('primary', 'ffffff', tokens['blue']), ('body', tokens['text'], tokens['bg']), ('secondary', tokens['muted'], tokens['panel']), ('error', tokens['red'], tokens['panel'])]:
    light, dark = sorted((luminance(foreground), luminance(background)), reverse=True)
    ratio = (light + .05) / (dark + .05)
    assert ratio >= 4.5, f'{label} contrast {ratio:.2f} < 4.5'
    print(f'{label}: {ratio:.2f}:1 PASS')
