// Paste deliberately into a MongoDB Compass shell. This file is not executed by QuanTRAM.
const quantram = db.getSiblingDB("quantram_db");

function ensureCollection(name, validator) {
  if (!quantram.getCollectionNames().includes(name)) {
    quantram.createCollection(name, { validator });
  } else {
    quantram.runCommand({ collMod: name, validator });
  }
}

const objectID = { bsonType: "objectId" };
const date = { bsonType: "date" };
const number = { bsonType: ["double", "int", "long", "decimal"] };

ensureCollection("quantram_apertures", {
  $jsonSchema: {
    bsonType: "object",
    required: ["_id", "sequence_num", "open", "shut", "status", "semantic_contract_version", "model_version", "schema_version", "created_at"],
    properties: {
      _id: objectID,
      sequence_num: { bsonType: ["int", "long"] },
      status: { enum: ["OPEN", "SHUT"] },
      semantic_contract_version: { bsonType: "string" },
      model_version: { bsonType: "string" },
      schema_version: { bsonType: "string" },
      open: date,
      shut: { bsonType: ["date", "null"] }
    }
  }
});

ensureCollection("quantram_payloads", {
  $jsonSchema: {
    bsonType: "object",
    required: ["_id", "aperture_id", "bar"],
    properties: {
      _id: objectID,
      aperture_id: objectID,
      bar: {
        bsonType: "object",
        required: ["symbol", "instrument_id", "instrument_type", "tradable", "interval", "interval_start_unix_ms", "interval_end_unix_ms", "open", "high", "low", "close", "volume", "event_count", "source_timestamp", "receipt_unix_ms", "source", "quality_status", "is_final", "is_backfilled", "source_transition", "market_snapshot_id"],
        properties: {
          symbol: { bsonType: "string" },
          instrument_id: { bsonType: "string" },
          instrument_type: { bsonType: "string" },
          tradable: { bsonType: "bool" },
          interval: { bsonType: "string" },
          interval_start_unix_ms: date,
          interval_end_unix_ms: date,
          open: number,
          high: number,
          low: number,
          close: number,
          volume: { bsonType: ["long", "int"] },
          event_count: { bsonType: ["long", "int"] },
          source_timestamp: { bsonType: "string" },
          receipt_unix_ms: date,
          source: { bsonType: "string" },
          quality_status: { bsonType: "string" },
          is_final: { bsonType: "bool" },
          is_backfilled: { bsonType: "bool" },
          source_transition: { bsonType: "bool" },
          market_snapshot_id: { bsonType: "string" }
        }
      }
    }
  }
});

ensureCollection("quantram_decisions", {
  $jsonSchema: {
    bsonType: "object",
    required: ["_id", "aperture_id", "payload_id"],
    properties: {
      _id: objectID,
      aperture_id: objectID,
      payload_id: objectID,
      decision_event: { bsonType: ["object", "null"] },
      adaptive_outputs: { bsonType: ["object", "null"] },
      price_event: { bsonType: ["object", "null"] }
    }
  }
});

ensureCollection("quantram_snapshot_policies", {
  $jsonSchema: {
    bsonType: "object",
    required: ["_id", "name", "status", "trigger"],
    properties: {
      _id: objectID,
      name: { bsonType: "string" },
      status: { enum: ["ACTIVE", "INACTIVE"] },
      trigger: {
        bsonType: "object",
        required: ["type", "every_n_bars"],
        properties: {
          type: { enum: ["EVERY_N_BARS"] },
          every_n_bars: { bsonType: ["int", "long"], minimum: 1 }
        }
      },
      created_at: date,
      updated_at: date
    }
  }
});

ensureCollection("quantram_snapshots", {
  $jsonSchema: {
    bsonType: "object",
    required: ["_id", "aperture_id", "policy_id", "payload_id", "symbol", "snapshot_num", "captured_at"],
    properties: {
      _id: objectID,
      aperture_id: objectID,
      policy_id: objectID,
      payload_id: objectID,
      symbol: { bsonType: "string" },
      snapshot_num: { bsonType: ["int", "long"], minimum: 1 },
      captured_at: date
    }
  }
});

ensureCollection("quantram_snapshot_runs", {
  $jsonSchema: {
    bsonType: "object",
    required: ["_id", "aperture_id", "policy_id", "symbol", "trigger_payload_id", "trigger_count", "status", "started_at", "completed_at", "snapshot_id", "error"],
    properties: {
      _id: objectID,
      aperture_id: objectID,
      policy_id: objectID,
      trigger_payload_id: objectID,
      symbol: { bsonType: "string" },
      trigger_count: { bsonType: ["int", "long"], minimum: 1 },
      status: { enum: ["STARTED", "SUCCESS", "ERROR"] },
      started_at: date,
      completed_at: { bsonType: ["date", "null"] },
      snapshot_id: { bsonType: ["objectId", "null"] },
      error: {
        bsonType: ["object", "null"],
        properties: {
          code: { bsonType: "string" },
          message: { bsonType: "string" }
        }
      }
    }
  }
});

quantram.quantram_apertures.createIndex({ sequence_num: 1 }, { unique: true });
quantram.quantram_payloads.createIndex({ aperture_id: 1, "bar.market_snapshot_id": 1 }, { unique: true });
quantram.quantram_payloads.createIndex({ aperture_id: 1, "bar.symbol": 1, "bar.interval_start_unix_ms": 1 });
quantram.quantram_decisions.createIndex({ payload_id: 1 }, { unique: true });
quantram.quantram_decisions.createIndex({ aperture_id: 1, payload_id: 1 });
quantram.quantram_snapshot_policies.createIndex({ status: 1, "trigger.type": 1 });
quantram.quantram_snapshots.createIndex({ aperture_id: 1, policy_id: 1, symbol: 1, payload_id: 1 }, { unique: true });
quantram.quantram_snapshots.createIndex({ aperture_id: 1, policy_id: 1, symbol: 1, snapshot_num: 1 });
quantram.quantram_snapshot_runs.createIndex({ aperture_id: 1, policy_id: 1, symbol: 1, trigger_count: 1, started_at: 1 });
quantram.quantram_snapshot_runs.createIndex({ status: 1, started_at: 1 });
quantram.quantram_snapshot_runs.createIndex(
  { aperture_id: 1, policy_id: 1, symbol: 1, trigger_payload_id: 1 },
  { unique: true, partialFilterExpression: { status: "SUCCESS" } }
);

// Optional deliberate policy insertion; review and uncomment in Compass when desired.
// quantram.quantram_snapshot_policies.insertOne({
//   name: "Development Every 10 Bars",
//   status: "ACTIVE",
//   trigger: { type: "EVERY_N_BARS", every_n_bars: NumberLong(10) },
//   created_at: new Date(),
//   updated_at: new Date()
// });
