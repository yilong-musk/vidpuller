# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | Español | [Français](README.fr.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Português](README.pt.md) | [Русский](README.ru.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

Descarga vídeos de Douyin, TikTok, Bilibili y YouTube a partir de una cronología JSON neutra respecto a la plataforma: autónomo, un solo binario estático, solo biblioteca estándar (no hay ni un `require` en `go.mod`). La extracción de YouTube va integrada; el techo por defecto es 1080p y `--max-height` lo sube a 1440p o 2160p.

## Plataformas admitidas

| Plataforma&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Contenido | URL suelta | Cronología |
|---|---|---|---|
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Vídeos de un creador | No (hace falta una firma de navegador) | Sí |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Vídeos | Sí (`get`) | Sí |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Vídeos y partes | Sí (`get`) | Sí (DASH + ffmpeg) |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Vídeos | Sí (`get`) | Sí (nativo + ffmpeg) |

## Características

- **Incremental**: un libro `.state.json` con clave `platform:id` se salta lo ya descargado; `--force` lo vuelve a bajar.
- **Sin marca de agua**: usa las URL de reproducción que da la plataforma o que genera la sesión actual; no hay un paso de quitar la marca.
- **Un solo formato de entrada**: cualquier script, servicio o archivo escrito a mano puede producir el JSON de la cronología; no hace falta un SDK ni un origen concreto.
- **Metadatos sin descargar**: `vidpuller info <url>` imprime un objeto JSON neutro (título, autor, ID, duración, dimensiones, fecha de publicación, portada, partes) para Bilibili, YouTube y TikTok.
- **Un solo binario**: enlazado estáticamente (`CGO_ENABLED=0`), sin dependencias Go de terceros.
- **Progreso legible por máquina**: `--progress` escribe líneas `PROGRESS <0-100>` en stderr para que quien llama las analice.

## Formato de entrada

El nivel superior puede ser `{ "items": [...] }`, un array desnudo `[...]` o un objeto que use `data` como alias de `items`. Ejemplo completo:

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

| Campo | Obligatorio | Descripción |
|---|---|---|
| `items` / `data` | Sí | Array de elementos; el nivel superior también puede ser un array desnudo |
| `platform` / `source` | No | `douyin`, `tiktok`, `bilibili` o `youtube`; también se puede poner en cada elemento |
| `video_id` | Sí | ID canónico del vídeo; alias: `aweme_id` (Douyin/TikTok), `id` (BV ID de Bilibili) |
| `title` / `desc` | No | Título; se aceptan los dos y entran en el nombre del archivo de salida |
| `url` | No | URL de la página del vídeo; Bilibili acepta un BV ID solo o una URL completa |
| `play_url` | No | URL directa del medio; obligatoria en Douyin, se vuelve a resolver en TikTok, no se usa en Bilibili |
| `cover_url` / `images` | No | Imagen de portada; se usa `images[0]` si falta `cover_url` |
| `published_at` / `create_time` | No | Hora de publicación; cadena RFC 3339 o segundos Unix |
| `author` | No | Nombre del autor, para el directorio de salida y la presentación |

Orden de detección: `--platform` > `platform` del elemento > `platform`/`source` del nivel superior > BV ID o URL de página de Bilibili > URL de YouTube > URL de la cronología > hosts de la URL del medio. Si ninguna regla encaja, el comando falla en lugar de adivinar. Se admiten cronologías mixtas cuando cada elemento lleva `platform`.

## Instalación

