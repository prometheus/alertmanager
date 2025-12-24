package tsdb

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/route"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	v1 "github.com/prometheus/prometheus/web/api/v1"
)

type QueryAPI struct {
	router *route.Router
}

type queryableAdapter struct {
	tsdb *TSDB
}

func (q *queryableAdapter) Querier(mint, maxt int64) (storage.Querier, error) {
	return q.tsdb.Querier(mint, maxt)
}

func (q *queryableAdapter) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	return q.tsdb.ChunkQuerier(mint, maxt)
}

func NewQueryAPI(tsdb *TSDB) (*QueryAPI, error) {
	// Create a queryable interface from TSDB that implements both Querier and ChunkQuerier
	queryable := &queryableAdapter{tsdb: tsdb}

	// Create Prometheus API with query engine
	engine := promql.NewEngine(promql.EngineOpts{
		Logger:               tsdb.logger,
		Reg:                  prometheus.DefaultRegisterer,
		MaxSamples:           50000000,
		Timeout:              2 * time.Minute,
		LookbackDelta:        5 * time.Minute,
		EnableAtModifier:     true,
		EnableNegativeOffset: true,
	})

	api := v1.NewAPI(
		engine,    // promql.QueryEngine
		queryable, // storage.SampleAndChunkQueryable
		nil,       // storage.Appendable
		nil,       // storage.ExemplarQueryable
		nil,       // ScrapePoolsRetriever func
		nil,       // TargetRetriever func
		nil,       // AlertmanagerRetriever func
		func() config.Config { return config.Config{} }, // config func
		nil,                   // flags map
		v1.GlobalURLOptions{}, // global URL options
		func(f http.HandlerFunc) http.HandlerFunc { return f }, // ready func
		nil,     // TSDBAdminStats
		"",      // db dir
		false,   // enable admin
		nil,     // logger
		nil,     // RulesRetriever func
		0, 0, 0, // remote read limits
		false,    // is agent
		nil,      // CORS origin
		nil,      // runtime info func
		nil,      // build info
		nil, nil, // notifications getter/sub
		prometheus.DefaultGatherer,   // gatherer
		prometheus.DefaultRegisterer, // registerer
		nil,                          // stats renderer (uses default)
		false,                        // rwEnabled
		remote.MessageTypes{},        // acceptRemoteWriteProtoMsgs
		false, false, false,          // OTLP flags
		false,         // stZeroIngestionEnabled
		5*time.Minute, // lookback delta
		false, false,  // type/unit labels, append metadata
		nil, // override error code
	)

	// Create a router and register the API
	router := route.New()
	api.Register(router)

	return &QueryAPI{
		router: router,
	}, nil
}

func (q *QueryAPI) Handler() http.Handler {
	return q.router
}
