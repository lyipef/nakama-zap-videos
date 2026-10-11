"""Efeitos compartilhados: partículas, marreta, tremida/flash e efeitos sonoros (estilos HUD e FORJA)."""
import math, random
import numpy as np
from PIL import ImageDraw

INK = (17, 17, 17)

class Burst:
    def __init__(s, t0, x, y, n, cols, speed=900, life=1.0, grav=1400, size=10, seed=1, up=False):
        r = random.Random(seed); s.t0 = t0; s.p = []; s.grav = grav
        for _ in range(n):
            a = r.uniform(-math.pi, 0) if up else r.uniform(0, 2 * math.pi)
            v = r.uniform(0.3, 1) * speed
            s.p.append((x, y, math.cos(a) * v, math.sin(a) * v, r.uniform(0.5, 1) * life, r.choice(cols), r.uniform(0.5, 1) * size))
    def draw(s, d, T):
        t = T - s.t0
        if t < 0: return
        for x, y, vx, vy, life, c, sz in s.p:
            if t > life: continue
            k = 1 - t / life
            px = x + vx * t; py = y + vy * t + 0.5 * s.grav * t * t
            r_ = sz * (0.4 + 0.6 * k)
            d.ellipse((px - r_, py - r_, px + r_, py + r_), fill=c)

def hammer(d, px, py, ang, s=1.0):
    L = 330 * s
    hx = px - math.cos(ang) * L; hy = py - math.sin(ang) * L
    d.line((px, py, hx, hy), fill=(120, 72, 40), width=int(34 * s))
    nx, ny = -math.sin(ang), math.cos(ang); ux, uy = math.cos(ang), math.sin(ang)
    hw, hl = 70 * s, 120 * s
    pts = [(hx + nx * hl - ux * hw, hy + ny * hl - uy * hw), (hx + nx * hl + ux * hw, hy + ny * hl + uy * hw),
           (hx - nx * hl + ux * hw, hy - ny * hl + uy * hw), (hx - nx * hl - ux * hw, hy - ny * hl - uy * hw)]
    d.polygon(pts, fill=(140, 140, 165), outline=INK, width=max(3, int(8 * s)))

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

# ---------- áudio ----------
SR = 44100
_rng = np.random.default_rng(5)
def tt(d): return np.arange(int(SR * d)) / SR
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
    t = tt(d); nz = _rng.standard_normal(len(t))
    X = np.fft.rfft(nz); fr = np.fft.rfftfreq(len(nz), 1 / SR)
    nz = np.fft.irfft(X / (1 + (fr / 2500) ** 2), len(nz))
    return nz / np.max(np.abs(nz)) * np.sin(np.pi * t / d) ** 2 * g
def impact(g=0.9):
    t = tt(0.9); f = 30 + 90 * np.exp(-t * 12)
    return (np.sin(2*np.pi*np.cumsum(f)/SR) * np.exp(-t * 4) + _rng.standard_normal(len(t)) * np.exp(-t * 25) * 0.4) * g
def clang(g=0.45):
    t = tt(1.3); s = np.zeros(len(t))
    for f, a in [(820, 1), (1290, 0.7), (1870, 0.5), (2710, 0.35), (3550, 0.2)]:
        s += a * np.sin(2*np.pi*f*t) * np.exp(-t * (3 + f / 900))
    s += _rng.standard_normal(len(t)) * np.exp(-t * 60) * 0.8
    return s * g

def mix_sfx(music_wav, out_wav, events, music_gain=0.75):
    import wave
    with wave.open(music_wav) as w:
        sr = w.getframerate(); ch = w.getnchannels()
        m = np.frombuffer(w.readframes(w.getnframes()), dtype="<i2").astype(float).reshape(-1, ch) / 32767
    m = m.mean(1) * music_gain
    for t0, sig in events:
        i = int(t0 * SR)
        if i >= len(m): continue
        n = min(len(sig), len(m) - i); m[i:i + n] += sig[:n]
    peak = np.max(np.abs(m))
    if peak > 0.95: m = m / peak * 0.95
    st = np.stack([m, m], 1)
    with wave.open(out_wav, "wb") as w:
        w.setnchannels(2); w.setsampwidth(2); w.setframerate(SR)
        w.writeframes((st * 32767).astype("<i2").tobytes())
