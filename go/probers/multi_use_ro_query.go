package main

import (
	"context"

	"cloud.google.com/go/spanner"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type multiUseReadOnlyQueryProbe struct {
	client    *spanner.Client
	numRows   int64
	queryMode string
}

func (p *multiUseReadOnlyQueryProbe) Name() string { return "multi_use_ro_query" }

func (p *multiUseReadOnlyQueryProbe) Probe(ctx context.Context) error {
	firstKey := randomKey(p.numRows)
	secondKey := randomKey(p.numRows)
	tx := p.client.ReadOnlyTransaction()
	defer tx.Close()
	readIter := tx.ReadWithOptions(
		ctx,
		tableName,
		spanner.Key{firstKey},
		[]string{columnKey, columnVal},
		&spanner.ReadOptions{RequestTag: requestTag(p.Name())},
	)
	if err := consumeRows(readIter); err != nil {
		return err
	}
	stmt := spanner.Statement{
		SQL:    "SELECT Key, Value FROM T WHERE Key = @Id",
		Params: map[string]interface{}{"Id": secondKey},
	}
	queryOpts := spanner.QueryOptions{RequestTag: requestTag(p.Name())}
	if p.queryMode == "stats" {
		mode := sppb.ExecuteSqlRequest_PROFILE
		queryOpts.Mode = &mode
	}
	queryIter := tx.QueryWithOptions(ctx, stmt, queryOpts)
	return consumeRows(queryIter)
}
