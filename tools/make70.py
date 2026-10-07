import math, subprocess, functools
from PIL import Image, ImageDraw, ImageFont

W, H, FPS = 1080, 1920, 30
F = "/home/claude/reel/fonts/"
DEJA = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"
OUT = "/mnt/user-data/outputs/reel_modelo_70s_reviravoltas_nakamazap.mp4"
WAV = "/home/claude/music/lofi_nakama.wav"

@functools.lru_cache(maxsize=None)
def dela(s): return ImageFont.truetype(F + "Dela.ttf", int(s))
@functools.lru_cache(maxsize=None)
def bang(s): return ImageFont.truetype(F + "Bangers.ttf", int(s))
@functools.lru_cache(maxsize=None)
def deja(s): return ImageFont.truetype(DEJA, int(s))

NAVY = (20, 20, 46); DOT = (30, 30, 66); INK = (17, 17, 17); CREAM = (255, 246, 229)
YEL = (255, 210, 63); ORA = (255, 122, 26); RED = (230, 57, 70); BLUE = (127, 214, 255)
PINK = (255, 143, 177); GREEN = (182, 240, 106); SUB = (74, 63, 102); DIM = (45, 45, 85)

BPM = 76; BEAT = 60 / BPM; BAR = BEAT * 4
CX = 500  # centro visual (deixa folga para os botões laterais do TikTok)

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

def pill(d, cx, y, text, font, fill, tcol=INK, padx=38, pady=10):
    l, t, r, b = font.getbbox(text)
    w = (r - l) + 2 * padx; h = (b - t) + 2 * pady
    d.rounded_rectangle((cx - w / 2 + 6, y + 6, cx + w / 2 + 6, y + h + 6), h / 2, fill=INK)
    d.rounded_rectangle((cx - w / 2, y, cx + w / 2, y + h), h / 2, fill=fill, outline=INK, width=6)
    d.text((cx - w / 2 + padx - l, y + pady - t), text, font=font, fill=tcol)
    return h

def bolt(d, cx, y, s, fill, outline=INK, width=4):
    pts = [(122, 12), (24, 238), (98, 238), (64, 428), (186, 168), (108, 168), (152, 12)]
    p = [(cx + (px - 105) * s, y + (py - 12) * s) for px, py in pts]
    d.polygon(p, fill=fill, outline=outline, width=width)

def logo(d, cx, cy, s):
    d.ellipse((cx - 190*s, cy - 120*s, cx + 10*s, cy + 40*s), fill=CREAM, outline=INK, width=max(3, int(10*s)))
    d.ellipse((cx - 30*s, cy - 80*s, cx + 190*s, cy + 90*s), fill=YEL, outline=INK, width=max(3, int(10*s)))
    pts = [(122, 12), (24, 238), (98, 238), (64, 428), (186, 168), (108, 168), (152, 12)]
    k = 0.6 * s
    d.polygon([(cx - 55*s + px*k, cy - 165*s + py*k) for px, py in pts], fill=ORA, outline=INK, width=max(3, int(8*s)))

