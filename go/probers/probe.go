package main

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/api/iterator"
)

type probe interface {
	Name() string
	Probe(context.Context) error
}

func newProbe(client *spanner.Client, cfg config) (probe, error) {
	switch cfg.probeType {
	case "test_noop":
		return &noopProbe{}, nil
	case "strong_read":
		return &strongReadProbe{client: client, numRows: cfg.numRows}, nil
	case "stale_read":
		return &staleReadProbe{client: client, numRows: cfg.numRows, maxStalenessSeconds: cfg.maxStalenessSeconds}, nil
	case "read_write":
		return &readWriteProbe{client: client, numRows: cfg.numRows, payloadSize: cfg.payloadSize}, nil
	case "rr_occ_dml":
		return &dmlProbe{client: client, numRows: cfg.numRows, payloadSize: cfg.payloadSize, repeatableRead: true}, nil
	case "dml":
		return &dmlProbe{client: client, numRows: cfg.numRows, payloadSize: cfg.payloadSize, repeatableRead: false}, nil
	case "blind_dml":
		return &blindDMLProbe{client: client, numRows: cfg.numRows, payloadSize: cfg.payloadSize}, nil
	case "multi_blind_dml":
		return &multiBlindDMLProbe{client: client, numRows: cfg.numRows, payloadSize: cfg.payloadSize, statements: 5}, nil
	case "strong_query":
		return &queryProbe{client: client, numRows: cfg.numRows, queryMode: cfg.queryMode, fixedKey: cfg.fixedKey}, nil
	case "stale_query":
		return &queryProbe{client: client, numRows: cfg.numRows, maxStalenessSeconds: cfg.maxStalenessSeconds, queryMode: cfg.queryMode, fixedKey: cfg.fixedKey}, nil
	case "multi_use_ro_query":
		return &multiUseReadOnlyQueryProbe{client: client, numRows: cfg.numRows, queryMode: cfg.queryMode}, nil
	case "write":
		return &writeProbe{client: client, numRows: cfg.numRows, payloadSize: cfg.payloadSize, replayProtection: true}, nil
	case "write_no_rp":
		return &writeProbe{client: client, numRows: cfg.numRows, payloadSize: cfg.payloadSize, replayProtection: false}, nil
	default:
		return nil, fmt.Errorf("unsupported PROBE_TYPE: %s", cfg.probeType)
	}
}

func consumeRows(iter *spanner.RowIterator) error {
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func hasAnyRow(iter *spanner.RowIterator) (bool, error) {
	defer iter.Stop()
	_, err := iter.Next()
	if errors.Is(err, iterator.Done) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func randomKey(numRows int64) int64 {
	return rand.Int63n(numRows)
}

func requestTag(name string) string {
	return "probe_type=" + name
}

func randomString(length int) string {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	out := make([]byte, length)
	for i := range out {
		out[i] = chars[rand.Intn(len(chars))]
	}
	return string(out)
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
