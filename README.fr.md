# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | [Español](README.es.md) | Français | [Deutsch](README.de.md) | [Português](README.pt.md) | [Русский](README.ru.md)

Télécharge des vidéos Douyin, TikTok, Bilibili et YouTube à partir d'une chronologie JSON neutre vis-à-vis de la plateforme : autonome, un seul binaire statique, bibliothèque standard uniquement (pas un seul `require` dans `go.mod`). L'extraction YouTube est intégrée ; le plafond par défaut est 1080p et `--max-height` le monte à 1440p ou 2160p.

## Plateformes prises en charge

| Plateforme&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Contenu | URL seule | Chronologie |
|---|---|---|---|
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Vidéos d'un créateur | Non (signature de navigateur requise) | Oui |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Vidéos | Oui (`get`) | Oui |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Vidéos et parties | Oui (`get`) | Oui (DASH + ffmpeg) |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Vidéos | Oui (`get`) | Oui (natif + ffmpeg) |

## Fonctions

- **Incrémental** : un registre `.state.json` indexé par `platform:id` saute ce qui est déjà téléchargé ; `--force` retélécharge.
- **Sans filigrane** : utilise les URL de lecture fournies par la plateforme ou produites par la session courante ; pas d'étape de retrait du filigrane.
- **Un seul format d'entrée** : tout script, service ou fichier écrit à la main peut produire le JSON de chronologie ; aucun SDK ni amont particulier n'est requis.
- **Métadonnées sans téléchargement** : `vidpuller info <url>` imprime un objet JSON neutre (titre, auteur, ID, durée, dimensions, date de publication, couverture, parties) pour Bilibili, YouTube et TikTok.
- **Binaire unique** : lié statiquement (`CGO_ENABLED=0`), sans dépendance Go tierce.
- **Progression lisible par machine** : `--progress` écrit des lignes `PROGRESS <0-100>` sur stderr, à analyser par l'appelant.

## Format d'entrée

Le niveau supérieur peut être `{ "items": [...] }`, un tableau nu `[...]`, ou un objet qui utilise `data` comme alias de `items`. Exemple complet :

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

| Champ | Requis | Description |
|---|---|---|
| `items` / `data` | Oui | Tableau d'éléments ; le niveau supérieur peut aussi être un tableau nu |
| `platform` / `source` | Non | `douyin`, `tiktok`, `bilibili` ou `youtube` ; peut aussi être posé sur chaque élément |
| `video_id` | Oui | ID canonique ; alias : `aweme_id` (Douyin/TikTok), `id` (BV ID Bilibili) |
| `title` / `desc` | Non | Titre ; les deux sont acceptés et servent au nom de fichier |
| `url` | Non | URL de la page vidéo ; Bilibili accepte un BV ID seul ou une URL complète |
| `play_url` | Non | URL média directe ; requis pour Douyin, recalculé pour TikTok, inutilisé pour Bilibili |
| `cover_url` / `images` | Non | Image de couverture ; `images[0]` si `cover_url` est absent |
| `published_at` / `create_time` | Non | Heure de publication ; chaîne RFC 3339 ou secondes Unix |
| `author` | Non | Nom d'auteur, pour le répertoire de sortie et l'affichage |

Ordre de détection : `--platform` > `platform` de l'élément > `platform`/`source` du niveau supérieur > BV ID ou URL de page Bilibili > URL YouTube > URL de chronologie > hôtes de l'URL média. Si aucune règle ne correspond, la commande échoue au lieu de deviner. Les chronologies mixtes sont prises en charge quand chaque élément porte `platform`.

## Installation

