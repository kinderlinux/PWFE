package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
)

func newTestUpdater(t *testing.T) *updater {
	t.Helper()
	return &updater{
		root:   t.TempDir(),
		logger: log.New(io.Discard, "", 0),
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{input: "1", want: 1},
		{input: " 42\n", want: 42},
		{input: "", wantErr: true},
		{input: "-3", wantErr: true},
		{input: "v3", wantErr: true},
	}

	for _, test := range tests {
		got, err := parseVersion(test.input)
		if test.wantErr {
			if err == nil {
				t.Fatalf("parseVersion(%q) beklenen hata, alınan %d", test.input, got)
			}
			continue
		}
		if err != nil {
			t.Fatalf("parseVersion(%q): %v", test.input, err)
		}
		if got != test.want {
			t.Fatalf("parseVersion(%q) = %d, istenen %d", test.input, got, test.want)
		}
	}
}

func TestCopyFileAndWriteVersion(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source")
	destination := filepath.Join(dir, "destination")

	if err := os.WriteFile(source, []byte("merhaba"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(source, destination, 0755); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "merhaba" {
		t.Fatalf("kopya içeriği = %q", data)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("kopya izni = %o", info.Mode().Perm())
	}

	versionPath := filepath.Join(dir, "version.txt")
	if err := writeVersion(versionPath, 7); err != nil {
		t.Fatalf("writeVersion: %v", err)
	}
	data, err = os.ReadFile(versionPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "7\n" {
		t.Fatalf("version.txt içeriği = %q", data)
	}
}
