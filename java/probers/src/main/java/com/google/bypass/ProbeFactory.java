package com.google.bypass;

import com.google.cloud.spanner.DatabaseClient;

final class ProbeFactory {
  private ProbeFactory() {}

  static Probe create(
      DatabaseClient client,
      String workload,
      int numRows,
      int payloadSize,
      long maxStalenessSeconds,
      String ycsbTable,
      String ycsbKey,
      String ycsbUserId,
      int ycsbZeroPadding,
      int fixedKey) {
    return switch (workload) {
      case "test_noop" -> new TestNoopProbe();
      case "stale_read" -> new StaleReadProbe(client, numRows, 15);
      case "strong_read" -> new StrongReadProbe(client, numRows);
      case "read_write" -> new ReadWriteProbe(client, numRows, payloadSize);
      case "rr_occ_dml" ->
          new DmlProbe(client, numRows, payloadSize, /* repeatableReadOcc= */ true);
      case "write" -> new WriteProbe(client, numRows, payloadSize, true);
      case "write_no_rp" -> new WriteProbe(client, numRows, payloadSize, false);
      case "dml" -> new DmlProbe(client, numRows, payloadSize, /* repeatableReadOcc= */ false);
      case "blind_dml" -> new BlindDmlProbe(client, numRows, payloadSize);
      case "multi_blind_dml" ->
          new MultiBlindDmlProbe(client, numRows, payloadSize, /* numDmlStatements= */ 5);
      case "strong_query" -> new QueryProbe(client, numRows, 0, fixedKey);
      case "stale_query" -> new QueryProbe(client, numRows, maxStalenessSeconds, fixedKey);
      case "multi_use_ro_query" -> new MultiUseReadOnlyQueryProbe(client, numRows);
      case "ycsb_fixed_read" ->
          new YcsbFixedReadProbe(client, ycsbTable, ycsbKey, ycsbUserId, ycsbZeroPadding);
      default -> throw new IllegalArgumentException("Unsupported workload: " + workload);
    };
  }
}
