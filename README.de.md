# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | Deutsch | [Português](README.pt.md) | [Русский](README.ru.md)

Lädt Videos von Douyin, TikTok, Bilibili und YouTube aus einer plattformneutralen JSON-Timeline — eigenständig, ein statisches Binary, nur die Standardbibliothek (in `go.mod` steht kein einziges `require`). Die YouTube-Extraktion ist eingebaut; die Vorgabe ist 1080p, `--max-height` hebt sie auf 1440p oder 2160p.

## Unterstützte Plattformen

| Plattform&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Inhalt | Einzel-URL | Timeline |
|---|---|---|---|
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Videos eines Creators | Nein (Browsersignatur nötig) | Ja |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Videos | Ja (`get`) | Ja |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Videos und Teile | Ja (`get`) | Ja (DASH + ffmpeg) |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Videos | Ja (`get`) | Ja (nativ + ffmpeg) |

## Funktionen

- **Inkrementell**: ein `.state.json`-Buch mit Schlüssel `platform:id` überspringt bereits Geladenes; `--force` lädt neu.
- **Ohne Wasserzeichen**: nutzt die Play-URLs der Plattform oder der aktuellen Sitzung; es gibt keinen Schritt zum Entfernen des Wasserzeichens.
- **Ein Eingabeformat**: jedes Skript, jeder Dienst oder jede von Hand geschriebene Datei kann die Timeline-JSON erzeugen; kein Provider-SDK und kein bestimmter Upstream nötig.
- **Metadaten ohne Download**: `vidpuller info <url>` druckt ein plattformneutrales JSON-Objekt (Titel, Autor, IDs, Dauer, Maße, Veröffentlichungsdatum, Cover, Teile) für Bilibili, YouTube und TikTok.
- **Ein Binary**: statisch gelinkt (`CGO_ENABLED=0`), keine Go-Abhängigkeiten von Dritten.
- **Maschinenlesbarer Fortschritt**: `--progress` schreibt `PROGRESS <0-100>`-Zeilen nach stderr, die der Aufrufer auswerten kann.

## Eingabeformat

Die oberste Ebene kann `{ "items": [...] }` sein, ein nacktes Array `[...]` oder ein Objekt, das `data` als Alias für `items` nutzt. Vollständiges Beispiel:

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

| Feld | Pflicht | Beschreibung |
|---|---|---|
| `items` / `data` | Ja | Elementliste; die oberste Ebene darf auch ein nacktes Array sein |
| `platform` / `source` | Nein | `douyin`, `tiktok`, `bilibili` oder `youtube`; auch je Element setzbar |
| `video_id` | Ja | Kanonische Video-ID; Aliase: `aweme_id` (Douyin/TikTok), `id` (Bilibili-BV-ID) |
| `title` / `desc` | Nein | Titel; beides wird akzeptiert und im Ausgabedateinamen verwendet |
| `url` | Nein | Videoseiten-URL; Bilibili nimmt eine BV-ID allein oder eine volle URL |
| `play_url` | Nein | Direkte Medien-URL; für Douyin nötig, für TikTok neu aufgelöst, für Bilibili ungenutzt |
| `cover_url` / `images` | Nein | Coverbild; `images[0]`, wenn `cover_url` fehlt |
| `published_at` / `create_time` | Nein | Veröffentlichungszeit; RFC-3339-Zeichenkette oder Unix-Sekunden |
| `author` | Nein | Autorenname, für Ausgabeverzeichnis und Anzeige |

Erkennungsreihenfolge: `--platform` > `platform` des Elements > `platform`/`source` der obersten Ebene > BV-ID oder Bilibili-Seiten-URL > YouTube-URL > Timeline-URL > Hosts der Medien-URL. Trifft keine Regel zu, schlägt der Befehl fehl, statt eine Plattform zu raten. Gemischte Timelines gehen, wenn jedes Element `platform` trägt.

## Installation

