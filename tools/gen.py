import math, subprocess, functools, sys, json
from PIL import Image, ImageDraw, ImageFont

CFG = json.load(open(sys.argv[1]))
W, H, FPS = 1080, 1920, 30
F = "/home/claude/reel/fonts/"
DEJA = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"

@functools.lru_cache(maxsize=None)
def dela(s): return ImageFont.truetype(F + "Dela.ttf", int(s))
@functools.lru_cache(maxsize=None)
def bang(s): return ImageFont.truetype(F + "Bangers.ttf", int(s))
@functools.lru_cache(maxsize=None)
def deja(s): return ImageFont.truetype(DEJA, int(s))

BASE = (42, 27, 92); INK = (17, 17, 17); CREAM = (255, 246, 229)
YEL = (255, 210, 63); ORA = (255, 122, 26); RED = (230, 57, 70); BLUE = (127, 214, 255)
PINK = (255, 143, 177); GREEN = (182, 240, 106); SUB = (74, 63, 102); DIM = (66, 50, 120)
COL = {"YEL": YEL, "ORA": ORA, "RED": RED, "BLUE": BLUE, "PINK": PINK, "GREEN": GREEN, "CREAM": CREAM}

BPM = 76; BEAT = 60 / BPM; BAR = BEAT * 4
CX = 500  # folga para os botões laterais do TikTok

def clamp(x): return max(0.0, min(1.0, x))
def ease(t): t = clamp(t); return 1 - (1 - t) ** 3
def back(t):
    t = clamp(t); c = 1.70158
    return 1 + (c + 1) * (t - 1) ** 3 + c * (t - 1) ** 2
def lerp(a, b, t): return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))

def fit(text, fontf, size, maxw):
    while size > 20:
        if fontf(size).getlength(text) <= maxw: return fontf(size)
        size -= 2
    return fontf(size)

def wrap(text, font, maxw):
    lines, cur = [], ""
    for w in text.split():
        t = (cur + " " + w).strip()
        if font.getlength(t) <= maxw: cur = t
        else: lines.append(cur); cur = w
    if cur: lines.append(cur)
    return lines

def fit_lines(text, fontf, hi, lo, maxw, maxlines):
    for size in range(hi, lo, -2):
        l = wrap(text, fontf(size), maxw)
        if len(l) <= maxlines and all(fontf(size).getlength(x) <= maxw for x in l): return fontf(size), l
    return fontf(lo), wrap(text, fontf(lo), maxw)

def ctext(d, y, text, font, fill, shadow=None, off=8, cx=CX):
    x = cx - font.getlength(text) / 2
    if shadow: d.text((x + off, y + off), text, font=font, fill=shadow)
    d.text((x, y), text, font=font, fill=fill)

def pop_text(d, y, text, fontf, size, fill, shadow, t, start, maxw=900, off=9):
    s = back((t - start) / 0.35)
    if s <= 0.05: return
    full = fit(text, fontf, size, maxw)
    sz = max(10, int(full.size * s))
    ctext(d, y + (full.size - sz) * 0.5, text, fontf(sz), fill, shadow, off)

def pill(d, x, y, text, font, fill, tcol=INK, padx=34, pady=10, center=True):
    l, t, r, b = font.getbbox(text)
    w = (r - l) + 2 * padx; h = (b - t) + 2 * pady
    x0 = x - w / 2 if center else x
    d.rounded_rectangle((x0 + 6, y + 6, x0 + w + 6, y + h + 6), h / 2, fill=INK)
    d.rounded_rectangle((x0, y, x0 + w, y + h), h / 2, fill=fill, outline=INK, width=6)
    d.text((x0 + padx - l, y + pady - t), text, font=font, fill=tcol)
    return w, h

def bolt(d, cx, y, s, fill, outline=INK, width=4):
    pts = [(122, 12), (24, 238), (98, 238), (64, 428), (186, 168), (108, 168), (152, 12)]
    d.polygon([(cx + (px - 105) * s, y + (py - 12) * s) for px, py in pts], fill=fill, outline=outline, width=width)

def logo(d, cx, cy, s):
    d.ellipse((cx - 190*s, cy - 120*s, cx + 10*s, cy + 40*s), fill=CREAM, outline=INK, width=max(3, int(10*s)))
    d.ellipse((cx - 30*s, cy - 80*s, cx + 190*s, cy + 90*s), fill=YEL, outline=INK, width=max(3, int(10*s)))
    pts = [(122, 12), (24, 238), (98, 238), (64, 428), (186, 168), (108, 168), (152, 12)]
    k = 0.6 * s
    d.polygon([(cx - 55*s + px*k, cy - 165*s + py*k) for px, py in pts], fill=ORA, outline=INK, width=max(3, int(8*s)))

