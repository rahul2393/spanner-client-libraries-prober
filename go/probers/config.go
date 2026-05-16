package main

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultProject            = "span-cloud-testing"
	defaultInstance           = "irahul-load-test"
	defaultDatabase           = "db"
	defaultProbeType          = "strong_read"
	defaultQPS                = 4
	defaultNumRows            = 10_000_000
	defaultPayloadSize        = 1000
	defaultMaxStalenessSecond = 60
	defaultWarmupCycles       = 1000
	defaultParallelism        = 8
	defaultLogIntervalSeconds = 10
	defaultQpsStepInterval    = 60
	defaultBurstAfterSeconds  = 900
	defaultHighQPSHoldSeconds = 300
	defaultLowQPSHoldSeconds  = 300
	defaultPprofAddr          = ":6060"

	defaultOTELServiceName                = "irahul-gloadtest"
	defaultOTELProjectID                  = "span-cloud-testing"
	defaultOTELMetricPrefix               = "custom.googleapis.com/irahul"
	defaultOTELTraceSamplingFraction      = 1.0
	defaultOTELMetricExportIntervalSecond = 10

	tableName = "T"
	columnKey = "Key"
	columnVal = "Value"
)

type loadMode string

const (
	loadModeQPS         loadMode = "qps"
	loadModeConcurrency loadMode = "concurrency"
)

type config struct {
	project             string
	instance            string
	database            string
	databasePath        string
	endpoint            string
	insecure            bool
	probeType           string
	queryMode           string
	loadMode            loadMode
	startQPS            float64
	endQPS              float64
	stepQPSPercent      float64
	qpsStepInterval     int
	burstEnabled        bool
	burstAfterSeconds   int
	qpsCycleEnabled     bool
	highQPSHoldSeconds  int
	lowQPSHoldSeconds   int
	numRows             int64
	payloadSize         int
	maxStalenessSeconds int64
	warmupCycles        int
	maxInflight         int
	logIntervalSeconds  int
	enablePprof         bool
	pprofAddr           string

	enableDirectAccess             bool
	enableGcpFallback              bool
	enableBypass                   bool
	enableDCP                      bool
	dcpInitialChannels             int
	dcpMinChannels                 int
	dcpMaxChannels                 int
	dcpMaxRPCPerChannel            float64
	dcpMinRPCPerChannel            float64
	enableOTEL                     bool
	otelProjectID                  string
	otelServiceName                string
	otelMetricPrefix               string
	otelExportDebug                bool
	otelTraceSamplingFraction      float64
	otelMetricExportIntervalSecond int
	cloudTraceEndpoint             string
	monitoringEndpoint             string
	enableSpannerEndToEndTracing   bool
}

