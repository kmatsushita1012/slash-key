package main

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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

func TestProjectNames(t *testing.T) {
	projects := []project{
		{DisplayName: "MaTool"},
		{DisplayName: "slash-key"},
	}

	got := projectNames(projects)
	want := []string{"MaTool", "slash-key"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("projectNames() = %#v, want %#v", got, want)
	}
}

func TestResolveProjectIndexUsesLastRegisteredDuplicate(t *testing.T) {
	projects := []project{
		{ID: "proj_old", DisplayName: "dup"},
		{ID: "proj_other", DisplayName: "other"},
		{ID: "proj_new", DisplayName: "dup"},
	}
	indexes := []indexFile{
		{ProjectID: "proj_old", Entries: []pathEntry{{RelativePath: "./old"}}},
		{ProjectID: "proj_other", Entries: []pathEntry{{RelativePath: "./other"}}},
		{ProjectID: "proj_new", Entries: []pathEntry{{RelativePath: "./new"}}},
	}

	got, err := resolveProjectIndex(projects, indexes, "dup")
	if err != nil {
		t.Fatalf("resolveProjectIndex error: %v", err)
	}
	if got.ProjectID != "proj_new" {
		t.Fatalf("resolveProjectIndex selected %q, want proj_new", got.ProjectID)
	}
}

func TestResolveProjectIndexNotFound(t *testing.T) {
	_, err := resolveProjectIndex(nil, nil, "missing")
	if err == nil {
		t.Fatal("expected error for missing project")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error")
	}
}

func TestParsePathArgs(t *testing.T) {
	projectName, query, err := parsePathArgs([]string{"p=slash-key", "q=src"})
	if err != nil {
		t.Fatalf("parsePathArgs error: %v", err)
	}
	if projectName != "slash-key" || query != "src" {
		t.Fatalf("parsePathArgs() = (%q, %q), want (%q, %q)", projectName, query, "slash-key", "src")
	}

	projectName, query, err = parsePathArgs([]string{"q=src", "p=slash-key"})
	if err != nil {
		t.Fatalf("parsePathArgs with reversed order error: %v", err)
	}
	if projectName != "slash-key" || query != "src" {
		t.Fatalf("parsePathArgs reversed order = (%q, %q), want (%q, %q)", projectName, query, "slash-key", "src")
	}
}

func TestParsePathArgsRequiresProject(t *testing.T) {
	cases := [][]string{
		nil,
		{"q=src"},
		{"src"},
		{"project=slash-key"},
	}

	for _, args := range cases {
		if _, _, err := parsePathArgs(args); err == nil {
			t.Fatalf("parsePathArgs(%#v) expected error", args)
		}
	}
}

func TestLocalSearchFiltersByProjectName(t *testing.T) {
	dataDir := t.TempDir()
	paths := appPaths{
		dataDir:     dataDir,
		registry:    filepath.Join(dataDir, "registry.json"),
		daemonState: filepath.Join(dataDir, "daemon.json"),
		indexDir:    filepath.Join(dataDir, "indexes"),
		logDir:      filepath.Join(dataDir, "logs"),
		logFile:     filepath.Join(dataDir, "logs", "daemon.log"),
	}
	if err := ensureDataDirs(paths); err != nil {
		t.Fatalf("ensureDataDirs error: %v", err)
	}

	registry := registryFile{
		Projects: []project{
			{ID: "proj_old", RootPath: "/tmp/old", DisplayName: "dup", CreatedAt: time.Now().UTC()},
			{ID: "proj_other", RootPath: "/tmp/other", DisplayName: "other", CreatedAt: time.Now().UTC()},
			{ID: "proj_new", RootPath: "/tmp/new", DisplayName: "dup", CreatedAt: time.Now().UTC()},
		},
	}
	if err := saveRegistry(paths, registry); err != nil {
		t.Fatalf("saveRegistry error: %v", err)
	}
	for _, index := range []indexFile{
		{ProjectID: "proj_old", RootPath: "/tmp/old", Entries: []pathEntry{{ProjectID: "proj_old", RelativePath: "./old-result", Basename: "old-result", Segments: []string{"old-result"}}}},
		{ProjectID: "proj_other", RootPath: "/tmp/other", Entries: []pathEntry{{ProjectID: "proj_other", RelativePath: "./other-result", Basename: "other-result", Segments: []string{"other-result"}}}},
		{ProjectID: "proj_new", RootPath: "/tmp/new", Entries: []pathEntry{{ProjectID: "proj_new", RelativePath: "./new-result", Basename: "new-result", Segments: []string{"new-result"}}}},
	} {
		if err := writeJSON(indexPath(paths, index.ProjectID), index); err != nil {
			t.Fatalf("writeJSON index error: %v", err)
		}
	}

	results, err := localSearch(paths, "dup", "")
	if err != nil {
		t.Fatalf("localSearch error: %v", err)
	}
	want := []string{"./new-result"}
	if !reflect.DeepEqual(results, want) {
		t.Fatalf("localSearch() = %#v, want %#v", results, want)
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
