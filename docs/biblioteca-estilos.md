# Biblioteca de efeitos e estilos (vídeo como código)

## Pipeline atual (padrão, funciona na nuvem): Python + Pillow + ffmpeg
- tools/gen.py (render por quadro), tools/fx.py (partículas, marreta, tremida/flash, SFX sintéticos: blip, chime, whoosh, impact, clang), tools/lofi.py (trilha), tools/template.py.
- Estilos: hud (tela de RPG) e forja. Layouts (kind): list, ab, flag.

## Camada opcional: Remotion (React + código -> mp4)
- Usar só se `npm i remotion @remotion/cli` e o render (Chromium do Playwright) funcionarem em poucos minutos no ambiente; se falhar, usar o pipeline atual sem insistir e avisar no relatório.
- Cada cena = componente em remotion/src/ (Gancho, Item, Cta), parametrizado pelo roteiro .md/JSON; saída 1080x1920, 30 fps, mesmo áudio (trilha + SFX).
- Regra: variar estilo (cores, tipografia, transição) entre vídeos para evitar aspecto genérico/repetitivo.

## Repertório de efeitos (variar entre vídeos)
- Transições: slide, wipe diagonal, glitch, flash branco, tremida de câmera (shake_xy), zoom-punch.
- Tipografia: pop com overshoot, typewriter, palavra-chave em amarelo, contador "3/7", barra de progresso.
- Partículas: Burst (faíscas na forja, confete no hud).
- Quebra de padrão a cada ~2 itens: efeito diferente + SFX diferente, para segurar retenção.

## Áudio / SFX (obrigatório em vídeo B)
- Animação por código fica "seca": todo vídeo B leva SFX nas viradas (item novo = blip/whoosh/clang, melhor por último = chime agudo, CTA = impact + chime) por cima da trilha lofi, já feito em fx.mix_sfx.
- Só sons sintéticos próprios ou acervo royalty-free com licença verificada; nunca trecho de anime nem música com direitos.
- Locução (opcional, futura): só se o usuário aprovar uma voz (ElevenLabs).
