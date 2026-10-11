import math, subprocess, functools, sys, random, wave
import numpy as np
from PIL import Image, ImageDraw, ImageFont, ImageFilter, ImageChops

OPT = sys.argv[1]  # A, B ou C
OUT = sys.argv[2]
W, H, FPS, DUR = 1080, 1920, 30, 10.0
F = "/home/claude/reel/fonts/"
DEJA = "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf"

@functools.lru_cache(maxsize=None)
def dela(s): return ImageFont.truetype(F + "Dela.ttf", max(8, int(s)))
@functools.lru_cache(maxsize=None)
def bang(s): return ImageFont.truetype(F + "Bangers.ttf", max(8, int(s)))
@functools.lru_cache(maxsize=None)
def deja(s): return ImageFont.truetype(DEJA, max(8, int(s)))

BASE = (42, 27, 92); INK = (17, 17, 17); CREAM = (255, 246, 229)
YEL = (255, 210, 63); ORA = (255, 122, 26); RED = (230, 57, 70); BLUE = (127, 214, 255)
PINK = (255, 143, 177); GREEN = (182, 240, 106); GRAY = (150, 140, 175)
CX = 520
BPM = 140; BEAT = 60 / BPM

def clamp(x): return max(0.0, min(1.0, x))
def ease(t): t = clamp(t); return 1 - (1 - t) ** 3
def back(t):
    t = clamp(t); c = 2.2
    return 1 + (c + 1) * (t - 1) ** 3 + c * (t - 1) ** 2
def lerp(a, b, t): return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(len(a)))

def fit(text, ff, size, maxw):
    while size > 16 and ff(size).getlength(text) > maxw: size -= 2
    return ff(size)

def ctext(d, y, text, font, fill, shadow=INK, off=7, cx=CX):
    x = cx - font.getlength(text) / 2
    if shadow: d.text((x + off, y + off), text, font=font, fill=shadow)
    d.text((x, y), text, font=font, fill=fill)

def pop(d, y, text, ff, size, fill, t, start, maxw=900, shadow=INK, cx=CX):
    s = back((t - start) / 0.3)
    if s <= 0.05: return
    f = fit(text, ff, size, maxw); sz = f.size * s
    ctext(d, y + (f.size - sz) / 2, text, ff(sz), fill, shadow, max(3, int(7 * s)), cx)

def pill(d, cx, y, text, font, fill, tcol=INK, padx=34, pady=10):
    l, t, r, b = font.getbbox(text); w = r - l + 2 * padx; h = b - t + 2 * pady
    d.rounded_rectangle((cx - w/2 + 6, y + 6, cx + w/2 + 6, y + h + 6), h/2, fill=INK)
    d.rounded_rectangle((cx - w/2, y, cx + w/2, y + h), h/2, fill=fill, outline=INK, width=6)
    d.text((cx - w/2 + padx - l, y + pady - t), text, font=font, fill=tcol)

# ---------------- partículas ----------------
class Burst:
    def __init__(s, t0, x, y, n, cols, speed=900, life=1.0, grav=1400, size=10, seed=1, up=False):
        r = random.Random(seed); s.t0 = t0; s.p = []
        for _ in range(n):
            a = r.uniform(-math.pi, 0) if up else r.uniform(0, 2 * math.pi)
            v = r.uniform(0.3, 1) * speed
            s.p.append((x, y, math.cos(a) * v, math.sin(a) * v, r.uniform(0.5, 1) * life, r.choice(cols), r.uniform(0.5, 1) * size))
        s.grav = grav
    def draw(s, d, T):
        t = T - s.t0
        if t < 0: return
        for x, y, vx, vy, life, c, sz in s.p:
            if t > life: continue
            k = 1 - t / life
            px = x + vx * t; py = y + vy * t + 0.5 * s.grav * t * t
            r_ = sz * (0.4 + 0.6 * k)
            d.ellipse((px - r_, py - r_, px + r_, py + r_), fill=c)

