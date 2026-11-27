# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[English](README.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md) | 日本語 | [한국어](README.ko.md) | [Español](README.es.md) | [Français](README.fr.md) | [Deutsch](README.de.md) | [Português](README.pt.md) | [Русский](README.ru.md)

Douyin、TikTok、Bilibili、YouTube の動画を、プラットフォームに依存しない JSON タイムラインからダウンロードします。自己完結の単一静的バイナリで、標準ライブラリのみです（`go.mod` に `require` は一つもありません）。YouTube の抽出は内蔵で、既定の上限は 1080p、`--max-height` で 1440p または 2160p まで上げられます。

## 対応プラットフォーム

<table>
  <thead>
    <tr><th width="136">プラットフォーム</th><th>内容</th><th>単体 URL</th><th>タイムライン</th></tr>
  </thead>
  <tbody>
    <tr><td><img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin</td><td>クリエイターの動画</td><td>不可（ブラウザ署名が必要）</td><td>可</td></tr>
    <tr><td><img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok</td><td>動画</td><td>可（<code>get</code>）</td><td>可</td></tr>
    <tr><td><img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili</td><td>動画と分 P</td><td>可（<code>get</code>）</td><td>可（DASH + ffmpeg）</td></tr>
    <tr><td><img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube</td><td>動画</td><td>可（<code>get</code>）</td><td>可（内蔵 + ffmpeg）</td></tr>
  </tbody>
</table>

## 機能

- **増分**: `.state.json` 台帳のキーは `platform:id`。ダウンロード済みは飛ばします。`--force` で取り直します。
- **透かしなし**: プラットフォームが渡した、または現在のセッションが発行した再生 URL を使います。透かし除去の工程はありません。
- **入力は一つ**: スクリプト、サービス、手書きファイルのどれでもタイムライン JSON を作れます。特定の provider SDK や上流は不要です。
- **ダウンロードせずにメタデータ**: `vidpuller info <url>` はプラットフォーム非依存の JSON（タイトル、作者、ID、長さ、寸法、公開日、カバー、パート）を出します。対象は Bilibili、YouTube、TikTok です。
- **単一バイナリ**: 静的リンク（`CGO_ENABLED=0`）、サードパーティの Go 依存はありません。
- **機械可読な進捗**: `--progress` は呼び出し元が解析できる `PROGRESS <0-100>` 行を stderr に書きます。

## 入力形式

トップレベルは `{ "items": [...] }`、素の配列 `[...]`、または `items` の別名 `data` を持つオブジェクトです。完全な例:

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

| フィールド | 必須 | 説明 |
|---|---|---|
| `items` / `data` | はい | 項目の配列。トップレベルが素の配列でもよい |
| `platform` / `source` | いいえ | `douyin`、`tiktok`、`bilibili`、`youtube`。項目ごとにも置ける |
| `video_id` | はい | 正規の動画 ID。別名: `aweme_id`（Douyin/TikTok）、`id`（Bilibili の BV ID） |
| `title` / `desc` | いいえ | タイトル。どちらも受け付け、出力ファイル名に使う |
| `url` | いいえ | 動画ページの URL。Bilibili は BV ID だけでも完全な URL でもよい |
| `play_url` | いいえ | 直接のメディア URL。Douyin では必須、TikTok では再解決、Bilibili では未使用 |
| `cover_url` / `images` | いいえ | カバー画像。`cover_url` が無いときは `images[0]` |
| `published_at` / `create_time` | いいえ | 公開時刻。RFC 3339 文字列または Unix 秒 |
| `author` | いいえ | 作者名。出力ディレクトリと表示に使う |

プラットフォーム判定の順: `--platform` > 項目の `platform` > トップレベルの `platform`/`source` > BV ID または Bilibili のページ URL > YouTube URL > タイムライン URL > メディア URL のホスト。どれにも当てはまらなければ、推測せずコマンドは失敗します。各項目が `platform` を持てば、混在タイムラインも使えます。

## インストール

