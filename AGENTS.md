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
| `$XDG_CONFIG_HOME/spotify-helper/config.toml` | `client_id`, redirect URI |
| `$XDG_CONFIG_HOME/spotify-helper/token.json` | access/refresh токены, права `0600` |
| `$XDG_CACHE_HOME/spotify-helper/` | кэш обложек и пр. |

Токены и `client_id` пользователя не коммитить (см. `.gitignore`).

## Структура (планируемая)

```
cmd/spotify-helper/   точка входа, разбор подкоманд
internal/auth/        PKCE, локальный callback-сервер, хранение и обновление токена
internal/api/         клиент Web API (player, search, queue, devices), лимиты и 429
internal/output/      JSON-контракт: типы ответов и ошибок
openspec/             спеки и изменения (на английском)
```

## Работа через OpenSpec

Изменения поведения идут через OpenSpec: `/opsx:propose` → `/opsx:apply` →
`/opsx:archive`. Начальное изменение — `bootstrap-helper`. Мелкие правки без
смены поведения — обычными коммитами.

## Команды

```bash
go build -o bin/spotify-helper ./cmd/spotify-helper
go test ./...
go vet ./...
```

## Открытые вопросы

- Значение `client_id` (создаёт пользователь в дашборде).
- Нужен ли режим демона/кэша состояния, или плагин сам опрашивает `state`.
