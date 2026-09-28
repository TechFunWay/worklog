package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvPreservesProcessEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("DOTENV_NEW=value\nDOTENV_EXISTING=from-file\nDOTENV_QUOTED=\"quoted value\"\n# comment\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"DOTENV_NEW", "DOTENV_EXISTING", "DOTENV_QUOTED"} {
		original, existed := os.LookupEnv(key)
		_ = os.Unsetenv(key)
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(key, original)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
	if err := os.Setenv("DOTENV_EXISTING", "from-process"); err != nil {
		t.Fatal(err)
	}

	LoadDotEnv(filepath.Join(dir, "missing.env"), path)
	if got := os.Getenv("DOTENV_NEW"); got != "value" {
		t.Fatalf("new value = %q, want value", got)
	}
	if got := os.Getenv("DOTENV_QUOTED"); got != "quoted value" {
		t.Fatalf("quoted value = %q", got)
	}
	if got := os.Getenv("DOTENV_EXISTING"); got != "from-process" {
		t.Fatalf("process environment was overridden: %q", got)
	}
}
