# slash-key design v0.2

## 1. Overview

### 設計目的

`slash-key` の v0.2 仕様を、実装に着手できる粒度の技術設計へ落とし込む。
対象はローカル専用の CLI + daemon + localhost API であり、キーボード補完システムから高速に path 検索できることを最優先とする。

### 設計スコープ

- project registry の管理
- project 配下 path の index 構築
- daemon の起動・停止・状態管理
- localhost API による project/path 参照
- CLI からの project 操作と検索

### スコープ外

- cloud sync
- authentication
- remote indexing
- semantic search
- AI search
- realtime file watching
- sqlite 永続化

### 前提

- 単一ユーザーのローカル PC 上で動作する
- daemon と CLI は同一マシン上の同一データ領域を参照する
- 実装言語は Go を採用する
- data directory は `~/.slash-key/` とする
- path 検索結果は project root 基準の relative path を返す
- `.gitignore` されている path は Git 本家にできるだけ忠実に解釈して index 対象外とする

## 2. Goals

- `slash-key path <query>` と `GET /path?q=...` が低レイテンシで応答できる
- daemon 起動中に `add` / `delete` / `add --codex` を実行した場合、検索結果へ即時反映される
- 失敗理由が CLI と API の両方で明確に分かる
- 初期実装は単純さを優先し、後から fuzzy search や incremental indexing を足せる構成にする
- `add --codex` は優先順位を下げ、コア機能の後で実装できる構成にする

## 3. Architecture

### 全体構成

```text
User / Keyboard Integration
        |
        v
      CLI --------------------+
        |                     |
        | control / query     | local files
        v                     v
     Daemon ----------> Project Registry Store
        |                     |
        | query               v
        +--------------> Path Index Store
        |
        v
   Localhost HTTP API
```

### モジュール分割

```text
slash-key
├── cmd/slash-key
├── internal/cli
├── internal/application
│   ├── project
│   ├── daemon
│   ├── index
│   └── search
├── internal/domain
│   ├── project
│   ├── pathentry
│   ├── daemonstate
│   └── errors
├── internal/infra
│   ├── registry
│   ├── indexstore
│   ├── scanner
│   ├── codexfinder
│   ├── process
│   └── httpserver
└── internal/runtime
    ├── config
    └── paths
```

### 依存方向

- `cli` は `application` のみを呼ぶ
- `http server` は `application` のみを呼ぶ
- `application` は `domain` と `infra` interface に依存する
- `infra` は filesystem / process / network を担当する

## 4. Runtime Data Layout

### データ保存方針

v0.2 では `~/.slash-key/` を単一のローカル data directory とする。保存フォーマットは人間が読める JSON を基本とする。

```text
~/.slash-key/
├── registry.json
├── daemon.json
├── indexes/
│   ├── <projectId>.json
│   └── ...
└── logs/
    └── daemon.log
```

### ファイル責務

- `registry.json`
  - 登録済み project 一覧
  - projectId と absolute root path の対応
- `daemon.json`
  - daemon pid
  - bind port
  - 起動時刻
  - data version
- `indexes/<projectId>.json`
  - project ごとの index 済み relative path 一覧
  - build timestamp
  - project root の fingerprint

### projectId

- absolute path を正規化した値から安定生成する
- directory rename や symlink 差異に備え、保存前に realpath ベースで正規化する

## 5. Domain Model

### Project

```text
Project
- id: string
- rootPath: absolute path
- displayName: basename(rootPath)
- createdAt: timestamp
```

### PathEntry

```text
PathEntry
- projectId: string
- relativePath: string
- kind: file | directory
- segments: string[]
- basename: string
```

### DaemonState

```text
DaemonState
- pid: number
- port: number
- status: running | stopped | stale
- startedAt: timestamp
```

### Error Model

- `ProjectNotFound`
- `ProjectAlreadyExists`
- `InvalidDirectory`
- `DaemonNotRunning`
- `PortBindFailed`
- `IndexBuildFailed`
- `RegistryLoadFailed`

## 6. CLI Design

### コマンド一覧

- `slash-key start` / `slash-key start -e`
- `slash-key stop`
- `slash-key status`
- `slash-key list`
- `slash-key add <dirPath>`
- `slash-key add --codex`
- `slash-key delete <dirPath>`
- `slash-key path [query]`

