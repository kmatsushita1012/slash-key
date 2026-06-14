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

### GET /path?q=<query>

`query` に一致する path を返します。空 query なら全 path を返します。

```bash
curl "http://localhost:4821/path?q=src"
```

## Shell API

### slash-key list

登録済み project の root path を出力します。

```bash
slash-key list
```

### slash-key path [query]

index 化された path を検索します。

```bash
slash-key path src
```

## Notes

- `slash-key start -e` は VPN interface 上の安全なアドレスのみを自動選択して bind します
- `0.0.0.0` / `::` や LAN IP への bind は拒否されます
- `SLASH_KEY_LISTEN_ADDR` も loopback または VPN interface 上の実アドレスだけ許可されます