func loadConfig() (config, error) {
	cfg := config{
		project:             getEnvAny([]string{"SPANNER_PROJECT_ID", "GOOGLE_CLOUD_PROJECT"}, defaultProject),
		instance:            getEnvAny([]string{"SPANNER_INSTANCE_ID", "SPANNER_INSTANCE"}, defaultInstance),
		database:            getEnvAny([]string{"SPANNER_DATABASE_ID", "SPANNER_DATABASE"}, defaultDatabase),
		databasePath:        strings.TrimSpace(os.Getenv("SPANNER_DATABASE_PATH")),
		endpoint:            normalizeEndpoint(getEnv("ENDPOINT", "")),
		insecure:            getEnvBool("INSECURE", false),
		probeType:           strings.ToLower(getEnv("PROBE_TYPE", defaultProbeType)),
		queryMode:           strings.ToLower(getEnv("QUERY_MODE", "normal")),
		loadMode:            loadMode(strings.ToLower(strings.ReplaceAll(getEnv("LOAD_MODE", string(loadModeQPS)), "-", "_"))),
		startQPS:            getEnvFloat64Any([]string{"START_QPS", "QPS"}, defaultQPS),
		endQPS:              getEnvFloat64("END_QPS", 0),
		stepQPSPercent:      getEnvFloat64Any([]string{"STEP_QPS_PERCENT", "STEP_QPS"}, 0),
		qpsStepInterval:     getEnvIntAny([]string{"QPS_STEP_INTERVAL_SECONDS", "INTERVAL_SECONDS"}, defaultQpsStepInterval),
		burstEnabled:        getEnvBoolAny([]string{"BURST_ENABLED", "BURST_MODE"}, false),
		burstAfterSeconds:   getEnvInt("BURST_AFTER_SECONDS", defaultBurstAfterSeconds),
		qpsCycleEnabled:     getEnvBoolAny([]string{"QPS_CYCLE_ENABLED", "CYCLE_ENABLED"}, false),
		highQPSHoldSeconds:  getEnvInt("HIGH_QPS_HOLD_SECONDS", defaultHighQPSHoldSeconds),
		lowQPSHoldSeconds:   getEnvInt("LOW_QPS_HOLD_SECONDS", defaultLowQPSHoldSeconds),
		numRows:             getEnvInt64("NUM_ROWS", defaultNumRows),
		payloadSize:         getEnvInt("PAYLOAD_SIZE", defaultPayloadSize),
		maxStalenessSeconds: getEnvInt64("MAX_STALENESS_SECONDS", defaultMaxStalenessSecond),
		warmupCycles:        getEnvInt("WARMUP_CYCLES", defaultWarmupCycles),
		maxInflight:         getEnvIntAny([]string{"MAX_INFLIGHT", "PARALLELISM"}, defaultParallelism),
		logIntervalSeconds:  getEnvInt("LOG_INTERVAL_SECONDS", defaultLogIntervalSeconds),
		enablePprof:         getEnvBool("PPROF_ENABLED", false),
		pprofAddr:           getEnv("PPROF_ADDR", defaultPprofAddr),

		enableDirectAccess:             getEnvBool("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", false),
		enableGcpFallback:              getEnvBool("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", false),
		enableBypass:                   getEnvBool("GOOGLE_SPANNER_EXPERIMENTAL_LOCATION_API", false),
		enableDCP:                      getEnvBoolAny([]string{"SPANNER_DCP_ENABLED", "DCP_ENABLED"}, false),
		dcpInitialChannels:             getEnvInt("SPANNER_DCP_INITIAL_CHANNELS", 0),
		dcpMinChannels:                 getEnvInt("SPANNER_DCP_MIN_CHANNELS", 0),
		dcpMaxChannels:                 getEnvInt("SPANNER_DCP_MAX_CHANNELS", 0),
		dcpMaxRPCPerChannel:            getEnvFloat64("SPANNER_DCP_MAX_RPC_PER_CHANNEL", 0),
		dcpMinRPCPerChannel:            getEnvFloat64("SPANNER_DCP_MIN_RPC_PER_CHANNEL", 0),
		enableOTEL:                     getEnvBool("OTEL_ENABLED", true),
		otelProjectID:                  getEnv("OTEL_PROJECT_ID", defaultOTELProjectID),
		otelServiceName:                getEnv("OTEL_SERVICE_NAME", defaultOTELServiceName),
		otelMetricPrefix:               getEnv("OTEL_METRIC_PREFIX", defaultOTELMetricPrefix),
		otelExportDebug:                getEnvBool("OTEL_EXPORT_DEBUG", false),
		otelTraceSamplingFraction:      getEnvFloat64("OTEL_TRACE_SAMPLING_FRACTION", defaultOTELTraceSamplingFraction),
		otelMetricExportIntervalSecond: getEnvInt("OTEL_METRIC_EXPORT_INTERVAL_SECONDS", defaultOTELMetricExportIntervalSecond),
		cloudTraceEndpoint:             normalizeEndpoint(strings.TrimSpace(os.Getenv("CLOUD_TRACE_ENDPOINT"))),
		monitoringEndpoint:             normalizeEndpoint(strings.TrimSpace(os.Getenv("SPANNER_MONITORING_HOST"))),
		enableSpannerEndToEndTracing:   getEnvBool("SPANNER_ENABLE_END_TO_END_TRACING", false),
	}
	if cfg.databasePath == "" {
		cfg.databasePath = fmt.Sprintf("projects/%s/instances/%s/databases/%s", cfg.project, cfg.instance, cfg.database)
	}
	return validateConfig(cfg)
}