# ---------- fundo (padrão varia por vídeo; base #2A1B5C) ----------
PAT = lerp(BASE, (255, 255, 255), 0.07)
bgimg = Image.new("RGB", (W + 160, H + 160), BASE)
_bd = ImageDraw.Draw(bgimg)
style = CFG["bg"]
if style == "dots":
    for yy in range(0, H + 160, 40):
        for xx in range(0, W + 160, 40):
            ox = 20 if (yy // 40) % 2 else 0
            _bd.ellipse((xx + ox - 4, yy - 4, xx + ox + 4, yy + 4), fill=PAT)
elif style == "stripes":
    for k in range(-H, W + H, 80):
        _bd.polygon([(k, 0), (k + 26, 0), (k + 26 + H + 160, H + 160), (k + H + 160, H + 160)], fill=PAT)
else:  # grid
    for k in range(0, W + 160, 80): _bd.line((k, 0, k, H + 160), fill=PAT, width=3)
    for k in range(0, H + 160, 80): _bd.line((0, k, W + 160, k), fill=PAT, width=3)

def make_bg(T, col):
    o = int(T * 14) % 80
    im = bgimg.crop((o, o, o + W, o + H))
    d = ImageDraw.Draw(im)
    d.polygon([(0, 1500), (W, 1250), (W, 1430), (0, 1710)], fill=lerp(BASE, col, 0.25))
    return im, d

ITEMS = CFG["items"]
NI = len(ITEMS)
for it in ITEMS: it["col"] = COL[it["col"]]
KIND = CFG["kind"]
RCOL = COL[CFG.get("rating_color", "YEL")]

# ---------- gancho ----------
_hook = []
_y = 420
for txt, c, sz in CFG["hook_lines"]:
    f = fit(txt, dela, sz, 880)
    _hook.append((txt, COL[c], sz, _y)); _y += int(f.size * 1.2) + 4
HOOK_BOX_Y = max(1160, _y + 40)

def draw_hook(d, t, T):
    shake = math.sin(t * 70) * 5 if t < 0.8 else 0
    s = back((t - 0.05) / 0.35)
    if s > 0.05:
        pill(d, CX, 300, CFG["hook_pill"], bang(max(10, int(62 * s))), YEL)
    for i, (txt, c, sz, y) in enumerate(_hook):
        pop_text(d, y + (shake if i % 2 == 0 else -shake), txt, dela, sz, c, INK, t, 0.15 + i * 0.2, 880)
    b = ease((t - 1.4) / 0.35)
    if b > 0:
        y = HOOK_BOX_Y + (1 - b) * 90
        d.rounded_rectangle((100, y + 10, 900, y + 160), 30, fill=INK)
        d.rounded_rectangle((100, y, 900, y + 150), 30, fill=ORA, outline=INK, width=8)
        ctext(d, y + 34, CFG["hook_box"], fit(CFG["hook_box"], bang, 84, 740), INK)

# ---------- itens ----------
x0, y0, cw, ch = 80, 430, 840, (700 if CFG["kind"] == "flag" else 640)
DY = ch - 640

def header(d, t, T, idx, col):
    pulse = max(0, 1 - ((T % BEAT) / BEAT) * 3.5)
    cs = int(130 * (1 + 0.05 * pulse) * (0.6 + 0.4 * back(t / 0.35)))
    d.text((86, 200), f"{idx}/{NI}", font=dela(cs), fill=INK)
    d.text((80, 194), f"{idx}/{NI}", font=dela(cs), fill=col)
    tag = "@nakama.zap"
    d.text((920 - deja(34).getlength(tag), 240), tag, font=deja(34), fill=CREAM)
    return pulse

def card(d, t, col):
    ox = int((1 - ease(t / 0.45)) * W)
    d.rounded_rectangle((x0 + 16 + ox, y0 + 16, x0 + cw + 16 + ox, y0 + ch + 16), 38, fill=col)
    d.rounded_rectangle((x0 + ox, y0, x0 + cw + ox, y0 + ch), 38, fill=CREAM, outline=INK, width=8)
    return ox

def reveal_block(d, t, ox, label, lcol, text, bottom=True):
    """pill + texto que aparece em 1.3s, ancorado na base do card"""
    wf = deja(38); wl = wrap(text, wf, cw - 80)
    pf = bang(46); l_, t_, r_, b_ = pf.getbbox(label); ph = (b_ - t_) + 20
    base_y = y0 + ch - 36 - len(wl) * 50
    a = ease((t - 1.3) / 0.4)
    if a <= 0: return
    yy = base_y - 14 - ph + int((1 - a) * 25)
    pw = (r_ - l_) + 60
    d.rounded_rectangle((x0 + 40 + ox, yy, x0 + 40 + pw + ox, yy + ph), ph / 2, fill=lcol, outline=INK, width=5)
    d.text((x0 + 70 - l_ + ox, yy + 10 - t_), label, font=pf, fill=INK)
    tc = lerp(CREAM, INK, a)
    for k, ln in enumerate(wl):
        d.text((x0 + 40 + ox, base_y + k * 50 + int((1 - a) * 25)), ln, font=wf, fill=tc)

def bolts(d, t, label, rating, col):
    if t <= 0.9: return
    ctext(d, 1108 + DY, label, bang(58), CREAM, INK, off=4)
    for i in range(5):
        bx = CX + (i - 2) * 80; ti = 2.0 + i * 0.28
        if i < rating and t >= ti:
            bolt(d, bx, 1190 + DY, 0.27 * (0.6 + 0.4 * back((t - ti) / 0.3)), col)
        else:
            bolt(d, bx, 1190 + DY, 0.27, DIM, outline=(90, 75, 140), width=3)

def footer_pill(d, idx, pulse):
    if idx == NI:
        pill(d, CX, 1350 + DY, CFG["last_pill"], bang(54), ORA)
    else:
        pill(d, CX, 1350 + DY, CFG["promise_pill"], bang(int(54 * (1 + 0.03 * pulse))), YEL)

def draw_list(d, t, T, idx, it):
    col = it["col"]; pulse = header(d, t, T, idx, col); ox = card(d, t, col)
    tf, tl = fit_lines(it["title"], dela, 84, 44, cw - 80, 2)
    ty = y0 + 36
    for ln in tl:
        d.text((x0 + 40 + ox, ty), ln, font=tf, fill=INK); ty += int(tf.size * 1.2)
    ew, eh = pill(d, x0 + 40 + ox, ty + 6, it["badge"], bang(40), col, padx=24, pady=6, center=False)
    py = ty + eh + 30
    for ln in wrap(it["premise"], deja(38), cw - 80):
        d.text((x0 + 40 + ox, py), ln, font=deja(38), fill=SUB); py += 50
    reveal_block(d, t, ox, "POR QUE VALE", col, it["why"])
    bolts(d, t, CFG["rating_label"], it["rating"], RCOL if idx < NI else ORA)
    if idx == NI and t > 0.7: pill(d, x0 + cw - 150 + ox, y0 - 40, "MEU #1", bang(56), ORA)
    footer_pill(d, idx, pulse)

def draw_ab(d, t, T, idx, it):
    col = it["col"]; pulse = header(d, t, T, idx, col); ox = card(d, t, col)
    vote = it["vote"]
    rows = [("A", it["a"], RED, y0 + 50), ("B", it["b"], BLUE, y0 + 230)]
    a = ease((t - 1.3) / 0.4)
    for letter, txt, lc, ry in rows:
        if letter == vote and a > 0:
            d.rounded_rectangle((x0 + 22 + ox, ry - 18, x0 + cw - 22 + ox, ry + 138), 28, outline=lerp(CREAM, INK, a), width=7)
        d.ellipse((x0 + 44 + ox, ry, x0 + 164 + ox, ry + 120), fill=lc, outline=INK, width=6)
        lf = dela(70); d.text((x0 + 104 + ox - lf.getlength(letter) / 2, ry + 12), letter, font=lf, fill=INK)
        tf, tl = fit_lines(txt, dela, 60, 34, cw - 240, 2)
        th = len(tl) * int(tf.size * 1.15)
        yy = ry + 60 - th / 2
        dim = letter != vote and a > 0
        for ln in tl:
            d.text((x0 + 190 + ox, yy), ln, font=tf, fill=lerp(INK, (170, 160, 190), 0.6 * a) if dim else INK)
            yy += int(tf.size * 1.15)
    vf = bang(56); ctext(d, y0 + 168, "VS", vf, ORA, INK, off=3, cx=x0 + 104 + ox)
    reveal_block(d, t, ox, f"MEU VOTO: {vote}", YEL, it["why"])
    if t > 0.9:
        s = 1 + 0.04 * pulse
        ctext(d, 1110, "E VOCÊ?", bang(int(64 * s)), CREAM, INK, off=4)
        pill(d, CX - 110, 1200, "A", dela(64), RED, padx=44)
        pill(d, CX + 110, 1200, "B", dela(64), BLUE, padx=44)
    footer_pill(d, idx, pulse)

def draw_flag(d, t, T, idx, it):
    col = it["col"]; pulse = header(d, t, T, idx, col); ox = card(d, t, col)
    pill(d, x0 + 40 + ox, y0 + 34, "🚩 RED FLAG".replace("🚩 ", ""), bang(48), RED, padx=28, pady=8, center=False)
    tf, tl = fit_lines(it["flag"], dela, 58, 38, cw - 80, 3)
    ty = y0 + 116
    for ln in tl:
        d.text((x0 + 40 + ox, ty), ln, font=tf, fill=INK); ty += int(tf.size * 1.18)
    for ln in wrap(it["why"], deja(36), cw - 80):
        d.text((x0 + 40 + ox, ty + 14), ln, font=deja(36), fill=SUB); ty += 48
    reveal_block(d, t, ox, "GREEN FLAG", GREEN, it["green"])
    bolts(d, t, CFG["rating_label"], it["rating"], RED)
    footer_pill(d, idx, pulse)

DRAW = {"list": draw_list, "ab": draw_ab, "flag": draw_flag}[KIND]

def draw_cta(d, t, T):
    l1, l2 = CFG["cta_title"]
    pop_text(d, 330, l1, dela, 96, CREAM, INK, t, 0.0, 880)
    pop_text(d, 450, l2, dela, 96, YEL, INK, t, 0.2, 880)
    for k, (txt, c) in enumerate(CFG["cta_boxes"]):
        a = ease((t - (0.7 + 0.4 * k)) / 0.35)
        if a <= 0: continue
        y = 650 + k * 165 + int((1 - a) * 70)
        d.rounded_rectangle((100, y + 9, 900, y + 139), 30, fill=INK)
        d.rounded_rectangle((100, y, 900, y + 130), 30, fill=COL[c], outline=INK, width=8)
        ctext(d, y + 24, txt, fit(txt, bang, 74, 740), INK)
    e = ease((t - 2.0) / 0.4)
    if e > 0:
        logo(d, CX, 1240, 0.8 * e + 0.01)
        ctext(d, 1345, "@nakama.zap", dela(58), CREAM, INK, off=6)
        ctext(d, 1425, CFG["cta_note"], fit(CFG["cta_note"], deja, 28, 860), (205, 195, 235))

SCENES = [("hook", BAR, YEL)]
for i in range(NI - 1): SCENES.append(("item", 3 * BAR, ITEMS[i]["col"]))
SCENES.append(("item", 2 * BAR, ITEMS[-1]["col"]))
SCENES.append(("cta", 1.5 * BAR, ORA))
starts = []; acc = 0
for _, dur, _c in SCENES: starts.append(acc); acc += dur
TOTAL = acc; N = int(round(TOTAL * FPS))
print("duração:", round(TOTAL, 2), "s frames:", N)

OUT = CFG["out"]
cmd = ["ffmpeg", "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgb24", "-s", f"{W}x{H}", "-r", str(FPS), "-i", "-",
       "-i", CFG["wav"], "-af", f"volume=0.9,afade=t=in:st=0:d=0.4,afade=t=out:st={TOTAL-1.8:.2f}:d=1.8",
       "-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "veryfast", "-crf", "20",
       "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-shortest", OUT]
p = subprocess.Popen(cmd, stdin=subprocess.PIPE)
prev_T = [2.3, starts[1] + 4.5, starts[NI] + 4.5, TOTAL - 0.15]; prev = {}
for fi in range(N):
    T = fi / FPS
    si = max(k for k in range(len(SCENES)) if starts[k] <= T + 1e-9)
    kind, dur, col = SCENES[si]; t = T - starts[si]
    im, d = make_bg(T, col)
    if kind == "hook": draw_hook(d, t, T)
    elif kind == "item": DRAW(d, t, T, si, ITEMS[si - 1])
    else: draw_cta(d, t, T)
    d.rectangle((0, 0, W, 14), fill=(30, 18, 70))
    d.rectangle((0, 0, int(W * T / TOTAL), 14), fill=YEL)
    for k, pt in enumerate(prev_T):
        if k not in prev and T >= pt: prev[k] = im.copy()
    p.stdin.write(im.tobytes())
p.stdin.close(); p.wait()
sheet = Image.new("RGB", (4 * 405, 720))
for k in range(4): sheet.paste(prev[k].resize((405, 720)), (k * 405, 0))
sheet.save(OUT.replace(".mp4", "_sheet.png"))
print("ok", OUT)
