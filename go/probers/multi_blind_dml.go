package main

import (
	"context"

	"cloud.google.com/go/spanner"
)

type multiBlindDMLProbe struct {
	client      *spanner.Client
	numRows     int64
	payloadSize int
	statements  int
}

func (p *multiBlindDMLProbe) Name() string { return "multi_blind_dml" }

func (p *multiBlindDMLProbe) Probe(ctx context.Context) error {
	_, err := p.client.ReadWriteTransactionWithOptions(
		ctx,
		func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
			keys := make([]int64, p.statements)
			if p.numRows <= int64(p.statements) {
				for i := range keys {
					keys[i] = int64(i)
				}
			} else {
				keys[0] = randomKey(p.numRows)
				for i := 1; i < len(keys); i++ {
					keys[i] = keys[i-1] + 1
				}
			}
			payload := randomString(p.payloadSize)
			for i, key := range keys {
				_, err := tx.UpdateWithOptions(
					ctx,
					spanner.Statement{
						SQL: "INSERT OR UPDATE T (Key, Value) VALUES(@Id, @payload)",
						Params: map[string]interface{}{
							"Id":      key,
							"payload": payload,
						},
					},
					spanner.QueryOptions{
						RequestTag:    requestTag(p.Name()),
						LastStatement: i == len(keys)-1,
					},
				)
				if err != nil {
					return err
				}
			}
			return nil
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
