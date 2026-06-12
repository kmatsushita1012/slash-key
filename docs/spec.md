# slash-key 仕様書 v0.2

## 1. 概要

`slash-key` は、ローカル PC 上の複数 project directory を登録し、`/` ベースのファイルパス補完を高速に返すための CLI/daemon ツールである。

主用途は、キーボード入力時の path 補完と、ローカルの localhost API を通じた高速検索である。

## 2. コンセプト

- ローカル専用で動作する
- project root を登録して配下の file / directory を index 化する
- daemon が検索と API 応答を担当する
- CLI は project 操作と daemon 制御を担当する
- 検索結果は project root 基準の relative path で返す

## 3. 用語

| 用語 | 説明 |
|---|---|
| Project | 登録済みの root directory |
| Registry | 登録済み project の一覧と metadata |
| Index | file / directory path の検索用データ |
| Query | path 検索文字列 |
| Daemon | localhost API server |

## 4. 基本方針

- 実装言語は Go とする
- 保存先の data directory は `~/.slash-key/` とする
- 永続化形式は JSON とする
- `.gitignore` は Git 本家にできるだけ忠実な挙動を目指して除外する
- 既定の待受は `127.0.0.1:4821` とする
- 外部公開が必要な場合は `slash-key start -e` で `0.0.0.0:4821` に bind する
- 必要に応じて `SLASH_KEY_LISTEN_ADDR` で bind address を上書きできる
- `SLASH_KEY_DATA_DIR` で data directory を上書きできる

## 5. データ配置

`~/.slash-key/` の構成は次のとおりとする。

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

### 5.1 `registry.json`

- 登録済み project の一覧を保持する
- projectId, rootPath, displayName, createdAt を含む

### 5.2 `daemon.json`

- daemon の pid を保持する
- bind port を保持する
- status を保持する
- startedAt を保持する
- dataVersion を保持する

### 5.3 `indexes/<projectId>.json`

- 1 project につき 1 index file を持つ
- file / directory の relative path 一覧を保持する
- index build 時刻を保持する

## 6. CLI 仕様

### 6.1 `slash-key start`

daemon を起動する。

#### 動作

- registry を読み込む
- project index をロードする
- API server を起動する
- port を bind する
- daemon を background process として起動する

#### 起動モード

- `slash-key start`
  - `127.0.0.1:4821` で待受する
- `slash-key start -e`
  - `0.0.0.0:4821` で待受する
- `SLASH_KEY_LISTEN_ADDR`
  - 指定されている場合は bind address として使用できる
- `SLASH_KEY_DATA_DIR`
  - 指定されている場合は data directory として使用できる

#### 出力例

```text
slash-key server started
http://localhost:4821
```

#### 注意

- `-e` は外部公開を許可する起動オプションである
- 外部からアクセスする場合は host firewall と VPN policy の許可が必要である

### 6.2 `slash-key stop`

起動中の daemon を停止する。

#### 出力例

```text
slash-key server stopped
```

### 6.3 `slash-key status`

daemon の起動状態を表示する。

#### 状態

- `running`
- `stopped`
- `stale`

#### 出力例

```text
running
http://localhost:4821
projects: 12
```

### 6.4 `slash-key list`

登録済み project 一覧を表示する。

#### 出力

- 1 行につき 1 project root path
- absolute path を表示する

### 6.5 `slash-key add <dirPath>`

directory を project として登録する。

#### 動作

- path existence check
- directory check
- duplicate check
- registry 更新
- index 更新
- daemon 起動中なら reload

### 6.6 `slash-key add --codex`

Codex 管理下 project を自動検出して追加する。

#### 方針

- v0.2 では優先度を下げる
- 将来の拡張対象として予約する

### 6.7 `slash-key delete <dirPath>`

登録済み project を削除する。

#### 動作

- registry から削除する
- 対応する index file を削除する
- daemon 起動中なら reload する

### 6.8 `slash-key path [query]`

index 化された path を検索する。

#### 入力

- `query` は省略可能
- `slash-key path` と `slash-key path ""` は空 query として扱う

#### 出力

- `./` から始まる relative path を出力する
- 1 行につき 1 path
- daemon が起動している場合は daemon の API を使う
- daemon が起動していない場合は local index file を直接読んで検索してよい

## 7. path 検索仕様

### 7.1 対象

