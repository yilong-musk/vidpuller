# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | [Español](README.es.md) | [Français](README.fr.md) | [日本語](README.ja.md) | [한국어](README.ko.md) | Português | [Русский](README.ru.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

Descarrega vídeos do Douyin, TikTok, Bilibili e YouTube a partir de uma cronologia JSON neutra quanto à plataforma — autónomo, um único binário estático, só a biblioteca padrão (não há um único `require` no `go.mod`). A extração do YouTube é integrada; o teto por omissão é 1080p e `--max-height` sobe-o para 1440p ou 2160p.

## Plataformas suportadas

| Plataforma&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp; | Conteúdo | URL única | Cronologia |
|---|---|---|---|
| <img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin | Vídeos de um criador | Não (precisa de uma assinatura de browser) | Sim |
| <img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok | Vídeos | Sim (`get`) | Sim |
| <img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili | Vídeos e partes | Sim (`get`) | Sim (DASH + ffmpeg) |
| <img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube | Vídeos | Sim (`get`) | Sim (nativo + ffmpeg) |

## Funcionalidades

- **Incremental**: um livro `.state.json` com chave `platform:id` salta o que já foi descarregado; `--force` volta a descarregar.
- **Sem marca de água**: usa os URL de reprodução que a plataforma fornece ou que a sessão atual gera; não há um passo de remoção da marca.
- **Um formato de entrada**: qualquer script, serviço ou ficheiro escrito à mão pode produzir o JSON da cronologia; não é preciso um SDK nem uma origem específica.
- **Metadados sem descarregar**: `vidpuller info <url>` imprime um objeto JSON neutro (título, autor, IDs, duração, dimensões, data de publicação, capa, partes) para Bilibili, YouTube e TikTok.
- **Binário único**: ligado estaticamente (`CGO_ENABLED=0`), sem dependências Go de terceiros.
- **Progresso legível por máquina**: `--progress` escreve linhas `PROGRESS <0-100>` no stderr para o chamador analisar.

## Formato de entrada

O nível de topo pode ser `{ "items": [...] }`, um array nu `[...]` ou um objeto que use `data` como alias de `items`. Exemplo completo:

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

| Campo | Obrigatório | Descrição |
|---|---|---|
| `items` / `data` | Sim | Array de itens; o topo também pode ser um array nu |
| `platform` / `source` | Não | `douyin`, `tiktok`, `bilibili` ou `youtube`; também se pode pôr em cada item |
| `video_id` | Sim | ID canónico do vídeo; aliases: `aweme_id` (Douyin/TikTok), `id` (BV ID do Bilibili) |
| `title` / `desc` | Não | Título; ambos são aceites e entram no nome do ficheiro de saída |
| `url` | Não | URL da página do vídeo; o Bilibili aceita um BV ID sozinho ou um URL completo |
| `play_url` | Não | URL direta do média; obrigatório no Douyin, resolvido de novo no TikTok, não usado no Bilibili |
| `cover_url` / `images` | Não | Imagem de capa; usa-se `images[0]` quando falta `cover_url` |
| `published_at` / `create_time` | Não | Hora de publicação; cadeia RFC 3339 ou segundos Unix |
| `author` | Não | Nome do autor, para o diretório de saída e a apresentação |

Ordem de deteção: `--platform` > `platform` do item > `platform`/`source` do topo > BV ID ou URL de página do Bilibili > URL do YouTube > URL da cronologia > hosts do URL do média. Se nenhuma regra bater, o comando falha em vez de adivinhar. Cronologias mistas são suportadas quando cada item traz `platform`.

## Instalação