Les binaires statiques pour Linux, macOS et Windows (amd64/arm64) sont joints à chaque [release](https://github.com/yilong-musk/vidpuller/releases). Téléchargez l'archive de votre plateforme, vérifiez-la avec `checksums.txt` et placez `vidpuller` sur le `PATH`.

Sous macOS ou Linux avec [Homebrew](https://brew.sh) :

```bash
brew install --cask yilong-musk/tap/vidpuller
```

Avec une chaîne Go (1.27+) :

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## Prérequis

- Go 1.27+ pour compiler.
- `ffmpeg` sur le `PATH` pour Bilibili et YouTube (l'audio et la vidéo DASH sont fusionnés).
- Cookies facultatifs pour une meilleure qualité : TikTok peut servir du 720p en anonyme et du 1080p avec une session connectée ; Bilibili ouvre le 1080p et plus avec `SESSDATA`. Pour YouTube, l'extracteur intégré utilise `--cookies`.

## Démarrage rapide

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + tests + contrôle de langue
make install      # installe dans ~/.local/bin/vidpuller
vidpuller version
```

## Utilisation

```bash
# Chronologie locale
vidpuller download ./timeline.json -n 5

# Chronologie distante (plateforme déduite du JSON ou de l'URL)
vidpuller download 'https://example.com/timeline.json' -n 5

# Plateforme explicite si la chronologie n'indique rien
vidpuller download ./timeline.json --platform douyin -n 5

# Inspecter avant de télécharger
vidpuller download '<timeline-url>' -n 10 --dry-run

# Seulement certains éléments
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# Lister les éléments
vidpuller list '<timeline-url>' -n 20

# Métadonnées seules (JSON, pas de téléchargement)
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# Une vidéo TikTok (les liens courts vt.tiktok.com sont acceptés)
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# Bilibili avec cookies
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# Une vidéo Bilibili (BV ID ou URL complète ; ?p=N choisit une partie)
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# Une vidéo YouTube (extracteur natif, jusqu'à 1080p)
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## Options

| Option | Description |
|---|---|
| `-o, --out DIR` | Racine de sortie, `./downloads` par défaut |
| `-n, --limit N` | Au plus N éléments (ceux déjà téléchargés ne comptent pas ; 0 = illimité) |
| `--since YYYY-MM-DD` | Seulement les éléments publiés ce jour-là ou après |
| `--ids A,B` | Seulement les ID vidéo donnés (`video_id` / `aweme_id`) |
| `--dry-run` | Affiche ce qui serait téléchargé |
| `--force` | Retélécharge les éléments déjà enregistrés |
| `--interval DUR` | Délai entre éléments, `3s` par défaut |
| `--platform douyin\|tiktok\|bilibili\|youtube` | Force la détection de plateforme |
| `--max-height N` | Hauteur YouTube maximale (1080 par défaut ; p. ex. 1440, 2160) |
| `--cookies FILE` | Cookies de session (Netscape / JSON / `k=v; k=v`) |
| `--progress` | Écrit des lignes `PROGRESS <0-100>` sur stderr |

## Sortie

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            registre incrémental (clé = platform:id)
```

Le fichier `.json` est l'élément d'origine ; `play_url` peut expirer et n'est gardé qu'à titre de référence.

## Notes d'implémentation

- **Douyin** : `play_url` est téléchargé directement avec `Referer: https://www.douyin.com/`.
- **TikTok** : les URL CDN sont liées à la session qui les a signées (`ttwid`/`msToken`/`tt_chain_token`) ; les lots recalculent donc l'URL de chaque vidéo avec la session courante. Réutiliser une URL amont avec un autre bocal de cookies renvoie 403.
- Le WAF de TikTok est une preuve de travail SHA-256 résolue avec la bibliothèque standard ; aucun navigateur n'est requis.
- **Choix de qualité** : le `UrlKey` de chaque variante (par exemple `…_bytevc1_1080p_1037801`) donne le codec et la résolution réels ; la plus haute résolution gagne, le débit départage. La qualité téléchargée est affichée (par exemple `1080p bytevc1 722kbps`).
- **Bilibili** : `view` fournit le cid, `playurl` les flux DASH, et `ffmpeg -c copy` les fusionne. Un hôte miroir est essayé d'abord pour les URL Akamai, le CDN d'origine en repli.
- **YouTube** : un extracteur InnerTube natif demande la réponse du lecteur avec le client VISIONOS, qui expose des URL de flux adaptatif directes sans chiffre de signature. La vidéo la plus haute jusqu'à `--max-height` est préférée (AVC/MP4 d'abord), associée au meilleur audio MP4, puis fusionnée par `ffmpeg -c copy`.

## Limites

- **Les URL Douyin d'une seule vidéo ne sont pas prises en charge** : l'endpoint de détail exige une signature de navigateur (`a_bogus`), et le HTTP pur n'obtient ni flux sans filigrane ni métadonnées. Pour une vidéo, utilisez `download --ids <video_id>` ou lisez l'élément dans un JSON de chronologie.
- Les posts image TikTok n'ont pas de `play_url` et sont sautés (la couverture reste lisible dans la chronologie).
- **YouTube** est à 1080p par défaut ; passez `--max-height 1440` ou `--max-height 2160` pour plus (les fichiers peuvent faire plusieurs centaines de Mo). Les vidéos limitées par l'âge, protégées par DRM ou bloquées par région peuvent exiger des cookies ou être impossibles à télécharger. Les playlists et les fils de chaîne ne sont pas développés ; passez des URL de vidéo seule.
- Le contenu privé ou restreint par région est signalé comme erreur par la plateforme ; l'outil ne voit pas ce que les données de chronologie n'incluent pas.
- La collecte de la chronologie, la signature et l'état de connexion sont à la charge de l'appelant ; cet outil ne consomme que le JSON.

## Documentation

L'anglais est la source de vérité ; cette page en est la traduction française.

- **[English](README.md)** : l'original anglais, seule source de vérité.
- **[AGENTS.md](AGENTS.md)** : contraintes du dépôt, dont la règle anglais seul et la politique de fusion.

## License

[MIT](LICENSE)
