# slash-key

`slash-key` は、登録済みプロジェクトディレクトリに対して `/` 風のパス補完を高速に行うローカル CLI と daemon です。

## Quick Start

最短手順はこれです。

```bash
brew trust kmatsushita1012/tap
brew tap kmatsushita1012/tap
brew install slash-key

# プロジェクトを登録
slash-key add <project-root-path>

# 起動
slash-key start -e
```

- `-e` は VPN 上の安全なアドレスを自動検出して bind します
- 明示的にアドレスを指定する場合は `-e <IPアドレス>` と指定してください
- ローカルのみで使うなら `slash-key start` を利用してください

## Install

### Homebrew tap

```bash
brew trust kmatsushita1012/tap
brew tap kmatsushita1012/tap
brew install slash-key
```

## Projects

### Add a Project

```bash
slash-key add <project-root-path>
```

### List Projects

```bash
slash-key list
```

### Remove a Project

```bash
slash-key delete <project-root-path>
```

## Server

### Start Server

```bash
slash-key start
```

VPN 上で公開したい場合は次を使えます。

```bash
slash-key start -e
```

`-e` に IP を付けると、そのアドレスへ bind します。

```bash
slash-key start -e 100.x.x.x
```

### Server Status

```bash
slash-key status
```

### Stop Server

```bash
slash-key stop
```

## HTTP API

### GET /list

登録済み project の一覧を返します。

```bash
curl http://localhost:4821/list
```

```json
["MaTool", "slash-key"]
```

### GET /path?p=<project>&q=<path>

`project` に一致する登録済み project の path を返します。`project` は登録 root path の末尾名です。`q` を省略した場合はその project の全 path を返します。同名 project が複数ある場合は、後から登録された project を使います。

```bash
curl "http://localhost:4821/path?p=slash-key&q=src"
```

## Shell API

### slash-key list

登録済み project の末尾名を出力します。

```bash
slash-key list
```

### slash-key path p=<project> [q=<path>]

指定 project の index 化された path を検索します。

```bash
slash-key path p=slash-key q=src
```

## Notes

- `slash-key start -e` は VPN interface 上の安全なアドレスのみを自動選択して bind します
- `0.0.0.0` / `::` や LAN IP への bind は拒否されます
- `SLASH_KEY_LISTEN_ADDR` も loopback または VPN interface 上の実アドレスだけ許可されます

## Build

Go でローカルビルドする場合は、リポジトリ直下で次を実行してください。

```bash
go build -o slash-key ./cmd/slash-key
```

`go.mod` の指定 Go バージョンは `1.26.0` です。

## Release / Homebrew Update

このリポジトリから直接、Git tag の作成と Homebrew tap の更新をまとめて行えます。

1. `.env.example` を `.env` にコピーして `HOMEBREW_TAP_PATH` を設定します
2. リポジトリのルートで次を実行します

```bash
scripts/update.sh 1.0.0
```

- `HOMEBREW_TAP_PATH` は `homebrew-tap` リポジトリのローカル絶対パスです
- スクリプトは `v1.0.0` タグを作成して push し、tap 側の `Formula/slash-key.rb` を更新して commit/push します
- すでに tag が存在する場合は、その tag の作成はスキップします