# fundo com retícula que desliza devagar (movimento constante = retenção)
dots = Image.new("RGB", (W + 160, H + 160), NAVY)
_dd = ImageDraw.Draw(dots)
for yy in range(0, H + 160, 40):
    for xx in range(0, W + 160, 40):
        ox = 20 if (yy // 40) % 2 else 0
        _dd.ellipse((xx + ox - 4, yy - 4, xx + ox + 4, yy + 4), fill=DOT)

def make_bg(T, col):
    o = int(T * 14) % 80
    im = dots.crop((o, o, o + W, o + H))
    d = ImageDraw.Draw(im)
    d.polygon([(0, 1500), (W, 1250), (W, 1430), (0, 1710)], fill=lerp(NAVY, col, 0.22))
    return im, d

ITEMS = [
    ("PSYCHO-PASS", "Um sistema que julga as pessoas antes mesmo de o crime acontecer.", "Faz pensar sobre justiça sem virar aula.", 3, RED),
    ("ERASED", "Um homem volta no tempo para impedir uma tragédia.", "Suspense que prende logo nos primeiros episódios.", 3, BLUE),
    ("STEINS;GATE", "Mensagens para o passado começam a mudar o presente.", "Começa devagar, mas o final recompensa a paciência.", 4, PINK),
    ("MONSTER", "Um médico caça o homem que ele mesmo salvou.", "Thriller psicológico, sem pressa de agradar.", 4, GREEN),
    ("CODE GEASS", "Um príncipe exilado lidera uma rebelião com um poder misterioso.", "Estratégia pura, com virada a cada poucos episódios.", 5, ORA),
    ("ATTACK ON TITAN", "Ataque dos Titãs: a humanidade vive cercada por muralhas e titãs.", "O mundo da história fica maior a cada temporada.", 5, BLUE),
    ("FULLMETAL ALCHEMIST: BROTHERHOOD", "Dois irmãos usam alquimia na busca pelo que perderam.", "Ação, drama e um final redondo. Sem enrolação.", 5, YEL),
]

# ---------- cenas ----------
HOOK_LINES = [("7 ANIMES", CREAM, 150), ("COM", CREAM, 90), ("REVIRAVOLTA", YEL, 130), ("QUE VOCÊ", CREAM, 120), ("NÃO VÊ VINDO", CREAM, 110)]
_hook_layout = []
_y = 400
for txt, colr, sz in HOOK_LINES:
    f = fit(txt, dela, sz, 880)
    _hook_layout.append((txt, colr, sz, _y))
    _y += int(f.size * 1.2) + 4

def draw_hook(d, t, T):
    shake = math.sin(t * 70) * 5 if t < 0.8 else 0
    s = back((t - 0.05) / 0.35)
    if s > 0.05:
        pill(d, CX, 290, "SALVA ESSE!", bang(max(10, int(66 * s))), YEL)
    for i, (txt, colr, sz, y) in enumerate(_hook_layout):
        pop_text(d, y + (shake if i % 2 == 0 else -shake), txt, dela, sz, colr, INK, t, 0.15 + i * 0.2, 880)
    b = ease((t - 1.4) / 0.35)
    if b > 0:
        y = 1160 + (1 - b) * 90
        d.rounded_rectangle((100, y + 10, 900, y + 160), 30, fill=INK)
        d.rounded_rectangle((100, y, 900, y + 150), 30, fill=ORA, outline=INK, width=8)
        ctext(d, y + 34, "O ÚLTIMO É O MELHOR", fit("O ÚLTIMO É O MELHOR", bang, 84, 740), INK, cx=CX)

def draw_item(d, t, T, idx, item):
    title, premise, why, rating, col = item
    last = idx == 7
    # contador "3/7" pulsando no ritmo da música
    pulse = max(0, 1 - ((T % BEAT) / BEAT) * 3.5)
    cs = int(130 * (1 + 0.05 * pulse) * (0.6 + 0.4 * back(t / 0.35)))
    d.text((86, 200), f"{idx}/7", font=dela(cs), fill=INK)
    d.text((80, 194), f"{idx}/7", font=dela(cs), fill=col)
    tag = "@nakama.zap"
    d.text((920 - deja(34).getlength(tag), 240), tag, font=deja(34), fill=CREAM)

    x0, y0, cw, ch = 80, 430, 840, 640
    ox = int((1 - ease(t / 0.45)) * W)
    d.rounded_rectangle((x0 + 16 + ox, y0 + 16, x0 + cw + 16 + ox, y0 + ch + 16), 38, fill=col)
    d.rounded_rectangle((x0 + ox, y0, x0 + cw + ox, y0 + ch), 38, fill=CREAM, outline=INK, width=8)

    # título (1 linha se couber, senão 2)
    tf = None; tlines = None
    for size in range(84, 55, -2):
        l = wrap(title, dela(size), cw - 80)
        if len(l) == 1: tf, tlines = dela(size), l; break
    if tf is None:
        for size in range(70, 40, -2):
            l = wrap(title, dela(size), cw - 80)
            if len(l) <= 2: tf, tlines = dela(size), l; break
    ty = y0 + 36
    for ln in tlines:
        d.text((x0 + 40 + ox, ty), ln, font=tf, fill=INK)
        ty += int(tf.size * 1.2)
    pf = deja(40)
    py = ty + 18
    for ln in wrap(premise, pf, cw - 80):
        d.text((x0 + 40 + ox, py), ln, font=pf, fill=SUB)
        py += 54

    # "por que vale" (critério próprio) aparece depois
    wf = deja(38)
    wl = wrap(why, wf, cw - 80)
    why_h = len(wl) * 50
    pf2 = bang(46)
    l_, t_, r_, b_ = pf2.getbbox("POR QUE VALE")
    pill_h = (b_ - t_) + 20
    base_y = y0 + ch - 36 - why_h
    a = ease((t - 1.3) / 0.4)
    if a > 0:
        yy = base_y - 14 - pill_h + int((1 - a) * 25)
        pw = (r_ - l_) + 60
        d.rounded_rectangle((x0 + 40 + ox, yy, x0 + 40 + pw + ox, yy + pill_h), pill_h / 2, fill=col, outline=INK, width=5)
        d.text((x0 + 70 - l_ + ox, yy + 10 - t_), "POR QUE VALE", font=pf2, fill=INK)
        tc = lerp(CREAM, INK, a)
        for k, ln in enumerate(wl):
            d.text((x0 + 40 + ox, base_y + k * 50 + int((1 - a) * 25)), ln, font=wf, fill=tc)

    # nota de plot twist = opinião própria
    if t > 0.9:
        ctext(d, 1108, "NÍVEL DE PLOT TWIST", bang(58), CREAM, INK, off=4)
        for i in range(5):
            bx = CX + (i - 2) * 80
            ti = 2.0 + i * 0.28
            if i < rating and t >= ti:
                sc = 0.27 * (0.6 + 0.4 * back((t - ti) / 0.3))
                bolt(d, bx, 1190, sc, YEL if not last else ORA)
            else:
                bolt(d, bx, 1190, 0.27, DIM, outline=(70, 70, 110), width=3)
    # promessa de retenção
    if last:
        if t > 0.7:
            pill(d, x0 + cw - 150 + ox, y0 - 40, "MEU #1", bang(56), ORA)
        pill(d, CX, 1350, "PROMESSA CUMPRIDA", bang(54), ORA)
    else:
        sz = int(54 * (1 + 0.03 * pulse))
        pill(d, CX, 1350, "O ÚLTIMO É O MELHOR", bang(sz), YEL)

def draw_cta(d, t, T):
    pop_text(d, 330, "QUAL REVIRAVOLTA", dela, 96, CREAM, INK, t, 0.0, 880)
    pop_text(d, 450, "VOCÊ NÃO VIU VINDO?", dela, 96, YEL, INK, t, 0.2, 880)
    boxes = [("COMENTA O NÚMERO", CREAM, 0.7), ("SALVA PRA MARATONAR", ORA, 1.1), ("SEGUE PRA PARTE 2", YEL, 1.5)]
    for k, (txt, colr, st) in enumerate(boxes):
        a = ease((t - st) / 0.35)
        if a <= 0: continue
        y = 650 + k * 165 + int((1 - a) * 70)
        d.rounded_rectangle((100, y + 9, 900, y + 139), 30, fill=INK)
        d.rounded_rectangle((100, y, 900, y + 130), 30, fill=colr, outline=INK, width=8)
        ctext(d, y + 24, txt, fit(txt, bang, 74, 740), INK, cx=CX)
    e = ease((t - 2.0) / 0.4)
    if e > 0:
        logo(d, CX, 1240, 0.8 * e + 0.01)
        ctext(d, 1345, "@nakama.zap", dela(58), CREAM, INK, off=6)
        ctext(d, 1425, "Nota de plot twist = opinião do @nakama.zap", deja(28), (200, 190, 230))

SCENES = [("hook", BAR, YEL)]
for i in range(6): SCENES.append(("item", 3 * BAR, ITEMS[i][4]))
SCENES.append(("item", 2 * BAR, ITEMS[6][4]))
SCENES.append(("cta", 1.5 * BAR, ORA))
starts = []; acc = 0
for _, dur, _c in SCENES:
    starts.append(acc); acc += dur
TOTAL = acc
N = int(round(TOTAL * FPS))
print("duração total:", round(TOTAL, 2), "s  frames:", N)

cmd = ["ffmpeg", "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgb24", "-s", f"{W}x{H}", "-r", str(FPS), "-i", "-",
       "-i", WAV, "-af", f"afade=t=in:st=0:d=0.4,afade=t=out:st={TOTAL-1.8:.2f}:d=1.8",
       "-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "veryfast", "-crf", "19",
       "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-shortest", OUT]
p = subprocess.Popen(cmd, stdin=subprocess.PIPE)

prev_T = [2.3, starts[1] + 4.5, starts[7] + 4.5, TOTAL - 0.15]
prev = {}
for fi in range(N):
    T = fi / FPS
    si = max(k for k in range(len(SCENES)) if starts[k] <= T + 1e-9)
    kind, dur, col = SCENES[si]
    t = T - starts[si]
    im, d = make_bg(T, col)
    if kind == "hook": draw_hook(d, t, T)
    elif kind == "item": draw_item(d, t, T, si, ITEMS[si - 1])
    else: draw_cta(d, t, T)
    d.rectangle((0, 0, W, 14), fill=(30, 18, 70))
    d.rectangle((0, 0, int(W * T / TOTAL), 14), fill=YEL)
    for k, pt in enumerate(prev_T):
        if k not in prev and T >= pt:
            prev[k] = im.copy()
    p.stdin.write(im.tobytes())
p.stdin.close(); p.wait()

sheet = Image.new("RGB", (4 * 405, 720))
for k in range(4):
    sheet.paste(prev[k].resize((405, 720)), (k * 405, 0))
sheet.save("/home/claude/reel/sheet70.png")
print("ok")
