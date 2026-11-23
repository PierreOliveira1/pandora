package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const catalogTTL = 24 * time.Hour

func (c *Core) loadCatalog(service string) (*Catalog, error) {
	data, err := os.ReadFile(c.store.Paths().ToolCacheFile(service))
	if err != nil {
		return nil, err
	}

	var catalog Catalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parsing tool cache for service %q: %w", service, err)
	}
	if catalog.Tools == nil {
		catalog.Tools = []*mcp.Tool{}
	}
	return &catalog, nil
}

func (c *Core) saveCatalog(service string, catalog *Catalog) (err error) {
	data, err := json.Marshal(catalog)
	if err != nil {
		return fmt.Errorf("encoding tool cache for service %q: %w", service, err)
	}

	path := c.store.Paths().ToolCacheFile(service)
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating tool cache directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".tools-*.json")
	if err != nil {
		return fmt.Errorf("creating temporary tool cache: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpName)
		}
	}()

	if err = tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("setting tool cache permissions: %w", err)
	}
	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("writing tool cache: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing tool cache: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replacing tool cache: %w", err)
	}
	return nil
}
