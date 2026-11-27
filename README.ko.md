# vidpuller

[![CI](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml/badge.svg)](https://github.com/yilong-musk/vidpuller/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.27-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

[Deutsch](README.de.md) | [English](README.md) | [Español](README.es.md) | [Français](README.fr.md) | [日本語](README.ja.md) | 한국어 | [Português](README.pt.md) | [Русский](README.ru.md) | [简体中文](README.zh-CN.md) | [繁體中文](README.zh-TW.md)

Douyin, TikTok, Bilibili, YouTube 동영상을 플랫폼에 무관한 JSON 타임라인에서 받습니다. 자체 완결된 정적 바이너리 하나이고, 표준 라이브러리만 씁니다(`go.mod`에 `require`가 하나도 없습니다). YouTube 추출은 내장이고, 기본 상한은 1080p이며 `--max-height`로 1440p 또는 2160p까지 올립니다.

## 지원 플랫폼

<table>
  <thead>
    <tr><th width="136">플랫폼</th><th>내용</th><th>단일 URL</th><th>타임라인</th></tr>
  </thead>
  <tbody>
    <tr><td><img src="docs/assets/douyin.png" width="18" height="18" align="absmiddle" alt="Douyin">&nbsp;Douyin</td><td>크리에이터 동영상</td><td>아니오 (브라우저 서명 필요)</td><td>예</td></tr>
    <tr><td><img src="docs/assets/tiktok.png" width="18" height="18" align="absmiddle" alt="TikTok">&nbsp;TikTok</td><td>동영상</td><td>예 (<code>get</code>)</td><td>예</td></tr>
    <tr><td><img src="docs/assets/bilibili.png" width="18" height="18" align="absmiddle" alt="Bilibili">&nbsp;Bilibili</td><td>동영상과 분할</td><td>예 (<code>get</code>)</td><td>예 (DASH + ffmpeg)</td></tr>
    <tr><td><img src="docs/assets/youtube.png" width="18" height="18" align="absmiddle" alt="YouTube">&nbsp;YouTube</td><td>동영상</td><td>예 (<code>get</code>)</td><td>예 (내장 + ffmpeg)</td></tr>
  </tbody>
</table>

## 기능

- **증분**: `.state.json` 원장의 키는 `platform:id`입니다. 이미 받은 항목은 건너뛰고, `--force`는 다시 받습니다.
- **워터마크 없음**: 플랫폼이 준 재생 URL 또는 현재 세션이 만든 URL을 씁니다. 워터마크 제거 단계는 없습니다.
- **입력 형식 하나**: 스크립트, 서비스, 손으로 쓴 파일 모두 타임라인 JSON을 만들 수 있습니다. 특정 provider SDK나 업스트림은 필요 없습니다.
- **받지 않고 메타데이터**: `vidpuller info <url>`은 플랫폼에 무관한 JSON(제목, 작성자, ID, 길이, 크기, 게시일, 커버, 파트)을 출력합니다. Bilibili, YouTube, TikTok입니다.
- **단일 바이너리**: 정적 링크(`CGO_ENABLED=0`), 서드파티 Go 의존성 없음.
- **기계가 읽는 진행률**: `--progress`는 호출자가 파싱할 `PROGRESS <0-100>` 줄을 stderr에 씁니다.

## 입력 형식

최상위는 `{ "items": [...] }`, 배열 그대로 `[...]`, 또는 `items`의 별칭 `data`를 쓰는 객체입니다. 완전한 예:

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

| 필드 | 필수 | 설명 |
|---|---|---|
| `items` / `data` | 예 | 항목 배열. 최상위가 배열 그대로여도 됩니다 |
| `platform` / `source` | 아니오 | `douyin`, `tiktok`, `bilibili`, `youtube`. 항목마다 둘 수도 있습니다 |
| `video_id` | 예 | 정규 동영상 ID. 별칭: `aweme_id`(Douyin/TikTok), `id`(Bilibili BV ID) |
| `title` / `desc` | 아니오 | 제목. 둘 다 받고 출력 파일 이름에 씁니다 |
| `url` | 아니오 | 동영상 페이지 URL. Bilibili는 BV ID만으로도, 전체 URL로도 됩니다 |
| `play_url` | 아니오 | 직접 미디어 URL. Douyin은 필수, TikTok은 다시 해석, Bilibili는 쓰지 않음 |
| `cover_url` / `images` | 아니오 | 커버 이미지. `cover_url`이 없으면 `images[0]` |
| `published_at` / `create_time` | 아니오 | 게시 시각. RFC 3339 문자열 또는 Unix 초 |
| `author` | 아니오 | 작성자 이름. 출력 디렉터리와 표시에 씁니다 |

플랫폼 판정 순서: `--platform` > 항목의 `platform` > 최상위 `platform`/`source` > BV ID 또는 Bilibili 페이지 URL > YouTube URL > 타임라인 URL > 미디어 URL 호스트. 아무 규칙도 맞지 않으면 추측하지 않고 명령이 실패합니다. 각 항목이 `platform`을 가지면 혼합 타임라인도 됩니다.

## 설치

Linux, macOS, Windows(amd64/arm64)용 정적 바이너리는 매 [release](https://github.com/yilong-musk/vidpuller/releases)에 첨부됩니다. 자기 플랫폼 아카이브를 받고 `checksums.txt`와 맞춘 뒤 `vidpuller`를 `PATH`에 두세요.

macOS 또는 Linux에 [Homebrew](https://brew.sh)가 있으면:

```bash
brew install --cask yilong-musk/tap/vidpuller
```

Go 도구 사슬(1.27+)이 있으면:

```bash
go install github.com/yilong-musk/vidpuller@latest
```

## 요구 사항

- 빌드에는 Go 1.27 이상.
- Bilibili와 YouTube 다운로드에는 `PATH`의 `ffmpeg`(DASH 음성과 영상을 합칩니다).
- 화질을 올리는 선택적 cookie: TikTok은 익명이면 720p, 로그인 세션이면 1080p일 수 있습니다. Bilibili는 `SESSDATA`로 1080p 이상을 엽니다. YouTube에서는 내장 추출기가 `--cookies`를 씁니다.

## 빠른 시작

```bash
git clone https://github.com/yilong-musk/vidpuller.git
cd vidpuller

make check        # gofmt + vet + 테스트 + 언어 검사
make install      # ~/.local/bin/vidpuller 에 설치
vidpuller version
```

## 사용법

```bash
# 로컬 타임라인
vidpuller download ./timeline.json -n 5

# 원격 타임라인 (플랫폼은 JSON 또는 URL에서 판정)
vidpuller download 'https://example.com/timeline.json' -n 5

# 타임라인에 단서가 없을 때 플랫폼을 명시
vidpuller download ./timeline.json --platform douyin -n 5

# 받기 전에 확인
vidpuller download '<timeline-url>' -n 10 --dry-run

# 지정한 항목만
vidpuller download '<timeline-url>' --ids 1234567890123456789,1234567890123456790

# 항목 나열
vidpuller list '<timeline-url>' -n 20

# 메타데이터만 (JSON, 다운로드 없음)
vidpuller info 'https://www.bilibili.com/video/BV1xxxxxxxxx'
vidpuller info 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'

# TikTok 하나 (vt.tiktok.com 단축 링크 지원)
vidpuller get 'https://www.tiktok.com/@<handle>/video/7000000000000000001'

# cookie가 있는 Bilibili
vidpuller download '<bilibili-timeline-url>' -n 3 \
  --cookies ~/cookies/bilibili.txt

# Bilibili 하나 (BV ID 또는 전체 URL. ?p=N 이 파트를 고름)
vidpuller get 'https://www.bilibili.com/video/BV1xxxxxxxxx'

# YouTube 하나 (내장 추출기, 최대 1080p)
vidpuller get 'https://www.youtube.com/watch?v=dQw4w9WgXcQ'
```

## 옵션

| 옵션 | 설명 |
|---|---|
| `-o, --out DIR` | 출력 루트. 기본값 `./downloads` |
| `-n, --limit N` | 최대 N개 (이미 받아 건너뛴 항목은 세지 않음. 0 = 무제한) |
| `--since YYYY-MM-DD` | 이 날짜 이후에 게시된 항목만 |
| `--ids A,B` | 지정한 동영상 ID만 (`video_id` / `aweme_id`) |
| `--dry-run` | 받을 대상을 출력 |
| `--force` | 이미 기록된 항목도 다시 받음 |
| `--interval DUR` | 항목 사이 대기. 기본값 `3s` |
| `--platform douyin\|tiktok\|bilibili\|youtube` | 플랫폼 판정을 덮어씀 |
| `--max-height N` | YouTube 최대 높이 (기본 1080. 예: 1440, 2160) |
| `--cookies FILE` | 로그인 cookie (Netscape / JSON / `k=v; k=v`) |
| `--progress` | stderr에 `PROGRESS <0-100>` 줄을 씀 |

## 출력

```
downloads/
  douyin/<author>/<video_id>_<title>.mp4|.jpg|.json
  tiktok/<author>/<video_id>_<title>.mp4|.jpg|.json
  bilibili/<author>/<video_id>_<title>.mp4|.jpg|.json
  youtube/<author>/<video_id>_<title>.mp4|.jpg|.json
  .state.json            증분 원장 (키 = platform:id)
```

`.json`은 원래 타임라인 항목입니다. `play_url`은 만료될 수 있어 참고용으로만 남깁니다.

## 구현 메모

- **Douyin**: `play_url`을 `Referer: https://www.douyin.com/`와 함께 직접 받습니다.
- **TikTok**: CDN URL은 서명한 세션(`ttwid`/`msToken`/`tt_chain_token`)에 묶입니다. 일괄 다운로드는 현재 세션으로 각 동영상 URL을 다시 해석합니다. 다른 cookie jar로 업스트림 URL을 쓰면 403입니다.
- TikTok WAF는 표준 라이브러리로 푸는 SHA-256 작업 증명입니다. 브라우저는 필요 없습니다.
- **화질 선택**: 각 rendition의 `UrlKey`(예: `…_bytevc1_1080p_1037801`)가 실제 코덱과 해상도입니다. 해상도가 우선이고, 같으면 비트레이트입니다. 받은 화질을 출력합니다(예: `1080p bytevc1 722kbps`).
- **Bilibili**: `view`가 cid, `playurl`이 DASH 스트림, `ffmpeg -c copy`가 합칩니다. Akamai URL은 미러 호스트를 먼저 시도하고, 원래 CDN으로 돌아갑니다.
- **YouTube**: 내장 InnerTube 추출기가 VISIONOS 클라이언트로 player response를 요청합니다. 이 클라이언트는 서명 암호 없는 직접 adaptive stream URL을 줍니다. `--max-height` 이하에서 가장 높은 영상(AVC/MP4 우선)을 가장 좋은 MP4 음성과 맞추고 `ffmpeg -c copy`로 합칩니다.

## 제한

- **Douyin 단일 동영상 URL은 지원하지 않습니다**: 상세 엔드포인트는 브라우저 서명(`a_bogus`)이 필요하고, 순수 HTTP로는 워터마크 없는 스트림이나 메타데이터를 얻을 수 없습니다. 하나만 받으려면 `download --ids <video_id>`를 쓰거나 타임라인 JSON에서 항목을 읽으세요.
- TikTok 이미지 게시물에는 `play_url`이 없어 건너뜁니다(커버는 타임라인에서 받을 수 있습니다).
- **YouTube** 기본값은 1080p입니다. 더 높게는 `--max-height 1440` 또는 `--max-height 2160`(파일이 수백 MB일 수 있습니다). 연령 제한, DRM, 지역 차단 동영상은 cookie가 필요하거나 받지 못할 수 있습니다. 재생목록과 채널 피드는 펼치지 않습니다. 단일 동영상 URL을 넘기세요.
- 비공개이거나 지역 제한된 콘텐츠는 플랫폼이 오류로 알립니다. 타임라인 데이터에 없는 것은 이 도구도 보지 못합니다.
- 타임라인 수집, 서명, 로그인 상태는 호출자 책임입니다. 이 도구는 JSON만 소비합니다.

## 문서

영어판이 유일한 기준입니다. 이 페이지는 그 한국어 번역입니다.

- **[English](README.md)**: 영어 원문. 유일한 기준.
- **[AGENTS.md](AGENTS.md)**: 저장소 제약. 영어 전용 규칙과 병합 정책을 포함합니다.

## License

[MIT](LICENSE)
