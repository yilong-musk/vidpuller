# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português](README.pt.md) | Русский

Скачивает видео Douyin, TikTok, Bilibili и YouTube из платформенно-нейтральной JSON-ленты — самодостаточный один статический бинарник, только стандартная библиотека (в `go.mod` нет ни одного `require`). Извлечение YouTube встроено; потолок по умолчанию 1080p, `--max-height` поднимает его до 1440p или 2160p.

## Поддерживаемые площадки

| Площадка&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Содержимое | Один URL | Лента |
|---|---|---|---|
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Видео автора | Нет (нужна подпись браузера) | Да |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Видео | Да (`get`) | Да |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Видео и части | Да (`get`) | Да (DASH + ffmpeg) |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Видео | Да (`get`) | Да (встроенный + ffmpeg) |

## Возможности

- **По шагам**: книга `.state.json` с ключом `platform:id` пропускает уже скачанное; `--force` скачивает заново.
- **Без водяного знака**: берутся URL воспроизведения от площадки или текущей сессии; отдельного снятия знака нет.
- **Один формат входа**: ленту JSON может собрать любой скрипт, сервис или написанный вручную файл; SDK провайдера и конкретный upstream не нужны.
- **Метаданные без скачивания**: `vidpuller info <url>` печатает нейтральный JSON (название, автор, ID, длительность, размеры, дата публикации, обложка, части) для Bilibili, YouTube и TikTok.
- **Один бинарник**: статическая линковка (`CGO_ENABLED=0`), без сторонних зависимостей Go.
- **Прогресс для машины**: `--progress` пишет в stderr строки `PROGRESS <0-100>`, которые разбирает вызывающий.

## Формат входа

Верхний уровень — `{ "items": [...] }`, голый массив `[...]` или объект, где `data` — псевдоним `items`. Полный пример:

```json
{
  "platform": "douyin",
  "author": "creator",
  "items": [
    {
      "video_id": "1234567890123456789",
      "title": "Example title",
      "url": "https://example.com/video/123",
      "play_url": "https://example.com/video.mp4",
      "cover_url": "https://example.com/cover.jpg",
      "published_at": "2026-01-02T03:04:05Z",
      "author": "creator"
    }
  ]
}
```

| Поле | Обязательно | Описание |
|---|---|---|
| `items` / `data` | Да | Массив элементов; верхний уровень тоже может быть голым массивом |
| `platform` / `source` | Нет | `douyin`, `tiktok`, `bilibili` или `youtube`; можно задать и на каждом элементе |
| `video_id` | Да | Канонический ID видео; псевдонимы: `aweme_id` (Douyin/TikTok), `id` (BV ID Bilibili) |
| `title` / `desc` | Нет | Название; принимаются оба и входят в имя выходного файла |
| `url` | Нет | URL страницы видео; Bilibili принимает один BV ID или полный URL |
| `play_url` | Нет | Прямой URL медиа; обязателен для Douyin, заново разрешается для TikTok, не используется для Bilibili |
| `cover_url` / `images` | Нет | Обложка; если нет `cover_url`, берётся `images[0]` |
| `published_at` / `create_time` | Нет | Время публикации; строка RFC 3339 или секунды Unix |
| `author` | Нет | Имя автора, для каталога вывода и показа |

Порядок определения: `--platform` > `platform` элемента > `platform`/`source` верхнего уровня > BV ID или URL страницы Bilibili > URL YouTube > URL ленты > хосты URL медиа. Если ни одно правило не подошло, команда завершается ошибкой, а не угадывает площадку. Смешанные ленты поддерживаются, когда у каждого элемента есть `platform`.

## Установка