- file
- directory

### 7.2 空 query

空 query の場合は全 path を返す。

### 7.3 マッチ条件

`query: fuga` に対して、以下に一致する path を返す。

- basename 完全一致
- basename 前方一致
- path segment 前方一致
- path 全体部分一致

### 7.4 ranking

同順位内では次の順で優先する。

1. segment 数が少ない path
2. 文字列長が短い path
3. 辞書順

### 7.5 大文字小文字

case-insensitive で検索する。

## 8. index 仕様

### 8.1 index 対象

- file
- directory

### 8.2 除外対象

固定除外:

- `.git`
- `node_modules`
- `dist`
- `build`
- `.next`
- `coverage`

追加除外:

- project ごとの `.gitignore`
- parent directory から継承される ignore rule
- negation pattern を含む Git の ignore 挙動

### 8.3 path 正規化

- project root からの relative path に変換する
- separator は `/` に統一する
- 出力は `./` で始める
- ルート自身は index に含めない

### 8.4 index 更新

- `add` / `delete` / `add --codex` 実行時に更新する
- daemon 起動中なら reload して即時反映する

## 9. Localhost API 仕様

### 9.1 Base URL

```text
http://localhost:4821
```

### 9.2 起動条件

- API は `slash-key start` または `slash-key start -e` 実行後のみ利用可能

### 9.3 `GET /list`

登録済み project root の一覧を返す。

#### response

```json
[
  "/Users/foo/workspace/app",
  "/Users/foo/dev/tools"
]
```

### 9.4 `GET /path`

path 検索を行う。

#### query parameter

- `q`
  - primary parameter
- `query`
  - backward-compatible parameter

#### response

```json
[
  "./fuga",
  "./scripts/fuga",
  "./src/fuga.ts"
]
```

#### 空 query

```http
GET /path
GET /path?q=
GET /path?query=
```

### 9.5 内部 API

以下は daemon 内部で使う非公開 API である。

- `GET /health`
- `POST /reload`
- `POST /shutdown`

## 10. エラー仕様

### 10.1 exit code

- `0`
  - success
- `1`
  - usage error / domain error
- `2`
  - runtime error / system error

### 10.2 代表的なエラー

- `ProjectNotFound`
- `ProjectAlreadyExists`
- `InvalidDirectory`
- `DaemonNotRunning`
- `PortBindFailed`
- `IndexBuildFailed`
- `RegistryLoadFailed`

### 10.3 CLI 表示

- 正常系は human-readable な text を出力する
- 異常系は短い要約を 1 行目に出力する
- 追加詳細は必要に応じて 2 行目以降に出力する

## 11. daemon 仕様

### 11.1 責務

- index をメモリロードする
- HTTP API を提供する
- registry 更新後に index を再読み込みする
- server state を保持する

### 11.2 プロセスモデル

- 単一プロセス、単一ポートで動作する
- 複数 daemon の同時起動はしない

### 11.3 reload

- `add`
- `delete`
- `add --codex`

上記で index rebuild と in-memory cache refresh を行う。

### 11.4 状態遷移

```text
stopped -> starting -> running -> reloading -> running -> stopping -> stopped
```

## 12. セキュリティ・安全性

- 既定では localhost のみで bind する
- 外部公開は `start -e` または `SLASH_KEY_LISTEN_ADDR` の明示時のみ行う
- `/path` は relative path のみ返す
- path traversal を受けても filesystem を直接公開しない

## 13. テスト方針

### 13.1 Unit

- path normalization
- duplicate project detection
- `.gitignore` 除外
- ranking
- daemon state 判定

### 13.2 Integration

- `add -> start -> /list -> /path`
- `delete` 後に検索結果から消えること
- reload 後に新 index が反映されること

### 13.3 Deferred

- `add --codex`

## 14. 非目的

v0.2 時点では対象外とする。

- cloud sync
- authentication
- remote indexing
- semantic search
- AI search
- realtime file watching
- sqlite 永続化

## 15. 将来拡張

- fuzzy search
- ranking の強化
- recent path boost
- editor integration
- shell integration
- websocket update
- incremental indexing
- file watching

## 16. 実装イメージ

```text
slash-key
 ├── CLI
 ├── daemon
 ├── localhost API
 ├── project registry
 ├── path index
 └── keyboard integration
```