Linux、macOS、Windows（amd64/arm64）向けの静的バイナリは毎回の [release](https://github.com/yilong-musk/vidpuller/releases) に添付されます。自分のプラットフォームのアーカイブを取り、`checksums.txt` と照合して、`vidpuller` を `PATH` に置いてください。

macOS または Linux で [Homebrew](https://brew.sh) がある場合:

```bash
brew install --cask yilong-musk/tap/vidpuller
```

Go ツールチェーン（1.27 以上）がある場合:

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## 要件

- ビルドには Go 1.27 以上。
- Bilibili と YouTube のダウンロードには `PATH` 上の `ffmpeg`（DASH の音声と映像を結合します）。
- 画質を上げる任意の cookie: TikTok は匿名だと 720p、ログイン済みセッションだと 1080p のことがあります。Bilibili は `SESSDATA` で 1080p 以上が開きます。YouTube では内蔵抽出器が `--cookies` を使います。

## クイックスタート

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + テスト + 言語チェック
make install      # ~/.local/bin/vidpuller にインストール
vidpuller version
```

## 使い方

```bash
# ローカルのタイムライン
vidpuller download ./timeline.json -n 5

# リモートのタイムライン（プラットフォームは JSON か URL から判定）
vidpuller download 'https://example.com/timeline.json' -n 5

# タイムラインに手がかりが無いときにプラットフォームを明示
vidpuller download ./timeline.json --platform douyin -n 5

# ダウンロード前に確認
vidpuller download '<timeline-url>' -n 10 --dry-run

# 指定した項目だけ
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# 項目を列挙
vidpuller list '<timeline-url>' -n 20

# メタデータのみ（JSON、ダウンロードしない）
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# TikTok を 1 本（vt.tiktok.com の短縮リンクも可）
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# cookie 付きの Bilibili
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# Bilibili を 1 本（BV ID または完全な URL。?p=N でパートを選ぶ）
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# YouTube を 1 本（内蔵抽出器、最大 1080p）
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## オプション

| オプション | 説明 |
|---|---|
| `-o, --out DIR` | 出力ルート。既定は `./downloads` |
| `-n, --limit N` | 最大 N 件（ダウンロード済みで飛ばしたものは数えない。0 = 無制限） |
| `--since YYYY-MM-DD` | この日付以降に公開された項目だけ |
| `--ids A,B` | 指定した動画 ID だけ（`video_id` / `aweme_id`） |
| `--dry-run` | ダウンロードされるものを表示する |
| `--force` | 記録済みの項目も取り直す |
| `--interval DUR` | 項目間の待ち。既定は `3s` |
| `--platform douyin\|tiktok\|bilibili\|youtube` | プラットフォーム判定を上書きする |
| `--max-height N` | YouTube の最大高さ（既定 1080。例: 1440、2160） |
| `--cookies FILE` | ログイン cookie（Netscape / JSON / `k=v; k=v`） |
| `--progress` | stderr に `PROGRESS <0-100>` 行を書く |

## 出力

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            増分台帳（キー = platform:id）
```

`.json` は元のタイムライン項目です。`play_url` は期限切れになることがあり、参照用に残すだけです。

## 実装メモ

- **Douyin**: `play_url` を `Referer: https://www.douyin.com/` 付きで直接ダウンロードします。
- **TikTok**: CDN URL は署名したセッション（`ttwid`/`msToken`/`tt_chain_token`）に縛られます。バッチでは現在のセッションで各動画の URL を取り直します。別の cookie jar で上流 URL を使うと 403 になります。
- TikTok の WAF は標準ライブラリで解く SHA-256 のプルーフ・オブ・ワークです。ブラウザは不要です。
- **画質の選択**: 各レンディションの `UrlKey`（例: `…_bytevc1_1080p_1037801`）が実際のコーデックと解像度です。解像度が優先で、同じならビットレートです。結果は表示されます（例: `1080p bytevc1 722kbps`）。
- **Bilibili**: `view` が cid、`playurl` が DASH ストリーム、`ffmpeg -c copy` が結合します。Akamai URL はまずミラーホストを試し、元の CDN にフォールバックします。
- **YouTube**: 内蔵の InnerTube 抽出器が VISIONOS クライアントで player response を取ります。このクライアントは署名暗号のない直接の adaptive stream URL を出します。`--max-height` 以下で最も高い映像（AVC/MP4 優先）を、最良の MP4 音声と組み合わせ、`ffmpeg -c copy` で結合します。

## 制限

- **Douyin の単体動画 URL は非対応**: 詳細エンドポイントはブラウザ署名（`a_bogus`）が必要で、純粋な HTTP では透かしなしのストリームもメタデータも取れません。1 本だけ欲しいときは `download --ids <video_id>` を使うか、タイムライン JSON から項目を読んでください。
- TikTok の画像投稿には `play_url` が無く、飛ばされます（カバーはタイムラインから取れます）。
- **YouTube** の既定は 1080p です。より高くするには `--max-height 1440` または `--max-height 2160`（ファイルは数百 MB になることがあります）。年齢制限、DRM、地域制限の動画は cookie が必要か、ダウンロードできないことがあります。プレイリストとチャンネルフィードは展開しません。単体の動画 URL を渡してください。
- 非公開または地域制限のコンテンツはプラットフォームがエラーとして返します。タイムラインデータに無いものは、このツールからも見えません。
- タイムラインの収集、署名、ログイン状態は呼び出し側の責任です。このツールは JSON を消費するだけです。

## ドキュメント

英語版が唯一の正本です。このページはその日本語訳です。

- **[English](README.md)**: 英語の原文。唯一の正本。
- **[AGENTS.md](AGENTS.md)**: リポジトリの制約。英語のみの規則とマージ方針を含む。

## License

[MIT](LICENSE)
