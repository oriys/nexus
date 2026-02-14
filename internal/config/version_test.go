package config

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

func testConfig(listen string) *Config {
	return &Config{
		Server: ServerConfig{Listen: listen},
	}
}

func TestVersionManager_SaveAndCurrent(t *testing.T) {
	vm := NewVersionManager(10)

	cfg := testConfig(":8080")
	rawData := []byte("server:\n  listen: :8080")
	vm.Save(cfg, rawData)

	cur := vm.Current()
	if cur == nil {
		t.Fatal("expected current version, got nil")
	}
	if cur.Version != 1 {
		t.Errorf("expected version 1, got %d", cur.Version)
	}
	if cur.Config.Server.Listen != ":8080" {
		t.Errorf("expected listen :8080, got %s", cur.Config.Server.Listen)
	}
	expectedHash := fmt.Sprintf("%x", sha256.Sum256(rawData))
	if cur.Hash != expectedHash {
		t.Errorf("expected hash %s, got %s", expectedHash, cur.Hash)
	}
}

func TestVersionManager_MultipleVersions(t *testing.T) {
	vm := NewVersionManager(10)

	vm.Save(testConfig(":8080"), []byte("v1"))
	vm.Save(testConfig(":8081"), []byte("v2"))
	vm.Save(testConfig(":8082"), []byte("v3"))

	if vm.Len() != 3 {
		t.Errorf("expected 3 versions, got %d", vm.Len())
	}

	cur := vm.Current()
	if cur.Version != 3 {
		t.Errorf("expected version 3, got %d", cur.Version)
	}
	if cur.Config.Server.Listen != ":8082" {
		t.Errorf("expected listen :8082, got %s", cur.Config.Server.Listen)
	}
}

func TestVersionManager_MaxHistory(t *testing.T) {
	vm := NewVersionManager(3)

	for i := 0; i < 5; i++ {
		vm.Save(testConfig(fmt.Sprintf(":%d", 8080+i)), []byte(fmt.Sprintf("v%d", i)))
	}

	if vm.Len() != 3 {
		t.Errorf("expected 3 versions after trimming, got %d", vm.Len())
	}

	list := vm.List()
	if list[0].Version != 3 {
		t.Errorf("expected oldest remaining version to be 3, got %d", list[0].Version)
	}
	if list[2].Version != 5 {
		t.Errorf("expected newest version to be 5, got %d", list[2].Version)
	}
}

func TestVersionManager_Rollback(t *testing.T) {
	vm := NewVersionManager(10)

	cfg1 := testConfig(":8080")
	cfg2 := testConfig(":9090")
	vm.Save(cfg1, []byte("v1"))
	vm.Save(cfg2, []byte("v2"))

	rolled, err := vm.Rollback()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rolled.Server.Listen != ":8080" {
		t.Errorf("expected rollback to :8080, got %s", rolled.Server.Listen)
	}

	cur := vm.Current()
	if cur.Config.Server.Listen != ":8080" {
		t.Errorf("expected current after rollback to be :8080, got %s", cur.Config.Server.Listen)
	}
	if vm.Len() != 3 {
		t.Errorf("expected 3 versions after rollback, got %d", vm.Len())
	}
}

func TestVersionManager_RollbackNoHistory(t *testing.T) {
	vm := NewVersionManager(10)

	_, err := vm.Rollback()
	if err == nil {
		t.Fatal("expected error on rollback with no history")
	}
	if err.Error() != "no previous version to rollback to" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestVersionManager_RollbackOnlyOne(t *testing.T) {
	vm := NewVersionManager(10)
	vm.Save(testConfig(":8080"), []byte("v1"))

	_, err := vm.Rollback()
	if err == nil {
		t.Fatal("expected error on rollback with only one version")
	}
	if err.Error() != "no previous version to rollback to" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestVersionManager_List(t *testing.T) {
	vm := NewVersionManager(10)
	vm.Save(testConfig(":8080"), []byte("v1"))
	vm.Save(testConfig(":9090"), []byte("v2"))

	list := vm.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 versions in list, got %d", len(list))
	}

	// Mutating the returned list should not affect the manager.
	list[0].Version = 999
	original := vm.List()
	if original[0].Version == 999 {
		t.Error("List() did not return a copy; mutations affected internal state")
	}
}

func TestVersionManager_Previous(t *testing.T) {
	vm := NewVersionManager(10)

	if vm.Previous() != nil {
		t.Error("expected nil Previous with no versions")
	}

	vm.Save(testConfig(":8080"), []byte("v1"))
	if vm.Previous() != nil {
		t.Error("expected nil Previous with only one version")
	}

	vm.Save(testConfig(":9090"), []byte("v2"))
	prev := vm.Previous()
	if prev == nil {
		t.Fatal("expected non-nil Previous")
	}
	if prev.Config.Server.Listen != ":8080" {
		t.Errorf("expected previous listen :8080, got %s", prev.Config.Server.Listen)
	}
	if prev.Version != 1 {
		t.Errorf("expected previous version 1, got %d", prev.Version)
	}
}

func TestVersionManager_DefaultMaxHistory(t *testing.T) {
	vm := NewVersionManager(0)
	if vm.maxHistory != 10 {
		t.Errorf("expected default maxHistory 10, got %d", vm.maxHistory)
	}

	vm2 := NewVersionManager(-5)
	if vm2.maxHistory != 10 {
		t.Errorf("expected default maxHistory 10, got %d", vm2.maxHistory)
	}
}
