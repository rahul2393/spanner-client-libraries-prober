package main

import (
	"context"

	"cloud.google.com/go/spanner"
)

type writeProbe struct {
	client           *spanner.Client
	numRows          int64
	payloadSize      int
	replayProtection bool
}

func (p *writeProbe) Name() string {
	if p.replayProtection {
		return "write"
	}
	return "write_no_rp"
}

func (p *writeProbe) Probe(ctx context.Context) error {
	key := randomKey(p.numRows)
	m := spanner.InsertOrUpdate(tableName, []string{columnKey, columnVal}, []interface{}{key, randomString(p.payloadSize)})
	opts := []spanner.ApplyOption{
		spanner.TransactionTag(requestTag(p.Name())),
		spanner.ApplyCommitOptions(spanner.CommitOptions{MaxCommitDelay: durationPtr(0)}),
	}
	if !p.replayProtection {
		opts = append(opts, spanner.ApplyAtLeastOnce())
	}
	_, err := p.client.Apply(ctx, []*spanner.Mutation{m}, opts...)
	return err
}
