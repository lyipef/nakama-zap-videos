# Zaptaku Editor

Aplica os templates do @zaptaku (Tela de RPG / Forja) em todos os vídeos de uma pasta.

Build do .exe (Windows):
1. Copie `templates/template_hud_loop.mp4` e `templates/template_forja_loop.mp4` para `assets/loop_hud.mp4` e `assets/loop_forja.mp4`.
2. Coloque um `ffmpeg.exe` em `assets/ffmpeg.exe` (ex.: `npm pack @ffmpeg-installer/win32-x64`).
3. `GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o ZaptakuEditor.exe .`