### CLI 共通ルール

- 正常系は人が読みやすい text 出力
- 異常系は 1 行目に要約、2 行目以降に詳細理由
- exit code は `0=success`, `1=usage/domain error`, `2=runtime/system error`

### 各コマンドの責務

#### `start`

- `registry.json` を読み込む
- 全 project index をメモリへロードする
- port `4821` に bind する
- PID 情報を `daemon.json` に保存する
- `start` は `127.0.0.1:4821`
- `start -e` は `0.0.0.0:4821`
- `SLASH_KEY_LISTEN_ADDR` が指定されていればそれを優先する

#### `stop`

- `daemon.json` の pid を参照して daemon を停止する
- stop 成功後は `daemon.json` を削除または `stopped` 扱いに更新する

#### `status`

- `daemon.json` の pid 存在確認
- process 生存確認
- 生存していれば `running`
- metadata はあるが process が無ければ `stale`

#### `list`

- registry にある project の absolute path を登録順で出力する

#### `add <dirPath>`

- path existence check
- directory check
- path normalization
- duplicate check
- registry 保存
- index rebuild
- daemon 起動中なら reload 実行

#### `add --codex`

- v0.2 では後回しの低優先機能として扱う
- Codex.app で表示されている project を探索対象とする
- 既存 registry と重複しない project のみ追加
- 追加対象が 1 件以上あれば index rebuild
- daemon 起動中なら reload 実行

#### `delete <dirPath>`

- path normalization
- registry から削除
- 対応 index file を削除
- daemon 起動中なら reload 実行

#### `path [query]`

- daemon 起動中なら HTTP API を優先利用
- daemon 未起動時は local index file を直接読んで検索してもよい
  - ただし実装を単純にするなら v0.2 では daemon 必須に寄せてもよい
- 出力は relative path のみ

## 7. Daemon Design

### daemon の責務

- index をメモリロードする
- HTTP API を提供する
- registry 変更時に index を再読み込みする
- server state を保持する

### プロセスモデル

- v0.2 は単一プロセス、単一ポート
- 複数 daemon の同時起動は禁止
- 二重起動検知時は既存 daemon を優先し、新規起動は失敗させる

### reload 方針

- trigger: `add`, `delete`, `add --codex`
- 最初の実装は full rebuild + full reload でよい
- reload 中は検索を止めず、旧 index を返し続ける
- 新 index 完成後にメモリ参照をアトミックに差し替える

### server state 遷移

```text
stopped
  -> starting
  -> running
  -> reloading
  -> running
  -> stopping
  -> stopped
```

## 8. Indexing Design

### index 対象

- file
- directory

### 除外ルール

固定除外:

- `.git`
- `node_modules`
- `dist`
- `build`
- `.next`
- `coverage`

追加除外:

- project ごとの `.gitignore` に一致する path
- 親 directory から継承される ignore rule
- negation pattern を含む Git 本家挙動に準拠する rule

### `.gitignore` 解釈方針

- 独自実装は避け、Git 本家挙動に近い既存 Go ライブラリを利用する
- 最低限、以下をテストで担保する
  - directory 単位 ignore
  - glob pattern
  - nested `.gitignore`
  - `!` による再 include
  - root 基準 pattern と相対 pattern の差異

### path 正規化ルール

- project root からの relative path へ変換する
- separator は `/` に統一する
- directory も file と同様に `./` 起点で出力する
- ルート自身は index 対象に含めない

### index フォーマット

```json
{
  "projectId": "proj_xxx",
  "rootPath": "/Users/foo/workspace/app",
  "builtAt": "2026-06-11T12:00:00Z",
  "entries": [
    {
      "relativePath": "./src",
      "kind": "directory",
      "basename": "src",
      "segments": ["src"]
    },
    {
      "relativePath": "./src/main.ts",
      "kind": "file",
      "basename": "main.ts",
      "segments": ["src", "main.ts"]
    }
  ]
}
```

### build 手順

1. project root を走査する
2. 除外 path をスキップする
3. directory / file を `PathEntry` に変換する
4. basename 検索用に軽量なメモリ index を生成する
5. 永続化して daemon メモリへ反映する

