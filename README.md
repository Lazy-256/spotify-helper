# spotify-helper

CLI на Go для управления Spotify через Web API: состояние, воспроизведение,
устройства Connect, очередь, поиск. Предназначен для плагина-виджета
Noctalia v5, но работает и сам по себе — весь вывод в JSON.

> Статус: каркас проекта. Команд пока нет, см. `openspec/` и `AGENTS.md`.

## Требования

- Go (версия — в `go.mod`)
- Spotify Premium
- Приложение в [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)

## Подготовка: свой client_id

1. В дашборде нажми **Create an App**.
2. Redirect URI: `http://127.0.0.1:8888/callback`. Отметь **Web API**.
3. Скопируй **Client ID** (secret не нужен) в
   `~/.config/spotify-helper/config.toml`:

   ```toml
   client_id = "…"
   ```

4. Один раз авторизуйся (команда появится в первом изменении):

   ```bash
   spotify-helper login
   ```

## Сборка

```bash
go build -o bin/spotify-helper ./cmd/spotify-helper
```

## Разработка

Спеки и планы — в `openspec/` (на английском). Соглашения и принятые решения —
в [`AGENTS.md`](AGENTS.md).
