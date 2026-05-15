package main

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
)

type readWriteProbe struct {
	client      *spanner.Client
	numRows     int64
	payloadSize int
}

func (p *readWriteProbe) Name() string { return "read_write" }

func (p *readWriteProbe) Probe(ctx context.Context) error {
	_, err := p.client.ReadWriteTransactionWithOptions(
		ctx,
		func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
			key := randomKey(p.numRows)
			_, readErr := tx.ReadRow(ctx, tableName, spanner.Key{key}, []string{columnKey, columnVal})
			if readErr != nil && spanner.ErrCode(readErr) != codes.NotFound {
				return readErr
			}
			m := spanner.InsertOrUpdate(tableName, []string{columnKey, columnVal}, []interface{}{key, randomString(p.payloadSize)})
			return tx.BufferWrite([]*spanner.Mutation{m})
		},
		spanner.TransactionOptions{TransactionTag: requestTag(p.Name())},
	)
	return err
}
