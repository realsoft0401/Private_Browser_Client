package BrowserEnv

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractImportTarGzRejectsSymlink(t *testing.T) {
	file := buildImportTarGzForTest(t, []tarEntryForTest{
		{name: "env/profile.json", body: `{}`},
		{name: "env/link", linkName: "../evil", typeflag: tar.TypeSymlink},
	})
	defer os.Remove(file.Name())
	defer file.Close()

	target := t.TempDir()
	err := extractImportTarGz(file, target)
	if err == nil || !strings.Contains(err.Error(), "不支持的 tar entry") {
		t.Fatalf("expected unsupported tar entry error, got %v", err)
	}
}

func TestExtractImportTarGzRejectsPathTraversal(t *testing.T) {
	file := buildImportTarGzForTest(t, []tarEntryForTest{
		{name: "../evil", body: "bad"},
	})
	defer os.Remove(file.Name())
	defer file.Close()

	target := t.TempDir()
	err := extractImportTarGz(file, target)
	if err == nil || !strings.Contains(err.Error(), "非法路径") {
		t.Fatalf("expected illegal path error, got %v", err)
	}
}

func TestExtractImportTarGzExtractsRegularFiles(t *testing.T) {
	file := buildImportTarGzForTest(t, []tarEntryForTest{
		{name: "env/profile.json", body: `{"ok":true}`},
	})
	defer os.Remove(file.Name())
	defer file.Close()

	target := t.TempDir()
	if err := extractImportTarGz(file, target); err != nil {
		t.Fatalf("extractImportTarGz returned error: %v", err)
	}
	bytes, err := os.ReadFile(filepath.Join(target, "env", "profile.json"))
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if strings.TrimSpace(string(bytes)) != `{"ok":true}` {
		t.Fatalf("unexpected extracted content: %s", string(bytes))
	}
}

type tarEntryForTest struct {
	name     string
	body     string
	linkName string
	typeflag byte
}

func buildImportTarGzForTest(t *testing.T, entries []tarEntryForTest) *os.File {
	t.Helper()
	file, err := os.CreateTemp("", "private-browser-import-test-*.tar.gz")
	if err != nil {
		t.Fatalf("create temp tar: %v", err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		header := &tar.Header{
			Name:     entry.name,
			Mode:     0644,
			Typeflag: typeflag,
			Linkname: entry.linkName,
		}
		if typeflag == tar.TypeReg || typeflag == tar.TypeRegA {
			header.Size = int64(len(entry.body))
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatalf("write tar header: %v", err)
		}
		if header.Size > 0 {
			if _, err := tarWriter.Write([]byte(entry.body)); err != nil {
				t.Fatalf("write tar body: %v", err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatalf("seek tar file: %v", err)
	}
	return file
}
