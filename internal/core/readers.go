package core

import (
	"fmt"

	"github.com/PierreOliveira1/pandora/internal/config"
)

func (c *Core) EventReaders() ([]config.EventReader, error) {
	cfg, err := c.store.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	readers := make([]config.EventReader, len(cfg.Readers))
	for i, reader := range cfg.Readers {
		readers[i] = reader
		readers[i].Command = append([]string(nil), reader.Command...)
	}
	return readers, nil
}
