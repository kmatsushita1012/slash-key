# slash-key

`slash-key` は、登録済みプロジェクトディレクトリに対して `/` 風のパス補完を高速に行うローカル CLI と daemon です。

## Install

### Homebrew tap

```bash
brew tap kmatsushita1012/tap
brew install slash-key
```

## Run Over VPN

```bash
slash-key start -e
```

- VPN 上の安全なアドレスを自動検出して bind します
- VPN アドレスを明示したい場合は次も使えます

```bash
slash-key start -e 100.x.x.x
```

- 出力例:

```text
slash-key server started
local: http://localhost:4821
network: http://100.x.x.x:4821
```

- `0.0.0.0` / `::` や LAN IP (`192.168.x.x`, `10.x.x.x`, `172.16-31.x.x`) への bind は拒否されます
- `http://<vpn-ip>:4821` から daemon にアクセスします
- host firewall と VPN policy で TCP `4821` の inbound を許可してください
- `SLASH_KEY_LISTEN_ADDR=...` も loopback または VPN interface 上の実アドレスだけ許可されます

## ローカル起動

```bash
slash-key start
```

## ビルド

ローカルでコンパイルするには、リポジトリ直下で次を実行します。

```bash
go build -o slash-key ./cmd/slash-key
```

テストも含めて確認するなら、こちらです。

```bash
go test ./...
```

## 補足

- 初期配布は Homebrew タップ経由で提供しています。
- 将来的には tap を使わず、Homebrew core に upstream する形を目指しています。
