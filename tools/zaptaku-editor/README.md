# Zaptaku Editor

Aplica os templates do @zaptaku (Tela de RPG / Forja) em todos os vídeos de uma pasta.

- Retoma de onde parou: pula os vídeos já prontos em `zaptaku_editados` (por qualidade) e apaga sobras `.part.mp4`.
- GPU (NVENC / Quick Sync / AMF) e CPU (x264) trabalhando juntas; se a GPU falhar num vídeo, refaz pela CPU.
- Trava de temperatura: pausa tudo com GPU ≥ 83°C ou CPU ≥ 90°C e retoma abaixo de 75°C / 80°C.
- Log em `zaptaku_editados/zaptaku_log.txt`.

Ajustes opcionais (variáveis de ambiente): `ZAPTAKU_GPU_JOBS`, `ZAPTAKU_CPU_JOBS`, `ZAPTAKU_GPU_MAX`, `ZAPTAKU_GPU_RETOMA`, `ZAPTAKU_CPU_MAX`, `ZAPTAKU_CPU_RETOMA`.

Build do .exe (Windows):
1. Copie `templates/template_hud_loop.mp4` e `templates/template_forja_loop.mp4` para `assets/loop_hud.mp4` e `assets/loop_forja.mp4`.
2. Baixe um ffmpeg 6 para Windows (ex.: release b6.0 do eugeneware/ffmpeg-static), comprima com `xz -9e` em `assets/ffmpeg.exe.xz` e grave o tamanho original em bytes em `assets/ffmpeg.size`.
3. `GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o ZaptakuEditor.exe .`