func validateConfig(cfg config) (config, error) {
	if cfg.loadMode == "" {
		cfg.loadMode = loadModeQPS
	}
	if cfg.queryMode == "" {
		cfg.queryMode = "normal"
	}
	switch {
	case cfg.loadMode != loadModeQPS && cfg.loadMode != loadModeConcurrency:
		return cfg, fmt.Errorf("LOAD_MODE must be one of [qps, concurrency], got %q", cfg.loadMode)
	case cfg.startQPS <= 0:
		return cfg, fmt.Errorf("START_QPS/QPS must be > 0, got %f", cfg.startQPS)
	case cfg.endQPS < 0:
		return cfg, fmt.Errorf("END_QPS must be >= 0, got %f", cfg.endQPS)
	case cfg.stepQPSPercent < 0:
		return cfg, fmt.Errorf("STEP_QPS_PERCENT/STEP_QPS must be >= 0, got %f", cfg.stepQPSPercent)
	case cfg.qpsStepInterval <= 0:
		return cfg, fmt.Errorf("QPS_STEP_INTERVAL_SECONDS/INTERVAL_SECONDS must be > 0, got %d", cfg.qpsStepInterval)
	case cfg.burstAfterSeconds < 0:
		return cfg, fmt.Errorf("BURST_AFTER_SECONDS must be >= 0, got %d", cfg.burstAfterSeconds)
	case cfg.highQPSHoldSeconds < 0:
		return cfg, fmt.Errorf("HIGH_QPS_HOLD_SECONDS must be >= 0, got %d", cfg.highQPSHoldSeconds)
	case cfg.lowQPSHoldSeconds < 0:
		return cfg, fmt.Errorf("LOW_QPS_HOLD_SECONDS must be >= 0, got %d", cfg.lowQPSHoldSeconds)
	case cfg.qpsCycleEnabled && cfg.endQPS <= cfg.startQPS:
		return cfg, fmt.Errorf("END_QPS must be greater than START_QPS/QPS when QPS_CYCLE_ENABLED=true, got start=%f end=%f", cfg.startQPS, cfg.endQPS)
	case cfg.qpsCycleEnabled && cfg.burstEnabled && cfg.highQPSHoldSeconds <= 0:
		return cfg, fmt.Errorf("HIGH_QPS_HOLD_SECONDS must be > 0 for burst QPS_CYCLE_ENABLED=true")
	case cfg.qpsCycleEnabled && !cfg.burstEnabled && cfg.lowQPSHoldSeconds <= 0:
		return cfg, fmt.Errorf("LOW_QPS_HOLD_SECONDS must be > 0 for non-burst QPS_CYCLE_ENABLED=true")
	case cfg.qpsCycleEnabled && !cfg.burstEnabled && cfg.stepQPSPercent <= 0:
		return cfg, fmt.Errorf("STEP_QPS_PERCENT/STEP_QPS must be > 0 for non-burst QPS_CYCLE_ENABLED=true")
	case cfg.numRows <= 0:
		return cfg, fmt.Errorf("NUM_ROWS must be > 0, got %d", cfg.numRows)
	case cfg.payloadSize <= 0:
		return cfg, fmt.Errorf("PAYLOAD_SIZE must be > 0, got %d", cfg.payloadSize)
	case cfg.maxInflight <= 0:
		return cfg, fmt.Errorf("MAX_INFLIGHT/PARALLELISM must be > 0, got %d", cfg.maxInflight)
	case cfg.loadMode == loadModeConcurrency && cfg.startQPS > float64(cfg.maxInflight):
		return cfg, fmt.Errorf("START_QPS/QPS worker count must be <= MAX_INFLIGHT in LOAD_MODE=concurrency, got start=%f max_inflight=%d", cfg.startQPS, cfg.maxInflight)
	case cfg.loadMode == loadModeConcurrency && cfg.endQPS > float64(cfg.maxInflight):
		return cfg, fmt.Errorf("END_QPS worker count must be <= MAX_INFLIGHT in LOAD_MODE=concurrency, got end=%f max_inflight=%d", cfg.endQPS, cfg.maxInflight)
	case cfg.maxStalenessSeconds <= 0:
		return cfg, fmt.Errorf("MAX_STALENESS_SECONDS must be > 0, got %d", cfg.maxStalenessSeconds)
	case cfg.warmupCycles < 0:
		return cfg, fmt.Errorf("WARMUP_CYCLES must be >= 0, got %d", cfg.warmupCycles)
	case cfg.logIntervalSeconds <= 0:
		return cfg, fmt.Errorf("LOG_INTERVAL_SECONDS must be > 0, got %d", cfg.logIntervalSeconds)
	case cfg.enablePprof && strings.TrimSpace(cfg.pprofAddr) == "":
		return cfg, fmt.Errorf("PPROF_ADDR must be non-empty when PPROF_ENABLED=true")
	case cfg.dcpInitialChannels < 0:
		return cfg, fmt.Errorf("SPANNER_DCP_INITIAL_CHANNELS must be >= 0, got %d", cfg.dcpInitialChannels)
	case cfg.dcpMinChannels < 0:
		return cfg, fmt.Errorf("SPANNER_DCP_MIN_CHANNELS must be >= 0, got %d", cfg.dcpMinChannels)
	case cfg.dcpMaxChannels < 0:
		return cfg, fmt.Errorf("SPANNER_DCP_MAX_CHANNELS must be >= 0, got %d", cfg.dcpMaxChannels)
	case cfg.dcpMaxRPCPerChannel < 0:
		return cfg, fmt.Errorf("SPANNER_DCP_MAX_RPC_PER_CHANNEL must be >= 0, got %f", cfg.dcpMaxRPCPerChannel)
	case cfg.dcpMinRPCPerChannel < 0:
		return cfg, fmt.Errorf("SPANNER_DCP_MIN_RPC_PER_CHANNEL must be >= 0, got %f", cfg.dcpMinRPCPerChannel)
	case cfg.otelTraceSamplingFraction < 0 || cfg.otelTraceSamplingFraction > 1:
		return cfg, fmt.Errorf("OTEL_TRACE_SAMPLING_FRACTION must be in [0,1], got %f", cfg.otelTraceSamplingFraction)
	case cfg.otelMetricExportIntervalSecond <= 0:
		return cfg, fmt.Errorf("OTEL_METRIC_EXPORT_INTERVAL_SECONDS must be > 0, got %d", cfg.otelMetricExportIntervalSecond)
	}
	switch cfg.queryMode {
	case "normal", "stats":
	default:
		return cfg, fmt.Errorf("QUERY_MODE must be one of [normal, stats], got %q", cfg.queryMode)
	}
	return cfg, nil
}

func normalizeEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		u, err := url.Parse(raw)
		if err == nil && u.Host != "" {
			return u.Host
		}
	}
	return strings.TrimPrefix(strings.TrimPrefix(raw, "http://"), "https://")
}

func getEnv(key, defaultVal string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return defaultVal
}

func getEnvAny(keys []string, defaultVal string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return defaultVal
}

func getEnvBool(key string, defaultVal bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnvBoolAny(keys []string, defaultVal bool) bool {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			parsed, err := strconv.ParseBool(v)
			if err != nil {
				return defaultVal
			}
			return parsed
		}
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnvIntAny(keys []string, defaultVal int) int {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			parsed, err := strconv.Atoi(v)
			if err != nil {
				return defaultVal
			}
			return parsed
		}
	}
	return defaultVal
}

func getEnvInt64(key string, defaultVal int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnvFloat64(key string, defaultVal float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return defaultVal
	}
	return parsed
}

func getEnvFloat64Any(keys []string, defaultVal float64) float64 {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			parsed, err := strconv.ParseFloat(v, 64)
			if err != nil {
				return defaultVal
			}
			return parsed
		}
	}
	return defaultVal
}
