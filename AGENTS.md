# spotify-helper

CLI-хелпер на Go для управления Spotify через Web API. Нужен виджету
**Noctalia v5** (плагин на Luau, живёт отдельно в `~/dev/lua/`): плагин
вызывает хелпер через `noctalia.runAsync(...)` и разбирает JSON из stdout.
Хелпер — единственное место, где есть сеть, OAuth и знание о Spotify; плагин
только показывает состояние и шлёт команды.

Цель — всё, что нужно виджету: состояние воспроизведения, play/pause/next/prev,
громкость, seek, shuffle/repeat, **устройства Connect и перенос воспроизведения**,
**очередь (чтение и добавление)**, **поиск** (треки, альбомы, артисты, плейлисты),
запуск трека/альбома/плейлиста/артиста.

Этот файл — обзор и указатель. Держи его и `README.md` актуальными в том же
коммите, что и изменения поведения.

**Язык.** Документация спеков (OpenSpec: `proposal.md`, `design.md`, `tasks.md`,
`specs/**` и т. п. в `openspec/`) пишется на АНГЛИЙСКОМ. Вся остальная
документация проекта, включая этот файл и `README.md`, — на РУССКОМ. Диалог с
пользователем при работе над проектом ВСЕГДА ВЕДЁТСЯ НА РУССКОМ, независимо от
языка редактируемого документа. Комментарии в коде — на английском.

---

## Решения (принятые)

- **Только Web API**, свой OAuth **PKCE** (без client secret). Не зависим от
  `spotify_player` и не читаем его токен. Причина: MPRIS и CLI `spotify_player`
  не видят и не двигают Connect-устройства (у пользователя воспроизведение
  идёт на Connect-колонке), а в CLI `spotify_player` нет добавления в очередь
  и каждая команда без демона занимает 24–60 с.
- Нужен **Spotify Premium** (для управления воспроизведением и для владельца
  приложения в Development Mode).
- Свой `client_id` из Spotify Developer Dashboard; redirect URI
  `http://127.0.0.1:8888/callback` (именно `127.0.0.1`, не `localhost`; порт
  фиксированный, должен совпадать посимвольно).
- Scopes: `user-read-playback-state user-modify-playback-state
  user-read-currently-playing playlist-read-private user-library-read`.
- Только стандартная библиотека Go, пока нет веской причины тащить зависимость.
- Вывод — JSON в stdout; ошибки — JSON в stderr и ненулевой код выхода.
  Формат — контракт с плагином (см. спеки в `openspec/specs/`).

## Ограничения Spotify (Development Mode, с февраля 2026)

Владелец приложения обязан иметь Premium; не более 5 пользователей; поиск
отдаёт до 10 результатов; часть каталожных эндпоинтов убрана; HTTP 429 несёт
`Retry-After`, который надо уважать (без автоповторов). Подробности и источники —
в документации Spotify «February 2026 Web API Dev Mode Changes».

## Файлы на диске

| Путь | Назначение |
|------|------------|
| `$XDG_CONFIG_HOME/spotify-helper/config.toml` | `client_id`, `redirect_uri` |
| `$XDG_CONFIG_HOME/spotify-helper/token.json` | access/refresh токены и `client_id`, права `0600`, каталог `0700` |
| `$XDG_CONFIG_HOME/spotify-helper/token.lock` | `flock`, чтобы параллельные процессы не обновляли токен дважды |
| `$XDG_CACHE_HOME/spotify-helper/rate-limit.json` | пауза после HTTP 429 |
| `$XDG_CACHE_HOME/spotify-helper/covers/` | кэш обложек (2 МиБ на файл, 30 дней, 100 МиБ) |

Токены и `client_id` пользователя не коммитить (см. `.gitignore`).

## Структура

```
cmd/spotify-helper/    main: только cli.Run
internal/cli/          подкоманды, Env (потоки, окружение, часы, HTTP, браузер), разбор аргументов
internal/config/       XDG-пути, config.toml (подмножество TOML), client_id
internal/auth/         PKCE, callback-сервер, token endpoint, хранилище токена, обновление под flock
internal/api/          клиент Web API, маппинг ошибок, wire → типы вывода, player/search/queue/library
internal/covers/       кэш обложек и пост-обработка --covers
internal/doctor/       офлайн-отчёт
internal/ratelimit/    кулдаун после 429 на диске
internal/fileutil/     атомарная запись, flock
internal/spotifyuri/   разбор spotify:… и ссылок open.spotify.com
internal/output/       контракт: типы вывода, Error, коды выхода, JSON-писатели
openspec/              спеки и изменения (на английском)
```

Правила кода:
- Тестируемость через `cli.Env`: все обращения наружу идут через него; тесты
  подменяют Spotify на `httptest`, каталоги — на `t.TempDir()` через XDG-переменные.
- Ошибки наружу — только `*output.Error`; печатает их один `cli.Run`.
- Типы вывода — это контракт с плагином (`internal/output/model.go`); поля wire
  Spotify в них не просачиваются, см. `internal/api/wire.go`.
- Запросы за картинками идут отдельным `Env.CoverHTTP`, без токена.

## Работа через OpenSpec

Изменения поведения идут через OpenSpec: `/opsx:propose` → `/opsx:apply` →
`/opsx:archive`. Начальное изменение — `bootstrap-helper` (заархивировано 2026-10-15,
основные спеки — в `openspec/specs/`). Мелкие правки без смены поведения — обычными
коммитами.

## Команды

```bash
go build -o bin/spotify-helper ./cmd/spotify-helper
go test -race ./...
go vet ./...
gofmt -l .
```

## Открытые вопросы

- Режим демона не нужен: вызов занимает 100–500 мс (измерено на реальном аккаунте).