def glow_layer(draw_fn, radius=28):
    g = Image.new("RGB", (W // 2, H // 2), (0, 0, 0)); gd = ImageDraw.Draw(g)
    draw_fn(gd, 0.5)
    return g.filter(ImageFilter.GaussianBlur(radius / 2)).resize((W, H))

def shake_xy(T, hits, amp=26, dur=0.35):
    dx = dy = 0
    for h in hits:
        t = T - h
        if 0 <= t < dur:
            k = (1 - t / dur) * amp
            dx += math.sin(t * 90) * k; dy += math.cos(t * 77) * k
    return int(dx), int(dy)

def flash_amt(T, hits, dur=0.18):
    a = 0
    for h in hits:
        t = T - h
        if 0 <= t < dur: a = max(a, 1 - t / dur)
    return a

# ---------------- ilustrações próprias ----------------
def anvil(d, cx, cy, s=1.0):
    P = lambda pts: [(cx + x * s, cy + y * s) for x, y in pts]
    d.polygon(P([(-260, -60), (200, -60), (300, -20), (200, 0), (120, 10), (90, 60), (-90, 60), (-120, 10), (-260, 0)]), fill=(70, 70, 90), outline=INK, width=8)
    d.polygon(P([(-90, 60), (90, 60), (130, 160), (-130, 160)]), fill=(55, 55, 75), outline=INK, width=8)
    d.rectangle(P([(-170, 160), (170, 200)]), fill=(45, 45, 62), outline=INK, width=8)
    d.line(P([(-240, -48), (190, -48)]), fill=(120, 120, 150), width=int(8 * s))

def hammer(d, px, py, ang, s=1.0):
    # px,py = pivô (cabo); ang em radianos (0 = horizontal p/ esquerda)
    L = 330 * s
    hx = px - math.cos(ang) * L; hy = py - math.sin(ang) * L
    d.line((px, py, hx, hy), fill=(120, 72, 40), width=int(34 * s))
    d.line((px, py, hx, hy), fill=INK, width=int(6 * s))
    nx, ny = -math.sin(ang), math.cos(ang)
    hw, hl = 70 * s, 120 * s
    ux, uy = math.cos(ang), math.sin(ang)
    pts = [(hx + nx * hl - ux * hw, hy + ny * hl - uy * hw), (hx + nx * hl + ux * hw, hy + ny * hl + uy * hw),
           (hx - nx * hl + ux * hw, hy - ny * hl + uy * hw), (hx - nx * hl - ux * hw, hy - ny * hl - uy * hw)]
    d.polygon(pts, fill=(140, 140, 165), outline=INK, width=int(8 * s))
    return hx, hy

def sword(d, cx, tipy, length, col=CREAM, s=1.0):
    bw = 46 * s
    d.polygon([(cx, tipy), (cx + bw, tipy + 90 * s), (cx + bw, tipy + length), (cx - bw, tipy + length), (cx - bw, tipy + 90 * s)], fill=col, outline=INK, width=8)
    d.line((cx, tipy + 40 * s, cx, tipy + length - 10), fill=lerp(col, (255, 255, 255), 0.6), width=int(8 * s))
    gy = tipy + length
    d.rounded_rectangle((cx - 150 * s, gy, cx + 150 * s, gy + 42 * s), 18 * s, fill=YEL, outline=INK, width=8)
    d.rectangle((cx - 22 * s, gy + 42 * s, cx + 22 * s, gy + 200 * s), fill=(120, 72, 40), outline=INK, width=7)
    d.ellipse((cx - 38 * s, gy + 190 * s, cx + 38 * s, gy + 266 * s), fill=ORA, outline=INK, width=7)

def book(d, cx, cy, s=1.0):
    d.rounded_rectangle((cx - 110*s + 10, cy - 140*s + 10, cx + 110*s + 10, cy + 140*s + 10), 16, fill=INK)
    d.rounded_rectangle((cx - 110*s, cy - 140*s, cx + 110*s, cy + 140*s), 16, fill=(120, 40, 50), outline=INK, width=8)
    d.rectangle((cx - 110*s, cy - 140*s, cx - 80*s, cy + 140*s), fill=(90, 28, 38))
    d.ellipse((cx - 50*s, cy - 50*s, cx + 50*s, cy + 50*s), fill=YEL, outline=INK, width=6)
    hammer(d, cx + 20*s, cy + 30*s, math.radians(-40), 0.18 * s)

# ---------------- áudio ----------------
SR = 44100; N = int(SR * DUR)
mix = np.zeros(N)
rng = np.random.default_rng(5)
def add(sig, t0, g=1.0):
    i = int(t0 * SR)
    if i >= N: return
    n = min(len(sig), N - i); mix[i:i + n] += sig[:n] * g
def tt(d): return np.arange(int(SR * d)) / SR
def kick():
    t = tt(0.4); f = 42 + 120 * np.exp(-t * 30)
    return np.sin(2 * np.pi * np.cumsum(f) / SR) * np.exp(-t * 7) * 0.9
def snare():
    t = tt(0.22); return rng.standard_normal(len(t)) * np.exp(-t * 20) * 0.35 + np.sin(2*np.pi*190*t) * np.exp(-t*30) * 0.3
def hat(v=1):
    t = tt(0.05); return np.diff(rng.standard_normal(len(t) + 1)) * np.exp(-t * 90) * 0.12 * v
def sub(f, d):
    t = tt(d); return np.sin(2 * np.pi * f * t) * np.exp(-t * 2.2) * (1 - np.exp(-t * 200)) * 0.55
def blip(f=1400, d=0.07, g=0.25):
    t = tt(d); return np.sign(np.sin(2 * np.pi * f * t)) * np.exp(-t * 40) * g
def chime(base=880, g=0.3):
    out = np.zeros(int(SR * 1.2))
    for k, m in enumerate([0, 4, 7, 12, 16]):
        t = tt(0.9); f = base * 2 ** (m / 12)
        s = (np.sin(2*np.pi*f*t) + 0.3*np.sin(4*np.pi*f*t)) * np.exp(-t * 4) * g
        i = int(k * 0.06 * SR); out[i:i + len(s)] += s[:len(out) - i]
    return out
def whoosh(d=0.45, g=0.35):
    t = tt(d); nz = rng.standard_normal(len(t))
    X = np.fft.rfft(nz); fr = np.fft.rfftfreq(len(nz), 1 / SR)
    nz = np.fft.irfft(X / (1 + (fr / 2500) ** 2), len(nz))
    env = np.sin(np.pi * t / d) ** 2
    return nz / np.max(np.abs(nz)) * env * g
def impact(g=0.9):
    t = tt(0.9); f = 30 + 90 * np.exp(-t * 12)
    return (np.sin(2*np.pi*np.cumsum(f)/SR) * np.exp(-t * 4) + rng.standard_normal(len(t)) * np.exp(-t * 25) * 0.4) * g
def clang(g=0.45):
    t = tt(1.3); s = np.zeros(len(t))
    for f, a in [(820, 1), (1290, 0.7), (1870, 0.5), (2710, 0.35), (3550, 0.2)]:
        s += a * np.sin(2*np.pi*f*t) * np.exp(-t * (3 + f / 900))
    s += rng.standard_normal(len(t)) * np.exp(-t * 60) * 0.8
    return s * g
def riser(d=1.5, g=0.25):
    t = tt(d); f = 200 + 1600 * (t / d) ** 2
    return np.sin(2*np.pi*np.cumsum(f)/SR) * (t / d) ** 2 * g + rng.standard_normal(len(t)) * (t / d) ** 3 * 0.08

def beat_track(start=0.0, stop=DUR, chords=(36, 36, 39, 34)):
    b = 0; t = start
    while t < stop - 0.05:
        bar_i = int(b // 8)
        if b % 8 in (0, 3, 6): add(kick(), t)
        if b % 8 in (2, 6): add(snare(), t, 0.8)
        add(hat(1 if b % 2 == 0 else 0.6), t)
        if b % 8 == 0:
            root = chords[bar_i % len(chords)]
            add(sub(440 * 2 ** ((root - 69) / 12), BEAT * 4), t)
        b += 1; t += BEAT / 2 * 2 / 2  # colcheia
        t = start + b * BEAT / 2

def finish_audio(path):
    m = mix.copy()
    fade = int(SR * 0.6); m[-fade:] *= np.linspace(1, 0, fade)
    m = m / (np.max(np.abs(m)) + 1e-9) * 0.9
    st = np.stack([m, m], 1)
    with wave.open(path, "wb") as w:
        w.setnchannels(2); w.setsampwidth(2); w.setframerate(SR)
        w.writeframes((st * 32767).astype("<i2").tobytes())

# ---------------- cena A: tela de RPG ----------------
def scene_A():
    BG = (12, 12, 34); CY = (127, 214, 255)
    hits = [3.2, 7.0]
    bursts = [Burst(1.45, CX, 980, 60, [YEL, ORA, CREAM], 1100, 1.1, 900, 12, 3),
              Burst(5.2, CX, 980, 90, [YEL, ORA, CREAM, GREEN], 1300, 1.3, 800, 14, 4),
              Burst(7.0, CX, 1020, 70, [YEL, ORA, RED], 1200, 1.0, 1200, 12, 5)]
    events = [(0.05, impact(0.6)), (0.6, blip(1200)), (0.75, blip(1600)), (1.4, chime(880)), (1.4, whoosh()),
              (3.2, impact(0.8))] + [(3.3 + k * 0.09, blip(900 - k * 60, 0.06, 0.18)) for k in range(10)] + \
             [(5.2, chime(660, 0.35)), (5.2, whoosh(0.6)), (5.25, chime(1320, 0.2)), (5.6, riser(1.4)), (7.0, impact(1.0)), (7.0, chime(990, 0.3))]
    beat_track(1.4, DUR)
    for t0, s in events: add(s, t0)

    def win(d, x0, y0, x1, y1, col, title, a):
        y0 += (1 - a) * 60
        d.rectangle((x0 + 10, y0 + 10, x1 + 10, y1 + 10), fill=(0, 0, 0))
        d.rectangle((x0, y0, x1, y1), fill=(18, 24, 58), outline=col, width=6)
        d.rectangle((x0, y0, x1, y0 + 64), fill=col)
        d.text((x0 + 24, y0 + 10), title, font=bang(48), fill=INK)
        return y0

    def frame(T):
        im = Image.new("RGB", (W, H), BG); d = ImageDraw.Draw(im)
        for k in range(0, H, 6): d.line((0, k, W, k), fill=(16, 16, 42))
        sy = int((T * 380) % H); d.rectangle((0, sy, W, sy + 4), fill=(30, 40, 80))
        # gancho
        if T < 3.2:
            pop(d, 200, "ELE ERA O PIOR", dela, 92, CREAM, T, 0.1)
            pop(d, 310, "JOGADOR DO SERVIDOR", dela, 70, YEL, T, 0.3)
        elif T < 5.2:
            pop(d, 200, "E AÍ PIOROU...", dela, 92, RED, T, 3.2)
        elif T < 7.0:
            pop(d, 200, "ATÉ QUE...", dela, 100, YEL, T, 5.2)
        # janela 1: item
        if 0.6 <= T < 3.2:
            a = ease((T - 0.6) / 0.3)
            y0 = win(d, 110, 520, 930, 1360, CY, "[SISTEMA]", a)
            txt = "Você encontrou um item."
            n = int(len(txt) * clamp((T - 0.7) / 0.5))
            d.text((150, y0 + 100), txt[:n], font=deja(40), fill=CREAM)
            if T >= 1.4:
                k = ease((T - 1.4) / 0.4)
                cx, cy = CX, y0 + 470
                ray = Image.new("RGB", (W, H), (0, 0, 0)); rd = ImageDraw.Draw(ray)
                for i in range(12):
                    ang = T * 0.8 + i * math.pi / 6
                    rd.polygon([(cx, cy), (cx + math.cos(ang) * 420, cy + math.sin(ang) * 420), (cx + math.cos(ang + 0.15) * 420, cy + math.sin(ang + 0.15) * 420)], fill=lerp((0, 0, 0), (120, 90, 10), k))
                im.paste(ImageChops.add(im, ray)); d = ImageDraw.Draw(im)
                book(d, cx, cy, 0.6 + 0.4 * back((T - 1.4) / 0.4))
                pop(d, y0 + 690, "LIVRO DO FERREIRO LENDÁRIO", bang, 64, YEL, T, 1.6, 760)
                pill(d, CX, y0 + 160, "RARIDADE: LENDÁRIA", bang(46), ORA)
        # janela 2: aviso
        if 3.2 <= T < 5.2:
            a = ease((T - 3.2) / 0.2)
            y0 = win(d, 110, 560, 930, 1220, RED, "[AVISO]", a)
            d.text((150, y0 + 110), "Ao aprender a habilidade,", font=deja(40), fill=CREAM)
            d.text((150, y0 + 160), "seu nível foi resetado.", font=deja(40), fill=CREAM)
            lvl = max(1, int(87 - 86 * ease((T - 3.3) / 1.0)))
            ctext(d, y0 + 280, f"NÍVEL {lvl}", dela(150), RED if lvl == 1 else CREAM)
        # janela 3: classe
        if 5.2 <= T < 7.0:
            a = ease((T - 5.2) / 0.25)
            g = glow_layer(lambda gd, s: gd.rectangle((110 * s, 600 * s, 930 * s, 1300 * s), outline=YEL, width=int(40 * s)))
            im.paste(ImageChops.add(im, g.point(lambda v: v * a))); d = ImageDraw.Draw(im)
            y0 = win(d, 110, 600, 930, 1300, YEL, "[LEVEL UP]", a)
            d.text((150, y0 + 100), "Classe desbloqueada:", font=deja(42), fill=CREAM)
            pop(d, y0 + 220, "FERREIRO", dela, 120, YEL, T, 5.35, 760)
            pop(d, y0 + 370, "LENDÁRIO", dela, 120, ORA, T, 5.55, 760)
            hammer(d, CX + 160, y0 + 640, math.radians(-30 + 25 * math.sin(T * 9)), 0.55)
        # final
        if T >= 7.0:
            g = glow_layer(lambda gd, s: ctext(gd, 760 * s, "OVERGEARED", dela(150 * s), ORA, None, cx=CX * s), 40)
            im.paste(ImageChops.add(im, g)); d = ImageDraw.Draw(im)
            pop(d, 760, "OVERGEARED", dela, 150, YEL, T, 7.0, 940)
            pop(d, 560, "O AZARADO QUE VIROU LENDA", bang, 76, CREAM, T, 7.3, 900)
            if T > 7.6: pill(d, CX, 980, "NOVO NA CRUNCHYROLL • OUT/2026", bang(50), BLUE)
            if T > 8.1:
                a = ease((T - 8.1) / 0.3); y = 1180 + (1 - a) * 80
                d.rounded_rectangle((110, y + 10, 930, y + 170), 30, fill=INK)
                d.rounded_rectangle((110, y, 930, y + 160), 30, fill=ORA, outline=INK, width=8)
                ctext(d, y + 22, "7 MOTIVOS PRA ASSISTIR", fit("7 MOTIVOS PRA ASSISTIR", bang, 70, 760), INK, None)
                ctext(d, y + 92, "SEGUE PRA VER", bang(50), CREAM, INK, 3)
        for b in bursts: b.draw(d, T)
        ctext(d, 1460, "@nakama.zap", dela(48), CREAM)
        return im, hits
    return frame

# ---------------- cena B: forja ----------------
def scene_B():
    strikes = [1.2, 2.9, 4.6]
    words = [("AZARADO", GRAY), ("FERREIRO", ORA), ("LENDÁRIO", YEL)]
    AX, AY = CX, 1180
    bursts = [Burst(s, AX - 20, AY - 70, 70, [YEL, ORA, (255, 240, 180)], 1300, 0.9, 1600, 11, 10 + i, up=True) for i, s in enumerate(strikes)]
    bursts.append(Burst(7.6, CX, 760, 90, [YEL, ORA, CREAM], 1400, 1.2, 900, 13, 20))
    hits = strikes + [7.6]
    beat_track(0.0, DUR, (36, 36, 34, 39))
    for s in strikes: add(clang(), s); add(impact(0.5), s)
    add(riser(1.7), 5.8); add(whoosh(0.6), 6.2); add(impact(1.0), 7.6); add(chime(990, 0.3), 7.6)

    def hammer_angle(T):
        # sobe devagar e desce rápido antes de cada golpe
        base = math.radians(-70)
        for s in strikes:
            if s - 0.9 <= T < s:
                k = (T - (s - 0.9)) / 0.9
                return base + (math.radians(-10) - base) * (k ** 4)
            if s <= T < s + 0.35:
                return math.radians(-10) + (base - math.radians(-10)) * ease((T - s) / 0.35)
        return base

    def frame(T):
        im = Image.new("RGB", (W, H), (26, 14, 40)); d = ImageDraw.Draw(im)
        heat = max(flash_amt(T, strikes, 0.6), 0.25 + 0.15 * math.sin(T * 3))
        g = glow_layer(lambda gd, s: gd.ellipse(((AX - 420) * s, (AY - 300) * s, (AX + 420) * s, (AY + 260) * s), fill=lerp((0, 0, 0), (200, 80, 10), heat)), 60)
        im.paste(ImageChops.add(im, g)); d = ImageDraw.Draw(im)
        pop(d, 190, "ELE ERA UM ZÉ NINGUÉM", dela, 74, CREAM, T, 0.1, 920)
        pop(d, 290, "NUM JOGO DE RPG...", dela, 74, YEL, T, 0.35, 920)
        # palavra estampada a cada golpe
        cur = None
        for i, s in enumerate(strikes):
            if T >= s: cur = (i, s)
        if cur and T < 5.8:
            i, s = cur; w, c = words[i]
            sc = 1 + 0.5 * (1 - ease((T - s) / 0.25))
            f = fit(w, dela, 150, 900); f = dela(f.size * sc)
            ctext(d, 560 - (f.size - 150) / 2, w, f, c, INK, 10)
            if i == 2:
                ctext(d, 760, "UM ITEM MUDOU TUDO", bang(64), CREAM, INK, 4)
        # espada surgindo
        if T >= 5.8:
            k = ease((T - 5.8) / 1.4)
            tip = AY - 120 - 620 * k
            gg = glow_layer(lambda gd, s: gd.polygon([(CX * s, (tip - 40) * s), ((CX + 90) * s, (tip + 90) * s), ((CX + 90) * s, (tip + 560) * s), ((CX - 90) * s, (tip + 560) * s), ((CX - 90) * s, (tip + 90) * s)], fill=(255, 180, 40)), 50)
            im.paste(ImageChops.add(im, gg))
            lay = Image.new("RGBA", (W, H), (0, 0, 0, 0)); ld = ImageDraw.Draw(lay)
            sword(ld, CX, tip, 520, CREAM, 1.0)
            sx = (T * 1.6) % 1.0; sy = tip + 40 + sx * 480
            ld.polygon([(CX - 46, sy), (CX + 46, sy - 40), (CX + 46, sy - 10), (CX - 46, sy + 30)], fill=(255, 255, 255, 255))
            ld.rectangle((0, AY - 75, W, H), fill=(0, 0, 0, 0))
            im.paste(lay, (0, 0), lay); d = ImageDraw.Draw(im)
        anvil(d, AX, AY, 1.15)
        if T < 5.8:
            hammer(d, AX + 360, AY - 260, hammer_angle(T), 1.0)
        if T >= 7.6:
            d.rectangle((0, 1260, W, 1720), fill=(26, 14, 40))
            pop(d, 1300, "OVERGEARED", dela, 150, YEL, T, 7.6, 940)
            if T > 7.9: pill(d, CX, 1500, "NOVO NA CRUNCHYROLL • OUT/2026", bang(50), ORA)
            if T > 8.4: ctext(d, 1620, "SEGUE PRA VER OS 7 MOTIVOS", fit("SEGUE PRA VER OS 7 MOTIVOS", bang, 64, 900), CREAM)
        elif T >= 6.3:
            pop(d, 1460, "E COMEÇOU A FORJAR LENDAS", bang, 70, CREAM, T, 6.3, 900)
        for b in bursts: b.draw(d, T)
        if T < 7.6 or T > 8.0: d.text((60, 1760), "@nakama.zap", font=dela(40), fill=CREAM)
        return im, hits
    return frame

# ---------------- cena C: tier list ----------------
def scene_C():
    tiers = [("S", RED), ("A", ORA), ("B", YEL), ("C", GREEN)]
    TY0, TH = 470, 200
    slots = {0: 2, 1: 3, 2: 3, 3: 2}
    path = [(1.6, None), (4.4, 3), (4.75, 2), (5.1, 1), (5.45, 2), (5.85, 0)]
    land = 5.85
    hits = [land]
    bursts = [Burst(land, 330 + 2 * 150 + 65, TY0 + 100, 90, [YEL, CREAM, PINK, BLUE], 1300, 1.2, 900, 13, 30)]
    beat_track(0.0, 2.2); beat_track(6.0, DUR)
    for k in range(4):
        for j in range(slots[k]): add(blip(700 + 150 * k + 60 * j, 0.06, 0.15), 0.35 + k * 0.2 + j * 0.06)
    add(whoosh(0.6), 1.6)
    t = 2.3; gap = 0.22
    while t < 4.35: add(snare(), t, 0.5); t += gap; gap = max(0.06, gap * 0.9)
    for tt_, _ in path[1:-1]: add(blip(1100, 0.08, 0.3), tt_); add(whoosh(0.25, 0.2), tt_ - 0.1)
    add(impact(1.0), land); add(chime(880, 0.35), land)

    def card(d, cx, cy, s, rot_dummy=0):
        w, h = 260 * s, 180 * s
        d.rounded_rectangle((cx - w/2 + 10, cy - h/2 + 10, cx + w/2 + 10, cy + h/2 + 10), 22, fill=INK)
        d.rounded_rectangle((cx - w/2, cy - h/2, cx + w/2, cy + h/2), 22, fill=CREAM, outline=INK, width=7)
        hammer(d, cx + 70 * s, cy - 10 * s, math.radians(-35), 0.22 * s)
        f = fit("OVERGEARED", dela, 40 * s, w - 24)
        d.text((cx - f.getlength("OVERGEARED") / 2, cy + h/2 - f.size - 18 * s), "OVERGEARED", font=f, fill=INK)

    def pos(T):
        # retorna (x, y, escala)
        hover = (CX, 1500, 1.25)
        if T < 1.6: return None
        if T < 2.2:
            k = ease((T - 1.6) / 0.6); return (CX, 2100 - 600 * k, 1.25)
        if T < 4.4: return (CX, 1500 + math.sin(T * 6) * 18, 1.25 + 0.04 * math.sin(T * 12))
        prev = hover
        for i in range(1, len(path)):
            t0, tier = path[i]
            tgt = (330 + slots[tier] * 150 + 65 if tier is not None else CX, TY0 + tier * TH + TH / 2, 0.62)
            if T < t0:
                pt0 = path[i - 1][0]
                k = ease((T - pt0) / (t0 - pt0)) if i > 1 else ease((T - 4.4) / 0.001)
                return tuple(prev[j] + (tgt[j] - prev[j]) * k for j in range(3))
            prev = tgt
        return prev

    def frame(T):
        im = Image.new("RGB", (W, H), BASE); d = ImageDraw.Draw(im)
        for k in range(-H, W, 90): d.line((k, 0, k + H, H), fill=(52, 36, 108), width=20)
        pop(d, 150, "TIER LIST DA", dela, 90, CREAM, T, 0.0)
        pop(d, 260, "TEMPORADA", dela, 110, YEL, T, 0.15)
        for k, (lab, c) in enumerate(tiers):
            a = ease((T - 0.3 - k * 0.2) / 0.3)
            if a <= 0: continue
            y = TY0 + k * TH; xoff = (1 - a) * -W
            d.rectangle((60 + xoff, y, 1000 + xoff, y + TH - 14), fill=(28, 18, 66), outline=INK, width=6)
            d.rectangle((60 + xoff, y, 230 + xoff, y + TH - 14), fill=c, outline=INK, width=6)
            ctext(d, y + 30, lab, dela(110), INK, None, cx=145 + xoff)
            for j in range(slots[k]):
                x = 260 + j * 150 + xoff
                d.rounded_rectangle((x, y + 24, x + 130, y + TH - 38), 16, fill=(70, 58, 120), outline=INK, width=5)
                ctext(d, y + 50, "?", dela(70), (140, 125, 190), None, cx=x + 65)
        p = pos(T)
        if p:
            if 2.2 <= T < 4.4:
                s = 1 + 0.06 * math.sin(T * 14)
                ctext(d, 1290, "ONDE ELE FICA?", bang(86 * s), CREAM)
            card(d, p[0], p[1], p[2])
        if T >= land:
            k = 1 - ease((T - land) / 0.4)
            if k > 0:
                stamp = dela(min(140 * (1 + 0.4 * k), fit("S TIER!", dela, 200, 960).size))
                ctext(d, 520 - 70 * k, "S TIER!", stamp, YEL, INK, 10)
            elif T < 7.2:
                ctext(d, 520, "S TIER!", dela(140), YEL, INK, 10)
        if T >= 6.6:
            pop(d, 1300, "MAS SERÁ QUE MERECE?", bang, 84, CREAM, T, 6.6, 920)
        if T >= 7.4:
            a = ease((T - 7.4) / 0.3); y = 1440 + (1 - a) * 80
            d.rounded_rectangle((110, y + 10, 930, y + 150), 30, fill=INK)
            d.rounded_rectangle((110, y, 930, y + 140), 30, fill=ORA, outline=INK, width=8)
            ctext(d, y + 30, "COMENTA: S, A OU B?", fit("COMENTA: S, A OU B?", bang, 76, 760), INK, None)
        if T >= 8.2:
            ctext(d, 1620, "OVERGEARED • NOVO NA CRUNCHYROLL", fit("OVERGEARED • NOVO NA CRUNCHYROLL", bang, 52, 900), BLUE)
        for b in bursts: b.draw(d, T)
        d.text((60, 1760), "@nakama.zap", font=dela(40), fill=CREAM)
        return im, hits
    return frame

frame = {"A": scene_A, "B": scene_B, "C": scene_C}[OPT]()
wav = OUT.replace(".mp4", ".wav"); finish_audio(wav)
cmd = ["ffmpeg", "-y", "-loglevel", "error", "-f", "rawvideo", "-pix_fmt", "rgb24", "-s", f"{W}x{H}", "-r", str(FPS), "-i", "-",
       "-i", wav, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-preset", "veryfast", "-crf", "20",
       "-c:a", "aac", "-b:a", "192k", "-movflags", "+faststart", "-shortest", OUT]
p = subprocess.Popen(cmd, stdin=subprocess.PIPE)
snaps = [1.0, 3.9, 5.9, 8.8]; prev = {}
for fi in range(int(DUR * FPS)):
    T = fi / FPS
    im, hits = frame(T)
    dx, dy = shake_xy(T, hits)
    if dx or dy: im = ImageChops.offset(im, dx, dy)
    fa = flash_amt(T, hits)
    if fa > 0: im = Image.blend(im, Image.new("RGB", (W, H), (255, 250, 235)), 0.55 * fa)
    for k, s in enumerate(snaps):
        if k not in prev and T >= s: prev[k] = im.copy()
    p.stdin.write(im.tobytes())
p.stdin.close(); p.wait()
sheet = Image.new("RGB", (4 * 405, 720))
for k in range(4): sheet.paste(prev[k].resize((405, 720)), (k * 405, 0))
sheet.save(OUT.replace(".mp4", "_sheet.png"))
print("ok", OUT)
