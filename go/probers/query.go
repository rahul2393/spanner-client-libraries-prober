package main

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type queryProbe struct {
	client              *spanner.Client
	numRows             int64
	maxStalenessSeconds int64
	queryMode           string
}

func (p *queryProbe) Name() string {
	if p.maxStalenessSeconds > 0 {
		return "stale_query"
	}
	return "strong_query"
}

func (p *queryProbe) Probe(ctx context.Context) error {
	key := randomKey(p.numRows)
	stmt := spanner.Statement{
		SQL:    "SELECT Key, Value FROM T WHERE Key = @Id",
		Params: map[string]interface{}{"Id": key},
	}
	ro := p.client.Single()
	if p.maxStalenessSeconds > 0 {
		ro = ro.WithTimestampBound(spanner.MaxStaleness(time.Duration(p.maxStalenessSeconds) * time.Second))
	}
	queryOpts := spanner.QueryOptions{RequestTag: requestTag(p.Name())}
	if p.queryMode == "stats" {
		mode := sppb.ExecuteSqlRequest_WITH_STATS
		queryOpts.Mode = &mode
	}
	iter := ro.QueryWithOptions(ctx, stmt, queryOpts)
	return consumeRows(iter)
}
