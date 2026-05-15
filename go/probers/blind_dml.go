package main

import (
	"context"

	"cloud.google.com/go/spanner"
)

type blindDMLProbe struct {
	client      *spanner.Client
	numRows     int64
	payloadSize int
}

func (p *blindDMLProbe) Name() string { return "blind_dml" }

func (p *blindDMLProbe) Probe(ctx context.Context) error {
	_, err := p.client.ReadWriteTransactionWithOptions(
		ctx,
		func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
			key := randomKey(p.numRows)
			_, err := tx.UpdateWithOptions(
				ctx,
				spanner.Statement{
					SQL: "INSERT OR UPDATE T (Key, Value) VALUES(@Id, @payload)",
					Params: map[string]interface{}{
						"Id":      key,
						"payload": randomString(p.payloadSize),
					},
				},
				spanner.QueryOptions{
					RequestTag:    requestTag(p.Name()),
					LastStatement: true,
				},
			)
			return err
		},
		spanner.TransactionOptions{
			TransactionTag: requestTag(p.Name()),
			CommitOptions: spanner.CommitOptions{
				MaxCommitDelay: durationPtr(0),
			},
		},
	)
	return err
}
