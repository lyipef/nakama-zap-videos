"""Templates de fundo do @zaptaku (1080x1920).
Gera, para cada estilo (hud, forja):
  template_<estilo>_fundo.png    fundo completo (janela do clipe escura)
  template_<estilo>_moldura.png  camada transparente: vai POR CIMA do clipe (janela vazada)
  template_<estilo>_loop.mp4     fundo animado de 6s em loop perfeito
e um template_guia.png com as zonas marcadas.
Uso: python3 template.py <pasta_saida>
"""
import math, random, subprocess, sys, os, functools
from PIL import Image, ImageDraw, ImageFont, ImageFilter, ImageChops

OUT = sys.argv[1] if len(sys.argv) > 1 else "."
os.makedirs(OUT, exist_ok=True)
W, H, FPS, LOOP = 1080, 1920, 30, 6.0
F = "/home/claude/reel/fonts/"
@functools.lru_cache(maxsize=None)
def dela(s): return ImageFont.truetype(F + "Dela.ttf", int(s))
@functools.lru_cache(maxsize=None)
def bang(s): return ImageFont.truetype(F + "Bangers.ttf", int(s))

INK = (17, 17, 17); CREAM = (255, 246, 229); YEL = (255, 210, 63); ORA = (255, 122, 26)
BLUE = (127, 214, 255); RED = (230, 57, 70); PINK = (255, 143, 177)

# zonas (iguais para os dois estilos)
WIN = (0, 656, 1080, 1264)          # clipe 16:9 em largura total
TITLE = (60, 150, 1020, 600)        # título / palavra de hype
SUB = (60, 1300, 1020, 1470)        # legenda / CTA
FOOT_Y = 1500                       # créditos + @ (acima da legenda do TikTok)
SAFE_R = 900                        # à direita disso ficam os botões do TikTok

def lerp(a, b, t): return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))

def logo(d, cx, cy, s):
    d.ellipse((cx - 190*s, cy - 120*s, cx + 10*s, cy + 40*s), fill=CREAM, outline=INK, width=max(3, int(10*s)))
    d.ellipse((cx - 30*s, cy - 80*s, cx + 190*s, cy + 90*s), fill=YEL, outline=INK, width=max(3, int(10*s)))
    pts = [(122, 12), (24, 238), (98, 238), (64, 428), (186, 168), (108, 168), (152, 12)]
    k = 0.6 * s
    d.polygon([(cx - 55*s + px*k, cy - 165*s + py*k) for px, py in pts], fill=ORA, outline=INK, width=max(3, int(8*s)))

def pill(d, x, y, text, font, fill, tcol=INK, padx=26, pady=8):
    l, t, r, b = font.getbbox(text); w = r - l + 2 * padx; h = b - t + 2 * pady
    d.rounded_rectangle((x + 5, y + 5, x + w + 5, y + h + 5), h / 2, fill=INK)
    d.rounded_rectangle((x, y, x + w, y + h), h / 2, fill=fill, outline=INK, width=5)
    d.text((x + padx - l, y + pady - t), text, font=font, fill=tcol)
    return w, h

def brackets(d, box, col, L=70, w=10, off=18):
    x0, y0, x1, y1 = box[0] + off, box[1] - off, box[2] - off, box[3] + off
    for (x, y, sx, sy) in [(x0, y0, 1, 1), (x1, y0, -1, 1), (x0, y1, 1, -1), (x1, y1, -1, -1)]:
        d.line((x, y, x + sx * L, y), fill=col, width=w)
        d.line((x, y, x, y + sy * L), fill=col, width=w)

