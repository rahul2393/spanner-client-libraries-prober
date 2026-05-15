package main

import (
	"context"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type dmlProbe struct {
	client         *spanner.Client
	numRows        int64
	payloadSize    int
	repeatableRead bool
}

func (p *dmlProbe) Name() string {
	if p.repeatableRead {
		return "rr_occ_dml"
	}
	return "dml"
}

func (p *dmlProbe) Probe(ctx context.Context) error {
	insertSQL := "INSERT T (Key, Value) VALUES(@Id, @payload)"
	updateSQL := "UPDATE T SET Value = @payload WHERE Key = @Id"
	_, err := p.client.ReadWriteTransactionWithOptions(
		ctx,
		func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
			key := randomKey(p.numRows)
			forUpdate := ""
			if p.repeatableRead {
				forUpdate = " FOR UPDATE"
			}
			readStmt := spanner.Statement{
				SQL:    "SELECT Key, Value FROM T WHERE Key = @Id" + forUpdate,
				Params: map[string]interface{}{"Id": key},
			}
			iter := tx.QueryWithOptions(ctx, readStmt, spanner.QueryOptions{RequestTag: requestTag(p.Name())})
			found, err := hasAnyRow(iter)
			if err != nil {
				return err
			}
			sql := insertSQL
			if found {
				sql = updateSQL
			}
			_, err = tx.UpdateWithOptions(
				ctx,
				spanner.Statement{
					SQL: sql,
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
		func() spanner.TransactionOptions {
			options := spanner.TransactionOptions{TransactionTag: requestTag(p.Name())}
			if p.repeatableRead {
				options.IsolationLevel = sppb.TransactionOptions_REPEATABLE_READ
			}
			return options
		}(),
	)
	return err
}
