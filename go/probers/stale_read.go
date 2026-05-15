package main

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
)

type staleReadProbe struct {
	client              *spanner.Client
	numRows             int64
	maxStalenessSeconds int64
}

func (p *staleReadProbe) Name() string { return "stale_read" }

func (p *staleReadProbe) Probe(ctx context.Context) error {
	key := randomKey(p.numRows)
	iter := p.client.Single().
		WithTimestampBound(spanner.MaxStaleness(time.Duration(p.maxStalenessSeconds)*time.Second)).
		ReadWithOptions(
			ctx,
			tableName,
			spanner.Key{key},
			[]string{columnKey, columnVal},
			&spanner.ReadOptions{RequestTag: requestTag(p.Name())},
		)
	return consumeRows(iter)
}