# ---------------- fundos animados ----------------
def bg_hud(t):
    im = Image.new("RGB", (W, H), (12, 12, 34)); d = ImageDraw.Draw(im)
    for k in range(0, H, 6): d.line((0, k, W, k), fill=(17, 17, 44))
    off = (t / LOOP) * 120
    for k in range(-120, W + 120, 120): d.line((k + off, 0, k + off, H), fill=(22, 24, 58), width=2)
    for k in range(-120, H + 120, 120): d.line((0, k + off, W, k + off), fill=(22, 24, 58), width=2)
    sy = int((t / LOOP) * H * 2) % H
    d.rectangle((0, sy, W, sy + 6), fill=(34, 46, 92))
    g = Image.new("RGB", (W // 4, H // 4)); gd = ImageDraw.Draw(g)
    p = 0.5 + 0.5 * math.sin(2 * math.pi * t / LOOP)
    gd.ellipse((-80, 120, W // 4 + 80, 360), fill=lerp((0, 0, 0), (20, 60, 110), 0.6 + 0.4 * p))
    im = ImageChops.add(im, g.filter(ImageFilter.GaussianBlur(30)).resize((W, H)))
    d = ImageDraw.Draw(im)
    rnd = random.Random(4)
    for i in range(26):  # "dados" subindo
        x = rnd.uniform(0, W); y0 = rnd.uniform(0, H); sp = rnd.choice([1, 2])
        y = (y0 - (t / LOOP) * H * sp) % H
        c = rnd.choice([BLUE, (90, 120, 200), YEL])
        d.text((x, y), rnd.choice(["0", "1", "+", "◆"]) if False else rnd.choice(["0", "1", "+"]), font=bang(28), fill=lerp((12, 12, 34), c, 0.55))
    return im

def bg_forja(t):
    im = Image.new("RGB", (W, H), (26, 14, 40)); d = ImageDraw.Draw(im)
    heat = 0.75 + 0.25 * math.sin(2 * math.pi * t / LOOP)
    g = Image.new("RGB", (W // 4, H // 4)); gd = ImageDraw.Draw(g)
    gd.ellipse((-90, 260, W // 4 + 90, H // 4 + 160), fill=lerp((0, 0, 0), (200, 75, 10), 0.55 * heat))
    gd.ellipse((-60, -120, W // 4 + 60, 90), fill=lerp((0, 0, 0), (90, 30, 60), 0.6))
    im = ImageChops.add(im, g.filter(ImageFilter.GaussianBlur(45)).resize((W, H)))
    d = ImageDraw.Draw(im)
    rnd = random.Random(9)
    for i in range(70):  # brasas subindo, loop perfeito
        x0 = rnd.uniform(0, W); y0 = rnd.uniform(0, H); sp = rnd.choice([1, 1, 2]); r = rnd.uniform(2.5, 7)
        ph = rnd.uniform(0, 2 * math.pi)
        y = (y0 - (t / LOOP) * H * sp) % H
        x = x0 + math.sin(2 * math.pi * t / LOOP * 2 + ph) * 18
        fade = min(1, y / 500)
        c = lerp((26, 14, 40), (255, 160 + int(r * 10), 60), fade)
        d.ellipse((x - r, y - r, x + r, y + r), fill=c)
    return im

# ---------------- moldura (fica por cima do clipe) ----------------
def frame_layer(style):
    lay = Image.new("RGBA", (W, H), (0, 0, 0, 0)); d = ImageDraw.Draw(lay)
    acc = BLUE if style == "hud" else ORA
    acc2 = YEL
    x0, y0, x1, y1 = WIN
    # bordas da janela
    d.rectangle((0, y0 - 10, W, y0), fill=acc2 + (255,)); d.rectangle((0, y1, W, y1 + 10), fill=acc2 + (255,))
    d.rectangle((0, y0 - 16, W, y0 - 10), fill=INK + (255,)); d.rectangle((0, y1 + 10, W, y1 + 16), fill=INK + (255,))
    brackets(d, (40, y0 + 40, W - 40, y1 - 40), acc + (255,), 64, 8, 0)
    # rótulo da janela
    if style == "hud":
        lab = "[ CENA ]"; f = bang(44)
        w = f.getlength(lab) + 40
        d.rectangle((60, y0 - 70, 60 + w, y0 - 16), fill=acc + (255,), outline=INK + (255,), width=5)
        d.text((80, y0 - 66), lab, font=f, fill=INK + (255,))
        for q in range(3): d.rectangle((W - 230 + q * 44, y0 - 56, W - 202 + q * 44, y0 - 28), fill=acc + (255,), outline=INK + (255,), width=3)
    else:
        lab = "FORJADO NO ZAPTAKU"; f = bang(44)
        w = f.getlength(lab) + 52
        d.rounded_rectangle((60, y0 - 74, 60 + w, y0 - 16), 29, fill=ORA + (255,), outline=INK + (255,), width=5)
        d.text((86, y0 - 70), lab, font=f, fill=INK + (255,))
    # topo: logo + @
    logo(d, 132, 92, 0.42)
    pill(d, 230, 60, "@zaptaku", dela(40), CREAM)
    # rodapé: espaço de créditos + @
    d.line((60, FOOT_Y - 14, SAFE_R - 60, FOOT_Y - 14), fill=(255, 255, 255, 60), width=2)
    d.text((60, FOOT_Y + 64), "@zaptaku", font=dela(44), fill=CREAM + (255,))
    return lay

def window_mask():
    m = Image.new("L", (W, H), 255); ImageDraw.Draw(m).rectangle(WIN, fill=0); return m

def compose(style, t, with_window_fill=True):
    bg = (bg_hud if style == "hud" else bg_forja)(t)
    if with_window_fill:
        ImageDraw.Draw(bg).rectangle(WIN, fill=(6, 6, 16))
        f = bang(54); txt = "SEU CLIPE AQUI (16:9)"
        ImageDraw.Draw(bg).text((W / 2 - f.getlength(txt) / 2, (WIN[1] + WIN[3]) / 2 - 30), txt, font=f, fill=(60, 60, 90))
    lay = frame_layer(style)
    bg.paste(lay, (0, 0), lay)
    return bg

for style in ("hud", "forja"):
    compose(style, 0).save(f"{OUT}/template_{style}_fundo.png")
    fr = frame_layer(style)
    # moldura com o fundo ao redor e a janela vazada
    full = (bg_hud if style == "hud" else bg_forja)(0).convert("RGBA")
    full.paste(fr, (0, 0), fr)
    full.putalpha(window_mask())
    full.save(f"{OUT}/template_{style}_moldura.png")
    # loop animado (janela vazia escura, pronto para sobrepor o clipe)
    p = subprocess.Popen(["ffmpeg", "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgb24", "-s", f"{W}x{H}", "-r", str(FPS), "-i", "-",
                          "-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "veryfast", "-crf", "20", "-movflags", "+faststart",
                          f"{OUT}/template_{style}_loop.mp4"], stdin=subprocess.PIPE)
    for i in range(int(LOOP * FPS)):
        im = compose(style, i / FPS, with_window_fill=False)
        ImageDraw.Draw(im).rectangle(WIN, fill=(6, 6, 16))
        p.stdin.write(im.tobytes())
    p.stdin.close(); p.wait()

# guia de zonas
g = compose("hud", 0).convert("RGBA"); ov = Image.new("RGBA", (W, H), (0, 0, 0, 0)); d = ImageDraw.Draw(ov)
def zone(box, col, label):
    d.rectangle(box, fill=col + (60,), outline=col + (255,), width=4)
    d.text((box[0] + 14, box[1] + 10), label, font=bang(40), fill=col + (255,))
zone(TITLE, YEL, f"TÍTULO / PALAVRA DE HYPE  (y {TITLE[1]}–{TITLE[3]})")
zone(WIN, BLUE, f"CLIPE 16:9 — 1080x608  (y {WIN[1]}–{WIN[3]})")
zone(SUB, PINK, f"LEGENDA / CTA  (y {SUB[1]}–{SUB[3]})")
zone((60, FOOT_Y, SAFE_R - 60, FOOT_Y + 130), GREEN if (GREEN := (182, 240, 106)) else GREEN, "CRÉDITOS + @")
zone((SAFE_R, 700, W, 1650), RED, "BOTÕES TIKTOK")
zone((0, 1650, W, H), RED, "LEGENDA DO TIKTOK/YOUTUBE — NÃO USAR")
g = Image.alpha_composite(g, ov).convert("RGB"); g.save(f"{OUT}/template_guia.png")
print("ok")
