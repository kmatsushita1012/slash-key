package main

import (
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