Los binarios estáticos para Linux, macOS y Windows (amd64/arm64) van adjuntos a cada [release](https://github.com/yilong-musk/vidpuller/releases). Descargue el archivo de su plataforma, compruébelo con `checksums.txt` y ponga `vidpuller` en el `PATH`.

En macOS o Linux con [Homebrew](https://brew.sh):

```bash
brew install --cask yilong-musk/tap/vidpuller
```

Con una cadena de herramientas Go (1.27+):

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## Requisitos

- Go 1.27+ para compilar.
- `ffmpeg` en el `PATH` para las descargas de Bilibili y YouTube (se unen el audio y el vídeo DASH).
- Cookies opcionales para más calidad: TikTok puede servir 720p en anónimo y 1080p con una sesión iniciada; Bilibili abre 1080p o más con `SESSDATA`. En YouTube, el extractor integrado usa `--cookies`.

## Inicio rápido

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + pruebas + comprobación de idioma
make install      # instala en ~/.local/bin/vidpuller
vidpuller version
```

## Uso

```bash
# Cronología local
vidpuller download ./timeline.json -n 5

# Cronología remota (la plataforma se deduce del JSON o de la URL)
vidpuller download 'https://example.com/timeline.json' -n 5

# Plataforma explícita cuando la cronología no da pistas
vidpuller download ./timeline.json --platform douyin -n 5

# Inspeccionar antes de descargar
vidpuller download '<timeline-url>' -n 10 --dry-run

# Solo ciertos elementos
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# Listar elementos
vidpuller list '<timeline-url>' -n 20

# Solo metadatos (JSON, sin descarga)
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# Un vídeo de TikTok (se admiten enlaces cortos vt.tiktok.com)
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# Bilibili con cookies
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# Un vídeo de Bilibili (BV ID o URL completa; ?p=N elige una parte)
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# Un vídeo de YouTube (extractor nativo, hasta 1080p)
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## Opciones

| Opción | Descripción |
|---|---|
| `-o, --out DIR` | Raíz de salida, por defecto `./downloads` |
| `-n, --limit N` | Descarga como mucho N elementos (los ya descargados no cuentan; 0 = sin límite) |
| `--since YYYY-MM-DD` | Solo elementos publicados en esa fecha o después |
| `--ids A,B` | Solo los ID de vídeo dados (`video_id` / `aweme_id`) |
| `--dry-run` | Imprime lo que se descargaría |
| `--force` | Vuelve a descargar elementos ya registrados |
| `--interval DUR` | Espera entre elementos, por defecto `3s` |
| `--platform douyin\|tiktok\|bilibili\|youtube` | Sustituye la detección de plataforma |
| `--max-height N` | Altura máxima de YouTube (por defecto 1080; p. ej. 1440, 2160) |
| `--cookies FILE` | Cookies de sesión (Netscape / JSON / `k=v; k=v`) |
| `--progress` | Escribe líneas `PROGRESS <0-100>` en stderr |

## Salida

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            libro incremental (clave = platform:id)
```

El archivo `.json` es el elemento original de la cronología; `play_url` puede caducar y solo se conserva como referencia.

## Notas de implementación

- **Douyin**: `play_url` se descarga directamente con `Referer: https://www.douyin.com/`.
- **TikTok**: las URL del CDN están atadas a la sesión que las firmó (`ttwid`/`msToken`/`tt_chain_token`), así que las descargas por lotes vuelven a resolver la URL de cada vídeo con la sesión actual; reutilizar una URL de origen con otro tarro de cookies devuelve 403.
- El WAF de TikTok es una prueba de trabajo SHA-256 resuelta con la biblioteca estándar; no hace falta navegador.
- **Elección de calidad**: el `UrlKey` de cada variante (por ejemplo `…_bytevc1_1080p_1037801`) da el códec y la resolución reales; gana la resolución más alta y el bitrate desempata. La calidad descargada se imprime (por ejemplo `1080p bytevc1 722kbps`).
- **Bilibili**: `view` da el cid, `playurl` da los flujos DASH y `ffmpeg -c copy` los une. Para las URL de Akamai se prueba primero un host espejo, con el CDN original como reserva.
- **YouTube**: un extractor InnerTube nativo pide la respuesta del reproductor con el cliente VISIONOS, que expone URL directas de flujo adaptativo sin cifrado de firma. Se prefiere el vídeo más alto hasta `--max-height` (AVC/MP4 primero), se empareja con el mejor audio MP4 y se une con `ffmpeg -c copy`.

## Limitaciones

- **No se admiten URL sueltas de Douyin**: el endpoint de detalle exige una firma de navegador (`a_bogus`) y el HTTP puro no obtiene un flujo sin marca ni metadatos. Para un vídeo concreto use `download --ids <video_id>` o lea el elemento de un JSON de cronología.
- Las publicaciones de imagen de TikTok no tienen `play_url` y se saltan (la portada sí se puede sacar de la cronología).
- **YouTube** usa 1080p por defecto; pase `--max-height 1440` o `--max-height 2160` para más calidad (los archivos pueden pesar varios cientos de MB). Los vídeos con límite de edad, DRM o bloqueo regional pueden necesitar cookies o no poder descargarse. Las listas y los canales no se expanden; pase URL de un solo vídeo.
- El contenido privado o restringido por región lo comunica la plataforma como error; la herramienta no ve lo que los datos de la cronología no incluyen.
- Recoger la cronología, firmar y mantener la sesión es responsabilidad de quien llama; esta herramienta solo consume el JSON.

## Documentación

El inglés es la fuente de verdad; esta página es su traducción al español.

- **[English](README.md)**: el original en inglés, única fuente de verdad.
- **[AGENTS.md](AGENTS.md)**: restricciones del repositorio, incluida la regla de solo inglés y la política de fusión.

## License

[MIT](LICENSE)
