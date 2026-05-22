package main

import (
	"context"
	"math/big"
	"os"
	"reflect"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
)

type readLargeResultSetProbe struct {
	client  *spanner.Client
	numRows int64
	stmt    spanner.Statement
}

func newReadLargeResultSetProbe(client *spanner.Client, numRows int64) *readLargeResultSetProbe {
	if numRows <= 0 {
		numRows = 100_000
	}
	sql := `SELECT
  MOD(FARM_FINGERPRINT(GENERATE_UUID()), 2) = 0 AS random_bool,
  CAST(GENERATE_UUID() AS BYTES) AS random_bytes,
  DATE_FROM_UNIX_DATE(ABS(MOD(FARM_FINGERPRINT(GENERATE_UUID()), 2932896))) AS random_date,
  CAST(FARM_FINGERPRINT(GENERATE_UUID()) / FARM_FINGERPRINT(GENERATE_UUID()) AS FLOAT32) AS random_float32,
  CAST(FARM_FINGERPRINT(GENERATE_UUID()) / FARM_FINGERPRINT(GENERATE_UUID()) AS FLOAT64) AS random_float64,
  MAKE_INTERVAL(ABS(MOD(FARM_FINGERPRINT(GENERATE_UUID()), 10)), ABS(MOD(FARM_FINGERPRINT(GENERATE_UUID()), 12)), ABS(MOD(FARM_FINGERPRINT(GENERATE_UUID()), 28)), ABS(MOD(FARM_FINGERPRINT(GENERATE_UUID()), 24)), ABS(MOD(FARM_FINGERPRINT(GENERATE_UUID()), 60)), ABS(MOD(FARM_FINGERPRINT(GENERATE_UUID()), 60))) AS random_interval,
  TO_JSON('{"key": "' || GENERATE_UUID() || '"}') AS random_json,
  FARM_FINGERPRINT(GENERATE_UUID()) AS random_int64,
  CAST(FARM_FINGERPRINT(GENERATE_UUID()) / FARM_FINGERPRINT(GENERATE_UUID()) AS NUMERIC) AS random_numeric,
  GENERATE_UUID() AS random_string,
  TIMESTAMP_MICROS(ABS(MOD(FARM_FINGERPRINT(GENERATE_UUID()), 1230219000000000))) AS random_timestamp,
  NEW_UUID() AS random_uuid
FROM UNNEST(GENERATE_ARRAY(1, @num_rows)) AS n`
	return &readLargeResultSetProbe{
		client:  client,
		numRows: numRows,
		stmt: spanner.Statement{
			SQL:    sql,
			Params: map[string]interface{}{"num_rows": numRows},
		},
	}
}

func (p *readLargeResultSetProbe) Name() string { return "read_large_result_set" }

func (p *readLargeResultSetProbe) Probe(ctx context.Context) error {
	queryOptions := spanner.QueryOptions{RequestTag: requestTag(p.Name())}
	applyOptionalRawDecode(&queryOptions)
	iter := p.client.Single().QueryWithOptions(ctx, p.stmt, queryOptions)
	defer iter.Stop()
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return err
		}
		if err := decodeLargeResultSetRow(row); err != nil {
			return err
		}
	}
}

func applyOptionalRawDecode(queryOptions *spanner.QueryOptions) {
	if os.Getenv("SPANNER_RAW_DECODE") != "true" && os.Getenv("SPANNER_RAW_DECODE") != "1" {
		return
	}
	v := reflect.ValueOf(queryOptions).Elem()
	f := v.FieldByName("ExperimentalRawDecode")
	if f.IsValid() && f.Kind() == reflect.Bool && f.CanSet() {
		f.SetBool(true)
	}
}

func decodeLargeResultSetRow(row *spanner.Row) error {
	var randomBool bool
	var randomBytes []byte
	var randomDate spanner.NullDate
	var randomFloat32 float32
	var randomFloat64 float64
	var randomInterval spanner.GenericColumnValue
	var randomJson spanner.NullJSON
	var randomInt64 int64
	var randomNumeric big.Rat
	var randomString string
	var randomTimestamp time.Time
	var randomUuid uuid.UUID

	if err := row.Column(0, &randomBool); err != nil {
		return err
	}
	if err := row.Column(1, &randomBytes); err != nil {
		return err
	}
	if err := row.Column(2, &randomDate); err != nil {
		return err
	}
	if err := row.Column(3, &randomFloat32); err != nil {
		return err
	}
	if err := row.Column(4, &randomFloat64); err != nil {
		return err
	}
	if err := row.Column(5, &randomInterval); err != nil {
		return err
	}
	if err := row.Column(6, &randomJson); err != nil {
		return err
	}
	if err := row.Column(7, &randomInt64); err != nil {
		return err
	}
	if err := row.Column(8, &randomNumeric); err != nil {
		return err
	}
	if err := row.Column(9, &randomString); err != nil {
		return err
	}
	if err := row.Column(10, &randomTimestamp); err != nil {
		return err
	}
	return row.Column(11, &randomUuid)
}
