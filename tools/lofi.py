import numpy as np, wave, subprocess, sys
seed = int(sys.argv[1]) if len(sys.argv) > 1 else 7
rng = np.random.default_rng(seed)
sr = 44100; bpm = 76; beat = 60 / bpm; bars = int(sys.argv[2]) if len(sys.argv) > 2 else 16
N = int(sr * beat * 4 * bars)
mix = np.zeros(N)

def add(sig, start):
    i = int(start * sr)
    if i >= N: return
    n = min(len(sig), N - i)
    mix[i:i + n] += sig[:n]

def hz(m): return 440 * 2 ** ((m - 69) / 12)

def ep(f, dur, vel=1.0):
    t = np.arange(int(sr * dur)) / sr
    env = np.exp(-t * 2.0) * (1 - np.exp(-t * 80))
    tone = np.sin(2*np.pi*f*t) + 0.35*np.sin(2*np.pi*2*f*t)*np.exp(-t*4) + 0.12*np.sin(2*np.pi*3*f*t)*np.exp(-t*7)
    trem = 1 + 0.12 * np.sin(2*np.pi*4.2*t)
    return tone * env * trem * vel * 0.12

def bass(f, dur):
    t = np.arange(int(sr * dur)) / sr
    env = np.exp(-t * 1.6) * (1 - np.exp(-t * 120))
    return (np.sin(2*np.pi*f*t) + 0.2*np.sin(2*np.pi*2*f*t)) * env * 0.32

def kick():
    t = np.arange(int(sr * 0.35)) / sr
    f = 45 + 110 * np.exp(-t * 28)
    return np.sin(2*np.pi*np.cumsum(f)/sr) * np.exp(-t * 11) * 0.7

def snare():
    t = np.arange(int(sr * 0.25)) / sr
    nz = np.diff(rng.standard_normal(len(t) + 1))
    return (nz * 0.35 * np.exp(-t * 22) + np.sin(2*np.pi*185*t) * 0.25 * np.exp(-t * 28))

def hat(vel=1.0):
    t = np.arange(int(sr * 0.06)) / sr
    nz = np.diff(rng.standard_normal(len(t) + 1), 2) if False else np.diff(rng.standard_normal(len(t) + 1))
    return nz * np.exp(-t * 70) * 0.16 * vel

chords = [([53,57,60,64], 41), ([52,55,59,62], 40), ([50,53,57,60], 38), ([48,52,55,59], 36)]
bar = beat * 4
for b in range(bars):
    notes, root = chords[b % 4]
    t0 = b * bar
    for i, m in enumerate(notes):
        add(ep(hz(m), 3.0, 1.0), t0 + i * 0.025)
        add(ep(hz(m), 1.6, 0.7), t0 + beat * 2.5 + i * 0.02)
    add(bass(hz(root), beat * 2.2), t0)
    add(bass(hz(root), beat * 1.4), t0 + beat * 2.5)
    add(kick(), t0); add(kick(), t0 + beat * 2.5)
    add(snare(), t0 + beat); add(snare(), t0 + beat * 3)
    for e in range(8):
        swing = 0.12 * beat if e % 2 else 0
        add(hat(1.0 if e % 2 == 0 else 0.6), t0 + e * beat / 2 + swing)

# suaviza (low-pass) via FFT
def lp(x, fc):
    X = np.fft.rfft(x); f = np.fft.rfftfreq(len(x), 1/sr)
    return np.fft.irfft(X / (1 + (f / fc) ** 4), len(x))
mix = lp(mix, 3800)

# reverb simples
ir_t = np.arange(int(sr * 1.2)) / sr
ir = rng.standard_normal(len(ir_t)) * np.exp(-ir_t * 4.5)
ir = lp(ir, 3000)
wet = np.fft.irfft(np.fft.rfft(mix, N + len(ir)) * np.fft.rfft(ir, N + len(ir)))[:N]
mix = mix + 0.06 * wet / (np.max(np.abs(wet)) + 1e-9) * np.max(np.abs(mix))

# chiado de vinil
crack = np.zeros(N)
idx = rng.integers(0, N, size=int(N / sr * 6))
crack[idx] = rng.uniform(-1, 1, len(idx)) * 0.12
hiss = lp(rng.standard_normal(N), 6000) * 0.004
mix = mix + crack + hiss

# fade in/out e normalização
fade = int(sr * 1.5)
mix[:fade] *= np.linspace(0, 1, fade); mix[-fade:] *= np.linspace(1, 0, fade)
mix = mix / np.max(np.abs(mix)) * 0.85
L = mix; R = np.concatenate([np.zeros(int(sr*0.008)), mix])[:N]
st = np.stack([L, R], 1)
pcm = (st * 32767).astype('<i2')
with wave.open('lofi_nakama.wav', 'wb') as w:
    w.setnchannels(2); w.setsampwidth(2); w.setframerate(sr); w.writeframes(pcm.tobytes())
print('dur', N / sr, 'rms', float(np.sqrt(np.mean(mix**2))), 'peak', float(np.max(np.abs(mix))))