## 9. Search Design

### 要求仕様

- query が空なら全 path を返す
- query が `fuga` の場合、basename または path 断片に `fuga` を含む path を返す
- file / directory の両方を返す

### v0.2 の検索アルゴリズム

- 大文字小文字は OS 依存にしないため case-insensitive 検索を採用する
- `normalizedQuery` を lowercase 化する
- 以下の順で match と ranking を行う

1. basename 完全一致
2. basename 前方一致
3. path segment 前方一致
4. path 全体部分一致

### ソート順

同順位内では以下を優先する:

1. segment 数が少ない path
2. 文字列長が短い path
3. 辞書順

### レスポンス件数

- v0.2 では上限なし
- 将来の editor integration を考慮し、内部実装では `limit` を渡せる構造にしておく

## 10. HTTP API Design

### Base URL

`http://localhost:4821`

### `GET /list`

response:

```json
[
  "/Users/foo/workspace/app",
  "/Users/foo/dev/tools"
]
```

### `GET /path`

query params:

- `q`: optional

response:

```json
[
  "./fuga",
  "./scripts/fuga",
  "./src/fuga.ts"
]
```

### API 共通ルール

- content-type は `application/json`
- 正常時は `200`
- 不正 request は `400`
- daemon 内部エラーは `500`

### エラーレスポンス

```json
{
  "error": {
    "code": "INDEX_BUILD_FAILED",
    "message": "failed to load index"
  }
}
```

## 11. Application Flow

### 初回セットアップ

```text
1. User runs `slash-key add <dirPath>`
2. registry.json に project を保存
3. indexes/<projectId>.json を生成
4. User runs `slash-key start`
5. daemon が registry / index をロード
6. keyboard integration から API を利用開始
```

### `add --codex` の扱い

```text
1. v0.2 のコア実装対象からは外す
2. interface と差し込みポイントだけを先に用意する
3. 後続で Codex.app project source を実装する
```

### project 追加時

```text
1. CLI が path を正規化
2. registry 更新
3. index rebuild
4. daemon が running なら reload
5. 次回検索から新 project を返す
```

### path 検索時

```text
1. CLI or keyboard integration が query を送る
2. daemon が memory index を検索
3. ranking
4. relative path array を返却
```

## 12. Failure Handling

### daemon 起動失敗

- port 使用中なら起動失敗
- `daemon.json` の stale 情報は自動清掃する

### index build 失敗

- 失敗した project の index は更新しない
- 既存 index があれば継続利用する
- CLI は失敗 project と理由を表示する

### registry 破損

- JSON parse error は `RegistryLoadFailed`
- 自動修復は行わない
- バックアップまたは手動修正を促す

## 13. Logging and Observability

### ログ出力

- daemon lifecycle
- registry 更新
- index build 開始/終了
- reload 開始/終了
- HTTP request の概要
- error stack

### ログレベル

- `info`
- `warn`
- `error`

## 14. Security and Safety

- localhost bind のみ
- 外部ネットワーク公開をしない
- project root の absolute path は `/list` のみで返し、`/path` は relative path に限定する
- path traversal 文字列を受け取っても filesystem を直接読む API は提供しない

## 15. Testing Strategy

### Unit Test

- path normalization
- duplicate project detection
- ignore 判定
- `.gitignore` 準拠の nested / negation pattern
- ranking
- stale daemon 判定

### Integration Test

- `add -> start -> /list -> /path`
- `delete` 後に検索結果から消えること
- daemon reload 後に新 index が反映されること

### Deferred Test

- `add --codex` の追加挙動

### End-to-End Test

- 実際の project directory を fixture として作成し、CLI 出力と API 応答を確認する

## 16. Implementation Order

1. runtime data directory と config 解決
2. registry store
3. file scanner + ignore 判定
4. index builder + search engine
5. CLI `add/list/delete/path`
6. daemon `start/status/stop`
7. HTTP API
8. daemon reload
9. `add --codex`

## 17. Open Questions

- `slash-key path` は daemon 必須にするか、未起動でも local index を読めるようにするか
- `.gitignore` 準拠を Go でどのライブラリに寄せるか
- `add --codex` で Codex.app の表示 project をどの手段で取得するか