Binários estáticos para Linux, macOS e Windows (amd64/arm64) vêm anexados a cada [release](https://github.com/yilong-musk/vidpuller/releases). Descarregue o arquivo da sua plataforma, confira-o com `checksums.txt` e ponha `vidpuller` no `PATH`.

Em macOS ou Linux com [Homebrew](https://brew.sh):

```bash
brew install --cask yilong-musk/tap/vidpuller
```

Com uma cadeia Go (1.27+):

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## Requisitos

- Go 1.27+ para compilar.
- `ffmpeg` no `PATH` para descargas do Bilibili e do YouTube (áudio e vídeo DASH são fundidos).
- Cookies opcionais para mais qualidade: o TikTok pode servir 720p em anónimo e 1080p com sessão iniciada; o Bilibili abre 1080p ou mais com `SESSDATA`. No YouTube, o extrator integrado usa `--cookies`.

## Início rápido

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + testes + verificação de língua
make install      # instala em ~/.local/bin/vidpuller
vidpuller version
```

## Utilização

```bash
# Cronologia local
vidpuller download ./timeline.json -n 5

# Cronologia remota (a plataforma deduz-se do JSON ou do URL)
vidpuller download 'https://example.com/timeline.json' -n 5

# Plataforma explícita quando a cronologia não dá pista
vidpuller download ./timeline.json --platform douyin -n 5

# Inspecionar antes de descarregar
vidpuller download '<timeline-url>' -n 10 --dry-run

# Só certos itens
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# Listar itens
vidpuller list '<timeline-url>' -n 20

# Só metadados (JSON, sem descarga)
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# Um vídeo do TikTok (ligações curtas vt.tiktok.com são aceites)
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# Bilibili com cookies
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# Um vídeo do Bilibili (BV ID ou URL completo; ?p=N escolhe uma parte)
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# Um vídeo do YouTube (extrator nativo, até 1080p)
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## Opções

| Opção | Descrição |
|---|---|
| `-o, --out DIR` | Raiz de saída, por omissão `./downloads` |
| `-n, --limit N` | No máximo N itens (os já descarregados não contam; 0 = sem limite) |
| `--since YYYY-MM-DD` | Só itens publicados nessa data ou depois |
| `--ids A,B` | Só os IDs de vídeo dados (`video_id` / `aweme_id`) |
| `--dry-run` | Imprime o que seria descarregado |
| `--force` | Volta a descarregar itens já registados |
| `--interval DUR` | Espera entre itens, por omissão `3s` |
| `--platform douyin\|tiktok\|bilibili\|youtube` | Substitui a deteção de plataforma |
| `--max-height N` | Altura máxima do YouTube (por omissão 1080; p. ex. 1440, 2160) |
| `--cookies FILE` | Cookies de sessão (Netscape / JSON / `k=v; k=v`) |
| `--progress` | Escreve linhas `PROGRESS <0-100>` no stderr |

## Saída

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            livro incremental (chave = platform:id)
```

O ficheiro `.json` é o item original da cronologia; `play_url` pode expirar e fica só como referência.

## Notas de implementação

- **Douyin**: `play_url` é descarregado diretamente com `Referer: https://www.douyin.com/`.
- **TikTok**: os URL do CDN estão presos à sessão que os assinou (`ttwid`/`msToken`/`tt_chain_token`), por isso as descargas em lote voltam a resolver o URL de cada vídeo com a sessão atual; reutilizar um URL de origem com outro frasco de cookies devolve 403.
- O WAF do TikTok é uma prova de trabalho SHA-256 resolvida com a biblioteca padrão; não é preciso browser.
- **Escolha de qualidade**: o `UrlKey` de cada variante (por exemplo `…_bytevc1_1080p_1037801`) dá o codec e a resolução reais; ganha a resolução mais alta e o bitrate desempata. A qualidade descarregada é impressa (por exemplo `1080p bytevc1 722kbps`).
- **Bilibili**: `view` dá o cid, `playurl` dá os fluxos DASH e `ffmpeg -c copy` junta-os. Para URL da Akamai tenta-se primeiro um host espelho, com o CDN original como reserva.
- **YouTube**: um extrator InnerTube nativo pede a resposta do leitor com o cliente VISIONOS, que expõe URL diretos de fluxo adaptativo sem cifra de assinatura. Prefere-se o vídeo mais alto até `--max-height` (AVC/MP4 primeiro), emparelhado com o melhor áudio MP4, e fundido com `ffmpeg -c copy`.

## Limitações

- **URL únicos do Douyin não são suportados**: o endpoint de detalhe exige uma assinatura de browser (`a_bogus`) e o HTTP puro não obtém um fluxo sem marca nem metadados. Para um vídeo use `download --ids <video_id>` ou leia o item de um JSON de cronologia.
- Publicações de imagem do TikTok não têm `play_url` e são saltadas (a capa ainda se pode obter da cronologia).
- **YouTube** fica em 1080p por omissão; passe `--max-height 1440` ou `--max-height 2160` para mais qualidade (os ficheiros podem ter várias centenas de MB). Vídeos com limite de idade, DRM ou bloqueio regional podem precisar de cookies ou não ser descarregáveis. Listas e feeds de canal não são expandidos; passe URL de um só vídeo.
- Conteúdo privado ou restrito por região é comunicado pela plataforma como erro; a ferramenta não vê o que os dados da cronologia não incluem.
- Recolher a cronologia, assinar e manter o estado de sessão é responsabilidade de quem chama; esta ferramenta só consome o JSON.

## Documentação

O inglês é a fonte de verdade; esta página é a tradução em português.

- **[English](README.md)**: o original em inglês, única fonte de verdade.
- **[AGENTS.md](AGENTS.md)**: restrições do repositório, incluindo a regra de só inglês e a política de fusão.

## License

[MIT](LICENSE)
