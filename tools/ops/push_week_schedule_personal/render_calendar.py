# -*- coding: utf-8 -*-
"""Render month schedule PNG from real week-schedule JSON (DB-driven)."""

import json
import sys
from pathlib import Path

try:
    from PIL import Image, ImageDraw, ImageFont
except ImportError:
    import subprocess

    subprocess.check_call([sys.executable, "-m", "pip", "install", "pillow", "-q"])
    from PIL import Image, ImageDraw, ImageFont


def load_font(size, bold=False):
    candidates = [
        r"C:\Windows\Fonts\msyhbd.ttc" if bold else r"C:\Windows\Fonts\msyh.ttc",
        r"C:\Windows\Fonts\msyh.ttc",
        r"C:\Windows\Fonts\simhei.ttf",
        r"C:\Windows\Fonts\simsun.ttc",
    ]
    for c in candidates:
        try:
            return ImageFont.truetype(c, size)
        except Exception:
            pass
    return ImageFont.load_default()


def main():
    json_path = Path(sys.argv[1])
    png_path = Path(sys.argv[2])
    data = json.loads(json_path.read_text(encoding="utf-8"))
    weeks = data["weeks"]
    title = data["title"]

    colors = {
        "outside": ("#f0f0f0", "#bfbfbf"),
        "work": ("#fff566", "#1f1f1f"),
        "rest": ("#ffffff", "#1f1f1f"),
        "holiday": ("#ff4d4f", "#ffffff"),
    }
    size_style = {
        "small": ("#fff7e6", "#fa8c16", "单休"),
        "big": ("#e6f4ff", "#1677ff", "双休"),
    }

    col_w, row_h, header_h, title_h, pad = 100, 78, 48, 72, 28
    cols = 9
    rows = len(weeks)
    scale = 2
    width = pad * 2 + col_w * cols
    height = pad * 2 + title_h + header_h + row_h * rows + 12

    img = Image.new("RGB", (width * scale, height * scale), "#ffffff")
    draw = ImageDraw.Draw(img)

    def rect(x, y, w, h):
        return [x * scale, y * scale, (x + w) * scale, (y + h) * scale]

    def text_center(cx, cy, text, font, fill):
        bb = draw.textbbox((0, 0), text, font=font)
        tw, th = bb[2] - bb[0], bb[3] - bb[1]
        draw.text((cx * scale - tw / 2, cy * scale - th / 2), text, fill=fill, font=font)

    f_title = load_font(30 * scale, True)
    f_head = load_font(16 * scale, True)
    f_day = load_font(22 * scale, True)
    f_week = load_font(16 * scale, True)
    f_small = load_font(12 * scale, False)
    f_size = load_font(20 * scale, True)

    text_center(width / 2, pad + title_h / 2, title, f_title, "#1f1f1f")

    headers = ["周数", "周一", "周二", "周三", "周四", "周五", "周六", "周日", "大小周"]
    top, left = pad + title_h, pad
    for i, header in enumerate(headers):
        x = left + i * col_w
        draw.rectangle(rect(x, top, col_w, header_h), fill="#fafafa", outline="#d9d9d9")
        text_center(x + col_w / 2, top + header_h / 2, header, f_head, "#1f1f1f")

    for ri, week in enumerate(weeks):
        y = top + header_h + ri * row_h
        draw.rectangle(rect(left, y, col_w, row_h), fill="#ffffff", outline="#d9d9d9")
        text_center(left + col_w / 2, y + row_h / 2 - 10, f"第{ri + 1}周", f_week, "#1f1f1f")
        ws, we = week["week_start"], week["week_end"]
        range_label = f"{ws[5:7]}/{ws[8:10]}-{we[5:7]}/{we[8:10]}"
        text_center(left + col_w / 2, y + row_h / 2 + 14, range_label, f_small, "#8c8c8c")

        for di, cell in enumerate(week["cells"]):
            x = left + (di + 1) * col_w
            bg, fg = colors.get(cell["state"], colors["work"])
            draw.rectangle(rect(x, y, col_w, row_h), fill=bg, outline="#d9d9d9")
            text_center(x + col_w / 2, y + row_h / 2, str(cell["day"]), f_day, fg)

        # Size label from REAL week_type (big/small)
        wt = week.get("week_type") or "big"
        if wt not in size_style:
            wt = "big"
        sbg, sfg, slabel = size_style[wt]
        sx = left + 8 * col_w
        draw.rectangle(rect(sx, y, col_w, row_h), fill=sbg, outline="#d9d9d9")
        text_center(sx + col_w / 2, y + row_h / 2, slabel, f_size, sfg)

    png_path.parent.mkdir(parents=True, exist_ok=True)
    img.save(png_path, "PNG")
    print(f"rendered {png_path} size={png_path.stat().st_size}")


if __name__ == "__main__":
    main()
