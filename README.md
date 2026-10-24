# spotify-helper

CLI на Go для управления Spotify через Web API: состояние, воспроизведение,
устройства Connect, очередь, поиск, библиотека. Предназначен для плагина-виджета
Noctalia v5, но работает и сам по себе — весь вывод в JSON.

Только стандартная библиотека Go, только Linux (`flock`, `xdg-open`).

> Статус: все команды реализованы, покрыты тестами с фейковым Spotify и один раз
> проверены на реальном аккаунте (раздел «Проверка на реальном аккаунте»).

## Требования

- Go (версия — в `go.mod`)
- Spotify Premium
- Приложение в [Spotify Developer Dashboard](https://developer.spotify.com/dashboard)

## Подготовка: свой client_id

1. В дашборде нажми **Create an App**.
2. Redirect URI: `http://127.0.0.1:8888/callback` (именно `127.0.0.1`, не
   `localhost`). Отметь **Web API**.
3. Скопируй **Client ID** (secret не нужен) в
   `~/.config/spotify-helper/config.toml`:

   ```toml
   client_id = "…"
   # redirect_uri = "http://127.0.0.1:8888/callback"   # по умолчанию
   ```

   Или передай через переменную `SPOTIFY_HELPER_CLIENT_ID` (она важнее файла).
4. Один раз авторизуйся:

   ```bash
   spotify-helper login
   ```

   Адрес авторизации печатается в stderr и открывается через `xdg-open`.
   Токен сохраняется в `~/.config/spotify-helper/token.json` (права `0600`) и
   обновляется автоматически. Токен привязан к `client_id`: смена `client_id`
   требует нового `login`.

   Запрашиваются права `user-read-playback-state user-modify-playback-state
   user-read-currently-playing playlist-read-private playlist-read-collaborative
   user-library-read user-library-modify`. `user-library-modify` нужно для
   `save`/`unsave`; если токен выдан до его появления, `doctor` покажет
   `scopes_missing`, а `save`/`unsave` ответят `scopes_missing` — выполни
   `spotify-helper login` ещё раз. Чтение (`saved`, `playlists`, `albums`) при этом
   работает. `playlist-read-collaborative` нужно только для треков **совместных**
   плейлистов в `playlist`: ни одна команда его не требует, `doctor` его не проверяет,
   а у старого токена такой плейлист просто придёт без треков (`tracks_available:false`,
   `playlist.collaborative:true`) — тогда нужен ещё один `login`.

В конфиге поддерживается только подмножество TOML: пустые строки, комментарии
`#` и пары `ключ = "строка в двойных кавычках"`. Всё остальное — ошибка
`not_configured`.

## Сборка и тесты

```bash
go build -o bin/spotify-helper ./cmd/spotify-helper
go test ./...
go vet ./...
```

### Установка в `~/.local/bin`

Плагину нужен бинарник в `PATH`. Версия берётся из коммита:

```bash
go build -ldflags "-X github.com/Lazy-256/spotify-helper/internal/cli.Version=0.1.0+$(git rev-parse --short HEAD)" -o ~/.local/bin/spotify-helper ./cmd/spotify-helper
```

## Команды

Флаги можно ставить где угодно: `search daft punk --type artist`. Отрицательные
числа — это значения: `volume -10`, `seek -5000`. `--` заканчивает разбор флагов.
`--device ID` у команд управления направляет запрос на конкретное устройство
(тогда команда не падает с `no_active_device`, если активного нет).

| Команда | Что делает |
|---|---|
| `login` / `logout` | авторизация (PKCE) / удалить токен |
| `state [--covers]` | что играет и где |
| `play`, `pause`, `toggle`, `next`, `prev` | транспорт |
| `volume <0-100\|+N\|-N>` | громкость, относительное значение ограничивается 0–100 |
| `seek <мс\|+мс\|-мс>` | позиция, ограничивается 0…(длительность − 1 с), трек не завершается |
| `shuffle <on\|off\|toggle>`, `repeat <off\|context\|track\|cycle>` | режимы |
| `play-uri <uri\|ссылка> [--context <альбом\|плейлист>]` | запустить трек, эпизод, альбом, плейлист, артиста или подкаст; с `--context` — трек или эпизод **внутри** альбома или плейлиста («играть отсюда») |
| `search <запрос> [--type …] [--limit 1-10] [--covers]` | поиск |
| `queue [--covers]`, `queue-add <uri\|ссылка>` | очередь: читать и добавлять трек/эпизод |
| `devices`, `transfer <id\|имя> [--play]` | устройства Connect |
| `playlists`, `albums` `[--limit 1-50] [--offset N] [--covers]` | ваша библиотека |
| `album <uri\|ссылка>`, `playlist <uri\|ссылка>` `[--limit 1-50] [--offset N] [--covers]` | что внутри: треки альбома или плейлиста |
| `artist <uri\|ссылка> [--limit 1-10] [--offset N] [--covers]` | артист и его альбомы и синглы (по 10) |
| `save <uri\|ссылка>`, `unsave <uri\|ссылка>` | добавить трек/эпизод в библиотеку («Любимые треки») и убрать |
| `saved <uri\|ссылка>` | лежит ли трек/эпизод в библиотеке |
| `cover <https-url>` | скачать картинку в кэш и вернуть путь |
| `doctor` | офлайн-диагностика |
| `version` | версия |

`play-uri` принимает `spotify:album:ID` и ссылки `https://open.spotify.com/…`.
`play-uri spotify:track:T --context spotify:album:A` запускает альбом с трека `T`
(`context_uri` плюс `offset.uri`), и воспроизведение идёт дальше по альбому; контекстом
может быть только альбом или плейлист, а `T` — трек или эпизод.
Удалить или переставить элементы очереди нельзя: в Web API нет такой операции.

`album`, `playlist` и `artist` ничего не требуют от устройства и Premium. У них есть
границы Development Mode (проверено 2026-10-24, см. «Что оказалось не так»): у артиста
страница не больше 10 (больший `--limit` уменьшается до 10, по умолчанию 10),
у плейлиста чужого пользователя нет треков, у редакционных плейлистов Spotify
(`37i9dQZ…`) нет даже шапки (`api_error`, HTTP 404), топ-треков артиста нет совсем.

### Формат вывода

Успех — один JSON-объект и перевод строки в stdout, код выхода 0. Команды без
данных печатают `{"ok":true}`. Новые поля могут добавляться; существующие не
переименовываются и не меняют тип.

`state`:

```json
{
  "active": true, "is_playing": true,
  "progress_ms": 250343, "fetched_at_ms": 1791903029671,
  "shuffle": true, "repeat": "off",
  "device": {"id": "…", "name": "Living Room", "type": "Speaker",
             "is_active": true, "is_restricted": false, "volume_percent": 93},
  "context": {"type": "playlist", "uri": "spotify:playlist:…"},
  "item": {"type": "track", "uri": "spotify:track:…", "id": "…", "name": "…",
           "artists": [{"name": "…", "uri": "…"}],
           "album": {"name": "…", "uri": "…"},
           "duration_ms": 345186, "explicit": false,
           "cover_url": "https://i.scdn.co/image/…"}
}
```

Если ничего не играет: `active:false`, `is_playing:false`, `progress_ms:0`,
а `device`, `context` и `item` равны `null`. Для эпизода `artists` пуст, а
`album` описывает подкаст. Позиция сейчас = `progress_ms` + (текущее время −
`fetched_at_ms`), пока `is_playing`.

`search`: `{"query":"…", "tracks":[Item…], "albums":[…], "artists":[…], "playlists":[…]}`.
Ключ есть только у запрошенных типов (у запрошенных без совпадений — `[]`).
Альбом: `uri,id,name,artists,release_date,total_tracks,cover_url`. Артист:
`uri,id,name,cover_url`. Плейлист: `uri,id,name,owner_name,cover_url`.
Поле `uri` каждого результата подходит для `play-uri`.

`queue`: `{"now_playing": Item|null, "queue": [Item…]}`.
`devices`: `{"devices":[{id,name,type,is_active,is_restricted,volume_percent}…]}`
(`id` и `volume_percent` могут быть `null`).
`playlists` / `albums`: `{"total":N,"offset":N,"limit":N,"playlists":[…]}` /
`{…,"albums":[…]}`.

`album`: `{"album":Album,"tracks":[Item…],"total":N,"offset":N,"limit":N}`; у треков `album`
— этот альбом, а `cover_url` — его обложка.
`playlist`: `{"playlist":{uri,id,name,owner_name,cover_url,"collaborative":bool},
"tracks_available":bool,"tracks":[Item…],"total":N,"offset":N,"limit":N}`. Треки — треки
и эпизоды (пустые и локальные записи пропущены). Если Spotify не отдал элементы плейлиста
(чужой плейлист), будет `tracks_available:false`, `tracks:[]`, `total:0` и код выхода 0.
`artist`: `{"artist":{uri,id,name,cover_url},"albums":[Album…],"total":N,"offset":N,"limit":N}`
— только альбомы и синглы, без сборников и «появляется в»; `limit` — применённое значение
(не больше 10). С `--covers` обложки получают шапка и все записи (у альбома это одна
загрузка на все треки, у плейлиста — до одной на разный альбом).

`volume` → `{"ok":true,"volume_percent":N}`, `seek` → `{"ok":true,"position_ms":N}`,
`shuffle` → `{"ok":true,"shuffle":bool}`, `repeat` → `{"ok":true,"repeat":"…"}`,
`queue-add`, `save`, `unsave` → `{"ok":true,"uri":"…"}`, `saved` →
`{"uri":"…","saved":bool}`, `transfer` →
`{"ok":true,"device":{"id":"…","name":"…"}}`,
`cover` → `{"ok":true,"url":"…","path":"/abs/path.jpg","cached":bool}`.

### Обложки

`ui.image` в Noctalia грузит только локальные файлы. `--covers` добавляет к
каждому элементу `cover_path` — путь к файлу в
`~/.cache/spotify-helper/covers/` (или `null`, если картинки нет или она не
скачалась за 8 секунд; команда при этом не падает). Без флага поля `cover_path`
нет и ничего не скачивается. Кэш: до 2 МиБ на файл, 30 дней, 100 МиБ суммарно,
очистка не чаще раза в сутки. Запросы за картинками идут без токена Spotify.

### Ошибки

Ошибка — один JSON-объект в **последней строке** stderr, stdout пуст, код выхода
не 0. (Только `login` может напечатать перед ней адрес авторизации.)

```json
{"error":{"code":"no_active_device","message":"…"}}
```

| `error.code` | Код выхода | Когда |
|---|---|---|
| `internal`, `api_error` | 1 | внутренняя ошибка; неожиданный ответ Spotify (в сообщении HTTP-статус; например, 404 для редакционного плейлиста или несуществующего альбома) |
| `usage` | 2 | неверные аргументы; Spotify не вызывается |
| `not_configured`, `not_authenticated`, `scopes_missing` | 3 | нет `client_id`; нет токена / он чужой / отозван — нужен `login`; у токена нет нужного права (`save`/`unsave` без `user-library-modify`) — тоже `login` |
| `no_active_device`, `device_not_found`, `ambiguous_device` | 4 | нет активного устройства; `transfer` не нашёл или нашёл несколько |
| `rate_limited` | 5 | HTTP 429; поле `error.retry_after` — секунды ожидания |
| `premium_required` | 6 | нужен Premium |
| `network_error` | 7 | сеть или таймаут (10 с на запрос) |

Повторов нет. После 429 хелпер запоминает паузу в
`~/.cache/spotify-helper/rate-limit.json`, и пока она не прошла, любая
команда сразу отвечает `rate_limited` с оставшимися секундами, не обращаясь к
Spotify.

### Рекомендации для плагина

- Опрашивай `state` не чаще раза в 2–3 секунды, пока виджет виден; между
  опросами двигай прогресс по `progress_ms` и `fetched_at_ms`, а после своей
  команды обнови состояние сразу. Точный лимит Spotify неизвестен, это
  осторожная оценка.
- При `rate_limited` не опрашивай, пока не пройдёт `retry_after`.
- Чтобы разбудить неактивное устройство, делай `transfer <устройство>` без `--play`,
  жди `state.active` (опрос раз в секунду, до 5–8 секунд), затем `play`; `transfer --play`
  одним запросом на такой колонке ничего не запускал (см. «Проверка на реальном аккаунте»).
- Токены, `client_id` и коды авторизации никогда не попадают в вывод.

## Диагностика

`spotify-helper doctor` без сети показывает, настроен ли `client_id` (и откуда),
есть ли токен, сколько ему жить, какие выданы scopes, идёт ли кулдаун, и пути к
файлам. Всегда выходит с 0, а проблемы лежат в `problems`
(`not_configured`, `not_authenticated`, `scopes_missing`, `rate_limited`).

## Проверка на реальном аккаунте

Автотесты работают с фейковым Spotify, поэтому один раз всё прогнали на живом
аккаунте (Premium, режим разработки, 2026-10-15). Колонка Connect `Living Room`
(hifiberry, сервис `spotify-soloist`) и Web Player в Firefox.

- [x] `login` проходит, `doctor` → `ok:true`
- [x] `state` при играющем треке и при остановленном воспроизведении
- [x] `play`, `pause`, `toggle`, `next`, `prev`, `volume`, `seek`, `shuffle`, `repeat`
- [x] `devices`, `transfer` на колонку Connect
- [x] пробуждение неактивной колонки (2026-10-17): `transfer <устройство>` без `--play`, затем `play`;
  `transfer --play` и `play-uri --device` одним запросом (2026-10-15) не сработали
- [x] `search` (все четыре типа, `limit` 10)
- [x] `queue`, `queue-add`
- [x] `play-uri` для трека, альбома, плейлиста, артиста и ссылки `open.spotify.com`
- [x] `playlists`, `albums`
- [x] `album`, `playlist`, `artist` (2026-10-24, Development Mode): альбом (10 треков, `--limit 50`), свой
  плейлист (6 треков), плейлисты других пользователей (`tracks_available:false`), страница артиста с
  `--offset` (33 релиза); `--covers` — 0,2–1,5 с
- [x] `play-uri <трек> --context <альбом>` (2026-10-24, на колонке): третий трек альбома заиграл в
  контексте альбома, `queue` показала следующие треки альбома по порядку
- [x] `saved` → `save` → `saved` → `unsave` → `saved` на настоящем аккаунте после `login` с новым правом (ответы `false`, `ok`, `true`, `ok`, `false`); заодно проверить, что Development Mode пропускает `/me/library`
- [x] `state --covers`, `queue --covers`, `search --covers`, `playlists --covers`, `albums --covers`
- [x] время вызова: `state`, `devices`, `queue` — 100–160 мс; команды управления —
  100–500 мс; `search` — 0,5–1,2 с (для сравнения: `spotify_player` без демона 24–60 с)
- [x] ни токен, ни `client_id` не попали ни в один stdout/stderr (68 файлов вывода, проверка
  по точным значениям; единственное исключение по спеку — адрес авторизации в `login`)

На самой колонке проверены `transfer`, `volume`, `play-uri` для трека, `pause`,
`play`, `toggle`, `seek`, `shuffle`, `repeat`. Очередь, `play-uri` для альбома,
плейлиста и артиста, `next` и `prev` проверены на Web Player.

### Что оказалось не так, как предполагали

- **Устройство без активной сессии просыпается, но не одним запросом.** 2026-10-15
  `play-uri … --device` и `transfer --play` на такое устройство возвращали успех
  (HTTP 204), но ничего не запускали. 2026-10-17 на той же колонке сработала
  двухшаговая схема: `transfer <устройство>` **без** `--play` за ~3 с сделал её
  активной (`devices` → `is_active:true`, `state` → `active:true`, пауза, последний трек
  и позиция из облака), а следующий `play` возобновил воспроизведение (прогресс шёл в
  реальном темпе). Почему так — гипотеза: запрос «стань активным и играй» теряет вторую
  половину, потому что колонка ещё не готова. Более «холодное» состояние колонки
  (например, после перезапуска её сервиса) не проверялось, поэтому вызывающему нужен
  запасной путь: если `state` не стал активным за 5–8 секунд, показать подсказку
  «запустите воспроизведение в приложении Spotify» (`no_active_device`/пустой `state`
  остаются сигналом для неё). Плагин делает именно так (кнопка «Play on <устройство>»,
  проверено на колонке).
- **Spotify отказывает с 403 «Restriction violated»**, когда текущая сессия не
  допускает команду (например, `next` в конце списка, `seek` в зависшей сессии).
  Хелпер отдаёт это как `api_error` с HTTP-статусом в сообщении.
- **`seek` в самый конец завершает трек.** После такой перемотки на `Living Room` сессия
  зависла: команды принимались (204), но не выполнялись, а после перезапуска сервиса
  старое состояние восстановилось. Причинная связь не доказана, поэтому хелпер
  ограничивает позицию значением «длительность минус 1 секунда» и трек не завершает.
- **Каталог в Development Mode (2026-10-24, на настоящем аккаунте).** `GET /artists/{id}/albums`
  принимает `limit` не больше 10 (больше — HTTP 400 «Invalid limit»), поэтому у `artist` страница
  по 10 и листать нужно `--offset`. Треки плейлиста отдаются только для своих и совместных
  плейлистов: у чужого плейлиста ответ 200 только с шапкой (`tracks_available:false`), а у
  редакционных плейлистов Spotify (`37i9dQZ…`) на шапку тоже HTTP 404 — как для несуществующего
  плейлиста, отличить их нельзя. `GET /albums/{id}/tracks` (`limit` 50) и `/artists/{id}` работают.
- **Очередь**: Spotify отдаёт не более 20 ближайших элементов; `queue-add` ставит
  трек в начало очереди (его проигрывает следующий `next`).
- **Библиотека**: `save`/`unsave` идемпотентны (Spotify отвечает 200 и когда трек уже
  был или не был). Сразу после записи `saved` может на мгновение показать старое
  значение, поэтому вызывающему лучше доверять результату своей записи.
- **`search`** может вернуть меньше 10 плейлистов: `null`-записи отбрасываются.
- Один раз колонка сообщила громкость 93 сразу после того, как была выставлена 20;
  позже громкость держалась стабильно. Причина на стороне устройства, не выяснена.

## Разработка

Спеки и планы — в `openspec/` (на английском), принятые решения и структура —
в [`AGENTS.md`](AGENTS.md).
