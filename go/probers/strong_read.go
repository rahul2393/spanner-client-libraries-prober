package main

import (
	"context"

	"cloud.google.com/go/spanner"
)

type strongReadProbe struct {
	client  *spanner.Client
	numRows int64
}

func (p *strongReadProbe) Name() string { return "strong_read" }

func (p *strongReadProbe) Probe(ctx context.Context) error {
	key := randomKey(p.numRows)
	iter := p.client.Single().ReadWithOptions(
		ctx,
		tableName,
		spanner.Key{key},
		[]string{columnKey, columnVal},
		&spanner.ReadOptions{RequestTag: requestTag(p.Name())},
	)
	return consumeRows(iter)
}
