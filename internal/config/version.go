package config

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

// ConfigVersion represents a saved configuration version.
type ConfigVersion struct {
	Version   int
	Hash      string
	Timestamp time.Time
	Config    *Config
}

// VersionManager manages configuration version history with rollback support.
type VersionManager struct {
	mu         sync.Mutex
	versions   []ConfigVersion
	maxHistory int
	nextVer    int
}

// NewVersionManager creates a new VersionManager. If maxHistory <= 0, defaults to 10.
func NewVersionManager(maxHistory int) *VersionManager {
	if maxHistory <= 0 {
		maxHistory = 10
	}
	return &VersionManager{
		versions:   make([]ConfigVersion, 0),
		maxHistory: maxHistory,
	}
}

// Save saves a new configuration version computed from rawData.
func (vm *VersionManager) Save(cfg *Config, rawData []byte) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	hash := fmt.Sprintf("%x", sha256.Sum256(rawData))
	vm.nextVer++
	version := vm.nextVer

	vm.versions = append(vm.versions, ConfigVersion{
		Version:   version,
		Hash:      hash,
		Timestamp: time.Now(),
		Config:    cfg,
	})

	if len(vm.versions) > vm.maxHistory {
		vm.versions = vm.versions[len(vm.versions)-vm.maxHistory:]
	}
}

// Current returns the latest configuration version, or nil if none exist.
func (vm *VersionManager) Current() *ConfigVersion {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if len(vm.versions) == 0 {
		return nil
	}
	v := vm.versions[len(vm.versions)-1]
	return &v
}

// Previous returns the second-to-last configuration version, or nil if fewer than 2 exist.
func (vm *VersionManager) Previous() *ConfigVersion {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if len(vm.versions) < 2 {
		return nil
	}
	v := vm.versions[len(vm.versions)-2]
	return &v
}

// List returns a copy of the version history.
func (vm *VersionManager) List() []ConfigVersion {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	out := make([]ConfigVersion, len(vm.versions))
	copy(out, vm.versions)
	return out
}

// Rollback rolls back to the previous configuration version.
func (vm *VersionManager) Rollback() (*Config, error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	if len(vm.versions) < 2 {
		return nil, fmt.Errorf("no previous version to rollback to")
	}

	prev := vm.versions[len(vm.versions)-2]
	vm.nextVer++
	version := vm.nextVer

	vm.versions = append(vm.versions, ConfigVersion{
		Version:   version,
		Hash:      prev.Hash,
		Timestamp: time.Now(),
		Config:    prev.Config,
	})

	if len(vm.versions) > vm.maxHistory {
		vm.versions = vm.versions[len(vm.versions)-vm.maxHistory:]
	}

	return prev.Config, nil
}

// Len returns the number of stored versions.
func (vm *VersionManager) Len() int {
	vm.mu.Lock()
	defer vm.mu.Unlock()

	return len(vm.versions)
}
