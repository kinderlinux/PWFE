package main

import (
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{name: "plain", input: "12", want: 12},
		{name: "whitespace", input: " 7\n", want: 7},
		{name: "zero", input: "0", want: 0},
		{name: "empty", input: "", wantErr: true},
		{name: "negative", input: "-1", wantErr: true},
		{name: "prefixed", input: "v2", wantErr: true},
		{name: "float", input: "1.2", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseVersion(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseVersion(%q) beklenen hata, alınan %d", test.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVersion(%q) beklenmeyen hata: %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("parseVersion(%q) = %d, istenen %d", test.input, got, test.want)
			}
		})
	}
}

func TestNormalizeUpdatePasswordArgs(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "bare flag becomes empty value",
			in:   []string{"pwfe", "--set-update-password"},
			want: []string{"pwfe", "--set-update-password="},
		},
		{
			name: "single dash bare flag",
			in:   []string{"pwfe", "-set-update-password"},
			want: []string{"pwfe", "-set-update-password="},
		},
		{
			name: "flag followed by another flag is treated as value",
			in:   []string{"pwfe", "--set-update-password", "--version"},
			want: []string{"pwfe", "--set-update-password", "--version"},
		},
		{
			name: "password starting with dash preserved",
			in:   []string{"pwfe", "--set-update-password", "-gizli"},
			want: []string{"pwfe", "--set-update-password", "-gizli"},
		},
		{
			name: "value preserved",
			in:   []string{"pwfe", "--set-update-password", "gizli"},
			want: []string{"pwfe", "--set-update-password", "gizli"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeUpdatePasswordArgs(append([]string(nil), test.in...))
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("normalizeUpdatePasswordArgs(%v) = %v, istenen %v", test.in, got, test.want)
			}
		})
	}
}

func TestUpdatePasswordRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/update.passwd"
	t.Setenv("PWFE_UPDATE_PASSWORD_FILE", path)

	if err := writeUpdatePassword("doğru-şifre"); err != nil {
		t.Fatalf("writeUpdatePassword: %v", err)
	}

	ok, err := verifyUpdatePassword("doğru-şifre")
	if err != nil || !ok {
		t.Fatalf("doğru şifre doğrulanamadı: ok=%v err=%v", ok, err)
	}

	ok, err = verifyUpdatePassword("yanlış")
	if err != nil {
		t.Fatalf("verifyUpdatePassword: %v", err)
	}
	if ok {
		t.Fatal("yanlış şifre kabul edildi")
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Fatalf("şifre dosyası izni = %o, istenen 600", perm)
		}
	}
}

func TestGenerateUpdatePassword(t *testing.T) {
	for _, length := range []int{1, 16, 32} {
		password, err := generateUpdatePassword(length)
		if err != nil {
			t.Fatalf("generateUpdatePassword(%d): %v", length, err)
		}
		if len(password) != length {
			t.Fatalf("uzunluk = %d, istenen %d", len(password), length)
		}
		for _, char := range password {
			if !strings.ContainsRune(passwordAlphabet, char) {
				t.Fatalf("şifre beklenmeyen karakter içeriyor: %q", char)
			}
		}
	}

	if _, err := generateUpdatePassword(0); err == nil {
		t.Fatal("sıfır uzunluk için hata bekleniyordu")
	}
}
