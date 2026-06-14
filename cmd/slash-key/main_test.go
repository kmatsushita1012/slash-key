package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestMatchEntryRanking(t *testing.T) {
	entry := pathEntry{
		RelativePath: "./src/fuga.ts",
		Basename:     "fuga.ts",
		Segments:     []string{"src", "fuga.ts"},
	}

	if score, ok := matchEntry(entry, "fuga.ts"); !ok || score != 0 {
		t.Fatalf("expected exact basename match, got score=%d ok=%v", score, ok)
	}
	if score, ok := matchEntry(entry, "fuga"); !ok || score != 1 {
		t.Fatalf("expected prefix basename match, got score=%d ok=%v", score, ok)
	}
	if score, ok := matchEntry(entry, "sr"); !ok || score != 2 {
		t.Fatalf("expected segment prefix match, got score=%d ok=%v", score, ok)
	}
	if score, ok := matchEntry(entry, "uga.t"); !ok || score != 3 {
		t.Fatalf("expected substring match, got score=%d ok=%v", score, ok)
	}
}

func TestSearchIndexesSortOrder(t *testing.T) {
	indexes := []indexFile{{
		Entries: []pathEntry{
			{RelativePath: "./src/fuga.ts", Basename: "fuga.ts", Segments: []string{"src", "fuga.ts"}},
			{RelativePath: "./fuga", Basename: "fuga", Segments: []string{"fuga"}},
			{RelativePath: "./scripts/fuga", Basename: "fuga", Segments: []string{"scripts", "fuga"}},
		},
	}}

	results := searchIndexes(indexes, "fuga")
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	if results[0] != "./fuga" {
		t.Fatalf("expected shortest exact/prefix match first, got %s", results[0])
	}
}

func TestGitIgnoredPaths(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".gitignore"), "ignored/\n*.log\n!important.log\n")
	mustMkdir(t, filepath.Join(root, "ignored"))
	mustWriteFile(t, filepath.Join(root, "ignored", "a.txt"), "x")
	mustWriteFile(t, filepath.Join(root, "debug.log"), "x")
	mustWriteFile(t, filepath.Join(root, "important.log"), "x")

	paths := []string{
		filepath.Join(root, "ignored"),
		filepath.Join(root, "ignored", "a.txt"),
		filepath.Join(root, "debug.log"),
		filepath.Join(root, "important.log"),
	}
	ignored, err := gitIgnoredPaths(root, paths)
	if err != nil {
		t.Fatalf("gitIgnoredPaths error: %v", err)
	}

	if !ignored[filepath.Join(root, "ignored")] {
		t.Fatal("expected ignored directory to be ignored")
	}
	if !ignored[filepath.Join(root, "debug.log")] {
		t.Fatal("expected debug.log to be ignored")
	}
	if ignored[filepath.Join(root, "important.log")] {
		t.Fatal("expected important.log to be re-included")
	}
}

func TestValidateServeListenAddr(t *testing.T) {
	withVPNInterfaceAddrs(t, []net.IP{net.ParseIP("100.64.0.10")})

	cases := []struct {
		name       string
		listenAddr string
		want       string
		wantErr    bool
	}{
		{name: "loopback", listenAddr: "127.0.0.1:4821", want: "127.0.0.1:4821"},
		{name: "localhost", listenAddr: "localhost:4821", want: "127.0.0.1:4821"},
		{name: "vpn", listenAddr: "100.64.0.10:4821", want: "100.64.0.10:4821"},
		{name: "wildcard ipv4", listenAddr: "0.0.0.0:4821", wantErr: true},
		{name: "wildcard ipv6", listenAddr: "[::]:4821", wantErr: true},
		{name: "lan", listenAddr: "192.168.0.10:4821", wantErr: true},
		{name: "wrong port", listenAddr: "100.64.0.10:8080", wantErr: true},
		{name: "non vpn public", listenAddr: "8.8.8.8:4821", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateServeListenAddr(tc.listenAddr)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("validateServeListenAddr(%q) expected error", tc.listenAddr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateServeListenAddr(%q) error: %v", tc.listenAddr, err)
			}
			if got != tc.want {
				t.Fatalf("validateServeListenAddr(%q) = %q, want %q", tc.listenAddr, got, tc.want)
			}
		})
	}
}

func TestResolveStartListenAddr(t *testing.T) {
	withVPNInterfaceAddrs(t, []net.IP{net.ParseIP("100.64.0.10")})

	if got, err := resolveStartListenAddr(nil); err != nil || got != "" {
		t.Fatalf("resolveStartListenAddr(nil) = %q, %v", got, err)
	}
	if got, err := resolveStartListenAddr([]string{"-e"}); err != nil || got != "100.64.0.10:4821" {
		t.Fatalf("resolveStartListenAddr([-e]) = %q, %v", got, err)
	}
	if got, err := resolveStartListenAddr([]string{"-e", "100.64.0.10"}); err != nil || got != "100.64.0.10:4821" {
		t.Fatalf("resolveStartListenAddr([-e 100.64.0.10]) = %q, %v", got, err)
	}
	if _, err := resolveStartListenAddr([]string{"-e", "192.168.0.10"}); err == nil {
		t.Fatal("expected explicit LAN IP to be rejected")
	}
	if _, err := resolveStartListenAddr([]string{"--bad"}); err == nil {
		t.Fatal("expected invalid usage to be rejected")
	}
}

func TestResolveStartListenAddrWithoutVPN(t *testing.T) {
	withVPNInterfaceAddrs(t, nil)

	if _, err := resolveStartListenAddr([]string{"-e"}); err == nil {
		t.Fatal("expected -e without VPN address to fail")
	}
}

func TestStartupURLLines(t *testing.T) {
	lines := startupURLLines("100.64.0.10:4821")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %#v", len(lines), lines)
	}
	if lines[0] != "local: http://localhost:4821" {
		t.Fatalf("unexpected local line: %q", lines[0])
	}
	if lines[1] != "network: http://100.64.0.10:4821" {
		t.Fatalf("unexpected network line: %q", lines[1])
	}

	lines = startupURLLines("127.0.0.1:4821")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line for local bind, got %d: %#v", len(lines), lines)
	}
	if lines[0] != "local: http://localhost:4821" {
		t.Fatalf("unexpected local-only line: %q", lines[0])
	}
}

func TestDaemonBaseURL(t *testing.T) {
	cases := []struct {
		name  string
		state daemonState
		want  string
	}{
		{name: "default loopback", state: daemonState{Port: 4821}, want: "http://127.0.0.1:4821"},
		{name: "vpn addr", state: daemonState{Port: 4821, ListenAddr: "100.64.0.10:4821"}, want: "http://100.64.0.10:4821"},
		{name: "localhost normalized", state: daemonState{Port: 4821, ListenAddr: "localhost:4821"}, want: "http://127.0.0.1:4821"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := daemonBaseURL(tc.state); got != tc.want {
				t.Fatalf("daemonBaseURL(%+v) = %q, want %q", tc.state, got, tc.want)
			}
		})
	}
}

func withVPNInterfaceAddrs(t *testing.T, addrs []net.IP) {
	t.Helper()
	orig := vpnInterfaceAddrs
	vpnInterfaceAddrs = func() ([]net.IP, error) {
		if addrs == nil {
			return nil, nil
		}
		out := make([]net.IP, len(addrs))
		copy(out, addrs)
		return out, nil
	}
	t.Cleanup(func() {
		vpnInterfaceAddrs = orig
	})
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
