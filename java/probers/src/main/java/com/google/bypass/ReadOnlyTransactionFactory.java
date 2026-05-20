package com.google.bypass;

import com.google.cloud.spanner.DatabaseClient;
import com.google.cloud.spanner.ReadOnlyTransaction;
import java.lang.reflect.Array;
import java.lang.reflect.InvocationTargetException;
import java.lang.reflect.Method;

/** Creates read-only transactions, including optional APIs that may only exist in local builds. */
final class ReadOnlyTransactionFactory {
  @SuppressWarnings({"rawtypes", "unchecked"})
  static ReadOnlyTransaction create(DatabaseClient client, boolean inlineBegin) {
    if (!inlineBegin) {
      return client.readOnlyTransaction();
    }
    try {
      Class<?> beginOptionClass = Class.forName("com.google.cloud.spanner.Options$BeginTransactionOption");
      Class<?> readOnlyOptionClass = Class.forName("com.google.cloud.spanner.Options$ReadOnlyTransactionOption");
      Object inline = Enum.valueOf(beginOptionClass.asSubclass(Enum.class), "INLINE");
      Method beginTransactionOption =
          Class.forName("com.google.cloud.spanner.Options")
              .getMethod("beginTransactionOption", beginOptionClass);
      Object option = beginTransactionOption.invoke(null, inline);
      Object options = Array.newInstance(readOnlyOptionClass, 1);
      Array.set(options, 0, option);
      Method readOnlyTransaction = DatabaseClient.class.getMethod("readOnlyTransaction", options.getClass());
      return (ReadOnlyTransaction) readOnlyTransaction.invoke(client, options);
    } catch (InvocationTargetException e) {
      Throwable cause = e.getCause();
      if (cause instanceof RuntimeException) {
        throw (RuntimeException) cause;
      }
      throw new IllegalStateException("Failed to create inline-begin read-only transaction", cause);
    } catch (ReflectiveOperationException e) {
      throw new IllegalStateException(
          "Inline-begin read-only transaction option is unavailable in this Spanner client build", e);
    }
  }

  private ReadOnlyTransactionFactory() {}
}
