"""Export the original vector geometry into application icon formats.

Pillow is authoring-only; no Python or Pillow dependency ships with PairShell.
"""
from pathlib import Path
import json
from PIL import Image, ImageDraw

ROOT = Path(__file__).resolve().parent
SCALE = 4
SIZE = 1024


def stroke(draw, points, color, width):
    points = [(x * SCALE, y * SCALE) for x, y in points]
    width *= SCALE
    draw.line(points, fill=color, width=width, joint="curve")
    radius = width / 2
    for x, y in points:
        draw.ellipse((x-radius, y-radius, x+radius, y+radius), fill=color)


image = Image.new("RGBA", (SIZE * SCALE, SIZE * SCALE))
draw = ImageDraw.Draw(image)
draw.rounded_rectangle((0, 0, SIZE*SCALE-1, SIZE*SCALE-1), radius=228*SCALE, fill="#151E2E")
stroke(draw, [(414,262),(310,262),(262,310),(262,714),(310,762),(414,762)], "#628BFF", 80)
stroke(draw, [(610,262),(714,262),(762,310),(762,714),(714,762),(610,762)], "#47D8C8", 80)
stroke(draw, [(402,422),(492,512),(402,602)], "#F0F6FF", 64)
stroke(draw, [(552,602),(626,602)], "#F0F6FF", 64)

sizes = (16, 20, 24, 32, 40, 48, 64, 128, 256, 512, 1024)
for size in sizes:
    image.resize((size, size), Image.Resampling.LANCZOS).save(ROOT / f"pairshell-{size}.png")
image.resize((256, 256), Image.Resampling.LANCZOS).save(
    ROOT / "pairshell.ico", sizes=[(s,s) for s in (16,24,32,48,64,128,256)]
)

iconset = ROOT / "PairShell.iconset"
iconset.mkdir(exist_ok=True)
for size in (16, 32, 128, 256, 512):
    for factor in (1, 2):
        suffix = "@2x" if factor == 2 else ""
        image.resize((size*factor, size*factor), Image.Resampling.LANCZOS).save(iconset / f"icon_{size}x{size}{suffix}.png")

preview = Image.new("RGB", (480, 180), "#E8EDF5")
for x, size in ((30,16),(82,32),(160,64),(290,128)):
    thumb = image.resize((size,size),Image.Resampling.LANCZOS)
    preview.paste(thumb,(x,(180-size)//2),thumb)
preview.save(ROOT / "size-preview.png")
(ROOT / "manifest.json").write_text(json.dumps({"name":"PairShell paired terminal","version":1,"colors":{"background":"#151E2E","pair_a":"#628BFF","pair_b":"#47D8C8","prompt":"#F0F6FF"},"sizes":sizes}, indent=2)+"\n", encoding="utf-8")
