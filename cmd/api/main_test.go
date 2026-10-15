package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFileKeepsExistingValues(t *testing.T) {
	t.Setenv("HTTP_ADDR", "127.0.0.1:9090")
	path := filepath.Join(t.TempDir(), ".env")
	contents := "HTTP_ADDR=127.0.0.1:8080\nOIDC_ISSUER=http://127.0.0.1:8081/realms/jungle\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := loadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("HTTP_ADDR"); got != "127.0.0.1:9090" {
		t.Fatalf("HTTP_ADDR = %q; existing value must stay", got)
	}
	if got := os.Getenv("OIDC_ISSUER"); got != "http://127.0.0.1:8081/realms/jungle" {
		t.Fatalf("OIDC_ISSUER = %q", got)
	}
	t.Cleanup(func() { os.Unsetenv("OIDC_ISSUER") })
}

func TestLoadProjectEnvReadsTheModuleRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("FROM_ROOT=yes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	subdir := filepath.Join(root, "cmd", "api")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(subdir)

	if err := loadProjectEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("FROM_ROOT"); got != "yes" {
		t.Fatalf("FROM_ROOT = %q", got)
	}
	t.Cleanup(func() { os.Unsetenv("FROM_ROOT") })
}

func TestLoadEnvFileIgnoresAMissingFile(t *testing.T) {
	if err := loadEnvFile(filepath.Join(t.TempDir(), ".env")); err != nil {
		t.Fatal(err)
	}
}

func TestRequiredEnvRejectsABlankValue(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := requiredEnv("DATABASE_URL"); err == nil {
		t.Fatal("blank DATABASE_URL must fail")
	}
}
