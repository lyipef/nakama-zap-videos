# Zaptaku Editor

Aplica os templates do @zaptaku (Tela de RPG / Forja / alternados) em todos os vídeos de uma pasta.
Abre uma janela própria (Edge/Chrome em modo app) com a interface.

- GPU (NVENC/QSV/AMF) + CPU juntas, ffmpeg 6.0 embutido
- Retoma de onde parou (pula vídeos já editados) e trava de temperatura com pausa/retomada automática
- Painel de temperatura: medidores GPU/CPU, uso, memória, consumo, ventoinha e gráfico de 5 min
- Opções: 4K/1080p, redução de ruído, nitidez, cor, volume, normalizar áudio, duração máxima, prioridade baixa, refazer, ação ao terminar
- Build: `GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui"`