Statische Binaries für Linux, macOS und Windows (amd64/arm64) hängen an jedem [Release](https://github.com/yilong-musk/vidpuller/releases). Archiv der eigenen Plattform laden, gegen `checksums.txt` prüfen und `vidpuller` auf den `PATH` legen.

Unter macOS oder Linux mit [Homebrew](https://brew.sh):

```bash
brew install --cask yilong-musk/tap/vidpuller
```

Mit einer Go-Toolchain (1.27+):

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## Voraussetzungen

- Go 1.27+ zum Bauen.
- `ffmpeg` auf dem `PATH` für Bilibili- und YouTube-Downloads (DASH-Audio und -Video werden zusammengefügt).
- Optionale Cookies für höhere Qualität: TikTok liefert anonym oft 720p und mit angemeldeter Sitzung 1080p; Bilibili schaltet 1080p und mehr mit `SESSDATA` frei. Bei YouTube nutzt der eingebaute Extraktor `--cookies`.

## Schnellstart

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + Tests + Sprachprüfung
make install      # installiert nach ~/.local/bin/vidpuller
vidpuller version
```

## Verwendung

```bash
# Lokale Timeline
vidpuller download ./timeline.json -n 5

# Entfernte Timeline (Plattform aus JSON oder URL)
vidpuller download 'https://example.com/timeline.json' -n 5

# Plattform ausdrücklich, wenn die Timeline keinen Hinweis hat
vidpuller download ./timeline.json --platform douyin -n 5

# Vor dem Laden ansehen
vidpuller download '<timeline-url>' -n 10 --dry-run

# Nur bestimmte Elemente
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# Elemente auflisten
vidpuller list '<timeline-url>' -n 20

# Nur Metadaten (JSON, kein Download)
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# Ein TikTok-Video (Kurzlinks vt.tiktok.com gehen)
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# Bilibili mit Cookies
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# Ein Bilibili-Video (BV-ID oder volle URL; ?p=N wählt einen Teil)
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# Ein YouTube-Video (nativer Extraktor, bis 1080p)
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## Optionen

| Option | Beschreibung |
|---|---|
| `-o, --out DIR` | Ausgabewurzel, Vorgabe `./downloads` |
| `-n, --limit N` | Höchstens N Elemente (bereits geladene zählen nicht; 0 = unbegrenzt) |
| `--since YYYY-MM-DD` | Nur Elemente, die an diesem Tag oder danach veröffentlicht wurden |
| `--ids A,B` | Nur die genannten Video-IDs (`video_id` / `aweme_id`) |
| `--dry-run` | Zeigt, was geladen würde |
| `--force` | Lädt bereits verzeichnete Elemente neu |
| `--interval DUR` | Pause zwischen Elementen, Vorgabe `3s` |
| `--platform douyin\|tiktok\|bilibili\|youtube` | Überschreibt die Plattformerkennung |
| `--max-height N` | Maximale YouTube-Höhe (Vorgabe 1080; z. B. 1440, 2160) |
| `--cookies FILE` | Login-Cookies (Netscape / JSON / `k=v; k=v`) |
| `--progress` | Schreibt `PROGRESS <0-100>`-Zeilen nach stderr |

## Ausgabe

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            inkrementelles Buch (Schlüssel = platform:id)
```

Die `.json`-Datei ist das ursprüngliche Timeline-Element; `play_url` kann ablaufen und bleibt nur als Hinweis.

## Hinweise zur Umsetzung

- **Douyin**: `play_url` wird direkt mit `Referer: https://www.douyin.com/` geladen.
- **TikTok**: CDN-URLs hängen an der Sitzung, die sie signiert hat (`ttwid`/`msToken`/`tt_chain_token`); Stapel lösen deshalb jede Video-URL mit der aktuellen Sitzung neu auf. Eine Upstream-URL mit einem anderen Cookie-Glas ergibt 403.
- TikToks WAF ist ein SHA-256-Proof-of-Work, gelöst mit der Standardbibliothek; kein Browser nötig.
- **Qualitätswahl**: der `UrlKey` jeder Variante (zum Beispiel `…_bytevc1_1080p_1037801`) nennt echten Codec und Auflösung; die höchste Auflösung gewinnt, die Bitrate entscheidet bei Gleichstand. Die geladene Qualität wird ausgegeben (zum Beispiel `1080p bytevc1 722kbps`).
- **Bilibili**: `view` liefert die cid, `playurl` die DASH-Ströme, `ffmpeg -c copy` fügt sie zusammen. Für Akamai-URLs wird zuerst ein Spiegelhost versucht, der ursprüngliche CDN ist der Rückfall.
- **YouTube**: ein nativer InnerTube-Extraktor holt die Player-Antwort mit dem VISIONOS-Client, der direkte Adaptive-Stream-URLs ohne Signaturchiffre liefert. Bevorzugt wird das höchste Video bis `--max-height` (AVC/MP4 zuerst), gepaart mit dem besten MP4-Audio, zusammengefügt mit `ffmpeg -c copy`.

## Grenzen

- **Einzelne Douyin-Video-URLs werden nicht unterstützt**: der Detail-Endpunkt verlangt eine Browsersignatur (`a_bogus`), reines HTTP bekommt weder einen wasserzeichenfreien Strom noch Metadaten. Für ein einzelnes Video `download --ids <video_id>` nutzen oder das Element aus einer Timeline-JSON lesen.
- TikTok-Bildbeiträge haben kein `play_url` und werden übersprungen (das Cover lässt sich weiter aus der Timeline holen).
- **YouTube** steht auf 1080p; `--max-height 1440` oder `--max-height 2160` für mehr (Dateien können mehrere hundert MB groß sein). Altersbeschränkte, DRM-geschützte oder regional gesperrte Videos brauchen vielleicht Cookies oder lassen sich nicht laden. Playlists und Kanalfeeds werden nicht aufgefaltet; einzelne Video-URLs übergeben.
- Privates oder regional beschränktes Material meldet die Plattform als Fehler; das Werkzeug sieht nicht, was die Timeline-Daten nicht enthalten.
- Timeline sammeln, signieren und den Login-Zustand halten ist Sache des Aufrufers; dieses Werkzeug verbraucht nur das JSON.

## Dokumentation

Englisch ist die maßgebliche Fassung; diese Seite ist die deutsche Übersetzung.

- **[English](README.md)**: englische Fassung, einzige maßgebliche Quelle.
- **[AGENTS.md](AGENTS.md)**: Repository-Regeln, darunter die Nur-Englisch-Regel und die Merge-Politik.

## License

[MIT](LICENSE)
