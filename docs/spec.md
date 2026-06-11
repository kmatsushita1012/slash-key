# slash-key 仕様書 v0.2

## 概要

`slash-key` は、ローカルPC上のプロジェクトディレクトリを管理し、  
高速なファイルパス補完を提供するためのローカルAPI/CLIツールである。

主用途は、キーボード入力時の `/` ベースのファイルパス補完。

---

# コンセプト

- ローカル環境専用
- 指定 directory を project として管理
- project 配下の file/dir path を index 化
- localhost API 経由で高速検索
- slash-key キーボード補完システムから利用

---

# 用語

| 用語 | 説明 |
|---|---|
| Project | 登録された root directory |
| Index | file/dir path の検索用データ |
| Query | path 検索文字列 |
| Daemon | localhost API server |

---

# CLI仕様

---

# daemon 起動

```bash
slash-key start
```

## 説明

localhost API server を起動する。

## 動作

- project index をロード
- API server 起動
- port bind
- background process 化可能

## 出力例

```text
slash-key server started
http://localhost:4821
```

---

# daemon 停止

```bash
slash-key stop
```

## 説明

起動中の localhost API server を停止する。

---

# daemon status

```bash
slash-key status
```

## 説明

server の起動状態を表示する。

## 出力例

```text
running
http://localhost:4821
projects: 12
```

---

# project 一覧

```bash
slash-key list
```

## 説明

登録済み project 一覧を表示する。

---

# project 追加

```bash
slash-key add <dirPath>
```

## 説明

directory を project として登録する。

## 動作

- path existence check
- directory check
- duplicate check
- index 更新

## daemon 再起動

daemon が起動中の場合:

- server を自動 reload
- index を再構築

### 理由

検索結果を即時反映するため。

---

# project 削除

```bash
slash-key delete <dirPath>
```

## 説明

登録済み project を削除する。

## 動作

- registry から削除
- index 削除

## daemon 再起動

daemon 起動中なら自動 reload。

---

# Codex project 一括追加

```bash
slash-key add --codex
```

## 説明

Codex 管理下 project を自動検出して追加する。

## daemon

起動中なら自動 reload。

---

# path 検索

```bash
slash-key path <query>
```

## 説明

index 化された filepath / dirpath を検索する。

---

# 検索仕様

## filename match

```text
query: fuga
```

match:

```text
./fuga
./scripts/fuga
./src/fuga.ts
```

---

## 空 query

```bash
slash-key path
```

または:

```bash
slash-key path ""
```

の場合:

全 path を返す。

## フィルター

.gitignoreされているファイルは対象から外す

---

# path 出力形式

```text
./src/components/button.tsx
./scripts/deploy.sh
./README.md
```

## format

- relative path
- project root 基準

---

# Localhost API仕様

## Base URL

```text
http://localhost:4821
```

---

# API 起動条件

API は `slash-key start` 実行後のみ利用可能。

---

# project list API

```http
GET /list
```

## response

```json
[
  "/Users/foo/workspace/app",
  "/Users/foo/dev/tools"
]
```

---

# path search API

```http
GET /path?query=fuga
```

## response

```json
[
  "./fuga",
  "./scripts/fuga",
  "./src/fuga.ts"
]
```

---

# 空 query

```http
GET /path?query=
```

または:

```http
GET /path
```

で全 path を返す。

---

# Index仕様

## index 対象

- file
- directory

---

# 除外候補

```text
node_modules
.git
dist
build
.next
coverage
```

---

# daemon reload仕様

## reload trigger

以下実行時:

- add
- delete
- add --codex

## reload 動作

- index rebuild
- in-memory cache refresh

## 備考

server process 自体を kill/start してもよいし、
hot reload 実装でもよい。

---

# 将来拡張

- fuzzy search
- realtime file watching
- sqlite index
- ranking
- recent path boost
- editor integration
- shell integration
- websocket update
- incremental indexing

---

# 非目的

v0.2 時点では対象外:

- cloud sync
- authentication
- remote indexing
- semantic search
- AI search

---

# 実装イメージ

```text
slash-key
 ├── CLI
 ├── daemon
 ├── localhost API
 ├── project registry
 ├── path index
 └── keyboard integration
```