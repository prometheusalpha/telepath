package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"telepath/internal/model"
)

type memKeyring map[string][]byte

func (m memKeyring) Get(name string) ([]byte, bool, error) { v, ok := m[name]; return v, ok, nil }
func (m memKeyring) Set(name string, v []byte) error       { m[name] = v; return nil }

func secretWorkspace() *model.Workspace {
	w := &model.Workspace{Env: []model.KV{
		{Key: "token", Value: "env-secret", Enabled: true, Secret: true},
		{Key: "base", Value: "https://a.dev", Enabled: true},
	}}
	n := model.NewRequest("login")
	n.Request.Auth = model.Auth{Kind: model.AuthBasic, Username: "ada", Password: "pw-secret"}
	w.Add(n, "")
	return w
}

func TestSealedSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "workspace.json")
	kr := memKeyring{}
	s, err := NewSealer(kr)
	if err != nil {
		t.Fatal(err)
	}
	w := secretWorkspace()
	if err := Save(path, w, s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	file := string(b)
	if strings.Contains(file, "env-secret") || strings.Contains(file, "pw-secret") || !strings.Contains(file, sealPrefix) {
		t.Fatalf("secrets in the clear:\n%s", file)
	}
	if !strings.Contains(file, "https://a.dev") || !strings.Contains(file, `"ada"`) {
		t.Error("what isn't secret got sealed")
	}
	if w.Env[0].Value != "env-secret" {
		t.Error("saving sealed the workspace in memory")
	}

	// The same key, as on the next start, which finds it stored.
	s2, _ := NewSealer(kr)
	got, locked, err := Load(path, s2)
	if err != nil || locked != 0 || got.Env[0].Value != "env-secret" || got.Items[0].Request.Auth.Password != "pw-secret" {
		t.Fatalf("loaded %+v, locked %d, %v", got.Env, locked, err)
	}
	if len(kr) != 1 {
		t.Errorf("keys %d", len(kr))
	}

	// Without the key, the sealed values stay, and survive a save.
	got, locked, _ = Load(path, nil)
	if locked != 2 || !strings.HasPrefix(got.Env[0].Value, sealPrefix) {
		t.Fatalf("without the key: locked %d, %q", locked, got.Env[0].Value)
	}
	Save(path, got, nil)
	again, locked, _ := Load(path, s2)
	if locked != 0 || again.Env[0].Value != "env-secret" {
		t.Errorf("a save without the key lost the secret: %q", again.Env[0].Value)
	}

	// Another key can't read them, and says so.
	other, _ := NewSealer(memKeyring{})
	if _, locked, _ := Load(path, other); locked != 2 {
		t.Errorf("another key: locked %d", locked)
	}
}
