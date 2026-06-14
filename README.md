# slash-key

`slash-key` is a local CLI and daemon for fast `/`-style path completion across registered project directories.

## Install

### Homebrew tap

```bash
brew tap kmatsushita1012/tap
brew install slash-key
```

## Run Locally

```bash
slash-key start
```

## Build

ローカルでコンパイルするには、リポジトリ直下で次を実行します。

```bash
go build -o slash-key ./cmd/slash-key
```

テストも含めて確認するなら、こちらです。

```bash
go test ./...
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

- Output example:

```text
slash-key server started
local: http://localhost:4821
network: http://100.x.x.x:4821
```

- `0.0.0.0` / `::` や LAN IP (`192.168.x.x`, `10.x.x.x`, `172.16-31.x.x`) への bind は拒否されます
- Then access the daemon from `http://<vpn-ip>:4821`
- The host firewall and VPN policy must allow inbound TCP `4821`
- `SLASH_KEY_LISTEN_ADDR=...` も loopback または VPN interface 上の実アドレスだけ許可されます

## Notes

- The initial Homebrew distribution is provided through the tap repo.
- The long-term goal is to make the formula available without a tap by upstreaming it to Homebrew core.