Статические бинарники для Linux, macOS и Windows (amd64/arm64) приложены к каждому [релизу](https://github.com/yilong-musk/vidpuller/releases). Скачайте архив своей платформы, сверьте его с `checksums.txt` и положите `vidpuller` в `PATH`.

На macOS или Linux с [Homebrew](https://brew.sh):

```bash
brew install --cask yilong-musk/tap/vidpuller
```

С цепочкой Go (1.27+):

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## Требования

- Go 1.27+ для сборки.
- `ffmpeg` в `PATH` для загрузок Bilibili и YouTube (аудио и видео DASH склеиваются).
- Необязательные cookie для более высокого качества: TikTok анонимно может отдавать 720p, а с вошедшей сессией — 1080p; Bilibili открывает 1080p и выше при `SESSDATA`. Для YouTube встроенный экстрактор использует `--cookies`.

## Быстрый старт

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + тесты + проверка языка
make install      # ставит в ~/.local/bin/vidpuller
vidpuller version
```

## Использование

```bash
# Локальная лента
vidpuller download ./timeline.json -n 5

# Удалённая лента (площадка определяется из JSON или URL)
vidpuller download 'https://example.com/timeline.json' -n 5

# Явная площадка, если в ленте нет подсказки
vidpuller download ./timeline.json --platform douyin -n 5

# Посмотреть до скачивания
vidpuller download '<timeline-url>' -n 10 --dry-run

# Только заданные элементы
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# Список элементов
vidpuller list '<timeline-url>' -n 20

# Только метаданные (JSON, без скачивания)
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# Одно видео TikTok (короткие ссылки vt.tiktok.com поддерживаются)
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# Bilibili с cookie
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# Одно видео Bilibili (BV ID или полный URL; ?p=N выбирает часть)
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# Одно видео YouTube (встроенный экстрактор, до 1080p)
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## Параметры

| Параметр | Описание |
|---|---|
| `-o, --out DIR` | Корень вывода, по умолчанию `./downloads` |
| `-n, --limit N` | Не больше N элементов (уже скачанные не считаются; 0 = без предела) |
| `--since YYYY-MM-DD` | Только элементы, опубликованные в этот день или позже |
| `--ids A,B` | Только указанные ID видео (`video_id` / `aweme_id`) |
| `--dry-run` | Печатает, что было бы скачано |
| `--force` | Скачивает заново уже записанные элементы |
| `--interval DUR` | Пауза между элементами, по умолчанию `3s` |
| `--platform douyin\|tiktok\|bilibili\|youtube` | Перекрывает определение площадки |
| `--max-height N` | Максимальная высота YouTube (по умолчанию 1080; например 1440, 2160) |
| `--cookies FILE` | Cookie входа (Netscape / JSON / `k=v; k=v`) |
| `--progress` | Пишет строки `PROGRESS <0-100>` в stderr |

## Вывод

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            пошаговая книга (ключ = platform:id)
```

Файл `.json` — исходный элемент ленты; `play_url` может протухнуть и хранится только для справки.

## Заметки о реализации

- **Douyin**: `play_url` скачивается напрямую с `Referer: https://www.douyin.com/`.
- **TikTok**: URL CDN привязаны к сессии, которая их подписала (`ttwid`/`msToken`/`tt_chain_token`), поэтому пакетная загрузка заново разрешает URL каждого видео текущей сессией; чужая банка cookie на upstream-URL даёт 403.
- WAF TikTok — это доказательство работы SHA-256, которое решает стандартная библиотека; браузер не нужен.
- **Выбор качества**: `UrlKey` каждого варианта (например `…_bytevc1_1080p_1037801`) даёт настоящие кодек и разрешение; побеждает большее разрешение, битрейт — при равенстве. Скачанное качество печатается (например `1080p bytevc1 722kbps`).
- **Bilibili**: `view` даёт cid, `playurl` — потоки DASH, `ffmpeg -c copy` их склеивает. Для URL Akamai сначала пробуется зеркальный хост, исходный CDN — запасной путь.
- **YouTube**: встроенный экстрактор InnerTube запрашивает ответ плеера клиентом VISIONOS, который отдаёт прямые URL адаптивных потоков без шифра подписи. Берётся самое высокое видео до `--max-height` (сначала AVC/MP4), к нему — лучшее аудио MP4, склейка через `ffmpeg -c copy`.

## Ограничения

- **Одиночные URL видео Douyin не поддерживаются**: конечная точка подробностей требует подпись браузера (`a_bogus`), чистый HTTP не получает ни поток без знака, ни метаданные. Для одного ролика используйте `download --ids <video_id>` или прочитайте элемент из JSON ленты.
- У картиночных постов TikTok нет `play_url`, они пропускаются (обложку всё ещё можно взять из ленты).
- **YouTube** по умолчанию 1080p; для выше передайте `--max-height 1440` или `--max-height 2160` (файлы могут быть на несколько сотен МБ). Видео с возрастным ограничением, DRM или региональной блокировкой могут требовать cookie или не скачиваться. Плейлисты и ленты каналов не разворачиваются; передавайте URL одного видео.
- Закрытое или регионально ограниченное содержимое площадка сообщает как ошибку; инструмент не видит того, чего нет в данных ленты.
- Сбор ленты, подпись и состояние входа — обязанность вызывающего; этот инструмент только потребляет JSON.

## Документация

Английский текст — единственный источник истины; эта страница — его русский перевод.

- **[English](README.md)**: английский оригинал, единственный источник истины.
- **[AGENTS.md](AGENTS.md)**: ограничения репозитория, включая правило «только английский» и политику слияния.

## License

[MIT](LICENSE)
