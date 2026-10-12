package main

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"github.com/klauspost/compress/zstd"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func helperMRS(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	b.Write([]byte{'M', 'R', 'S', 1, 1})
	for _, n := range []int64{1, 0} {
		_ = binary.Write(&b, binary.BigEndian, n)
	}
	b.WriteByte(1)
	_ = binary.Write(&b, binary.BigEndian, int64(1))
	b.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 10, 0, 0, 0})
	b.Write([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 10, 255, 255, 255})
	e, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	return e.EncodeAll(b.Bytes(), nil)
}
func TestRuleAssetValidatesAndPersistsOnlyHashedMRS(t *testing.T) {
	h := testHelper(t)
	data := helperMRS(t)
	body := map[string]any{"homeDir": h.config.managedConfigDir, "content": base64.StdEncoding.EncodeToString(data), "path": "/tmp/arbitrary.mrs"}
	saved, err := h.saveRuleAsset(body)
	if err != nil {
		t.Fatal(err)
	}
	path := saved["path"].(string)
	if filepath.Dir(path) != filepath.Join(h.config.managedConfigDir, "rule-import") {
		t.Fatal(path)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err = h.saveRuleAsset(body); err != nil {
		t.Fatal(err)
	}
	body["homeDir"] = t.TempDir()
	if _, err = h.saveRuleAsset(body); err == nil {
		t.Fatal("arbitrary home accepted")
	}
	body["homeDir"] = h.config.managedConfigDir
	body["content"] = base64.StdEncoding.EncodeToString([]byte("rules: []"))
	if _, err = h.saveRuleAsset(body); err == nil {
		t.Fatal("text accepted as MRS")
	}
}
func TestRuleAssetRejectsSymlinkAndExternalCore(t *testing.T) {
	h := testHelper(t)
	data := helperMRS(t)
	body := map[string]any{"homeDir": h.config.managedConfigDir, "content": base64.StdEncoding.EncodeToString(data)}
	if err := os.MkdirAll(h.config.managedConfigDir, 0750); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(h.config.managedConfigDir, "rule-import")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.saveRuleAsset(body); err == nil {
		t.Fatal("symlink accepted")
	}
	if err := atomicWrite(h.config.coreModeFile, []byte(`{"mode":"external"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := h.saveRuleAsset(body); err == nil || !strings.Contains(err.Error(), "外部 Core") {
		t.Fatalf("external write was not rejected by read-only policy: %v", err)
	}
}
