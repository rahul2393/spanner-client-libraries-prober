package com.google.bypass;

import com.google.cloud.spanner.DatabaseClient;
import com.google.cloud.spanner.Key;
import com.google.cloud.spanner.KeySet;
import com.google.cloud.spanner.Options;
import com.google.cloud.spanner.ReadOnlyTransaction;
import com.google.cloud.spanner.ResultSet;

/** Probe for multi-use read-only transactions that issue multiple reads. */
public final class MultiUseReadOnlyReadProbe implements Probe {
  private static final int READS_PER_TRANSACTION = 2;

  private final DatabaseClient client;
  private final int numRows;
  private final boolean inlineBegin;

  public MultiUseReadOnlyReadProbe(DatabaseClient client, int numRows, boolean inlineBegin) {
    this.client = client;
    this.numRows = numRows;
    this.inlineBegin = inlineBegin;
  }

  @Override
  public String getName() {
    return inlineBegin ? "multi_use_ro_read_inline_begin" : "multi_use_ro_read";
  }

  @Override
  public void probe() {
    int firstKey = (int) (Math.random() * numRows);
    try (ReadOnlyTransaction txn = ReadOnlyTransactionFactory.create(client, inlineBegin)) {
      for (int i = 0; i < READS_PER_TRANSACTION; i++) {
        int key = (firstKey + i) % numRows;
        try (ResultSet rs =
            txn.read(
                TABLE,
                KeySet.singleKey(Key.of(key)),
                COLUMNS,
                Options.tag("probe_type=" + getName()))) {
          if (rs.next()) {
            Probe.validateRows(rs);
          }
        }
      }
    }
  }
}
