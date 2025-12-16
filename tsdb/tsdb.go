package tsdb

import (
	"log/slog"
	"path/filepath"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
)

type TSDB struct {
	tsdb   *tsdb.DB
	logger *slog.Logger
}

func NewTSDB(dataDir string, retention time.Duration, logger *slog.Logger, reg prometheus.Registerer) (*TSDB, error) {
	opts := tsdb.DefaultOptions()
	opts.RetentionDuration = int64(retention / time.Millisecond)

	db, err := tsdb.Open(filepath.Join(dataDir, "tsdb"), logger, reg, opts, nil)
	if err != nil {
		return nil, err
	}

	return &TSDB{tsdb: db, logger: logger}, nil
}

func (t *TSDB) Close() error {
	t.logger.Info("closing TSDB")
	return t.tsdb.Close()
}

func (t *TSDB) Querier(mint, maxt int64) (storage.Querier, error) {
	return t.tsdb.Querier(mint, maxt)
}

func (t *TSDB) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	return t.tsdb.ChunkQuerier(mint, maxt)
}
