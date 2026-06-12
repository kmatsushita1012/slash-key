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

## Run Over VPN or LAN

```bash
slash-key start -e
```

- Then access the daemon from `http://<machine-ip>:4821`
- The host firewall and VPN policy must allow inbound TCP `4821`
- If you need a custom bind address, `SLASH_KEY_LISTEN_ADDR=...` is still supported

## Notes

- The initial Homebrew distribution is provided through the tap repo.
- The long-term goal is to make the formula available without a tap by upstreaming it to Homebrew core.
