package main

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	defaultProject             = "span-cloud-testing"
	defaultInstance            = "irahul-load-test"
	defaultDatabase            = "db"
	defaultProbeType           = "strong_read"
	defaultLoad                = 4
	defaultNumRows             = 10_000_000
	defaultPayloadSize         = 1000
	defaultMaxStalenessSecond  = 60
	defaultWarmupCycles        = 1000
	defaultParallelism         = 8
	defaultLogIntervalSeconds  = 10
	defaultLoadStepInterval    = 60
	defaultBurstAfterSeconds   = 900
	defaultHighLoadHoldSeconds = 300
	defaultLowLoadHoldSeconds  = 300
	defaultPprofAddr           = ":6060"

	defaultOTELServiceName                = "irahul-gloadtest"
	defaultOTELProjectID                  = "span-cloud-testing"
	defaultOTELMetricPrefix               = "custom.googleapis.com/irahul"
	defaultOTELTraceSamplingFraction      = 0.0
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
	startLoad           float64
	endLoad             float64
	loadSteps           []float64
	stepLoadPercent     float64
	loadStepInterval    int
	stepWarmupDiscard   int
	burstEnabled        bool
	burstAfterSeconds   int
	loadCycleEnabled    bool
	highLoadHoldSeconds int
	lowLoadHoldSeconds  int
	numRows             int64
	payloadSize         int
	fixedKey            int64
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
	loadSteps, err := parseLoadSteps(getEnv("LOAD_STEPS", ""))
	if err != nil {
		return config{}, err
	}
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
		startLoad:           getEnvFloat64Any([]string{"START_LOAD", "LOAD", "START_QPS", "QPS"}, defaultLoad),
		endLoad:             getEnvFloat64Any([]string{"END_LOAD", "END_QPS"}, 0),
		loadSteps:           loadSteps,
		stepLoadPercent:     getEnvFloat64Any([]string{"STEP_LOAD_PERCENT", "STEP_QPS_PERCENT", "STEP_QPS"}, 0),
		loadStepInterval:    getEnvIntAny([]string{"LOAD_STEP_INTERVAL_SECONDS", "QPS_STEP_INTERVAL_SECONDS", "INTERVAL_SECONDS"}, defaultLoadStepInterval),
		stepWarmupDiscard:   getEnvIntAny([]string{"STEP_WARMUP_DISCARD_SECONDS", "LOAD_STEP_WARMUP_DISCARD_SECONDS"}, 0),
		burstEnabled:        getEnvBoolAny([]string{"BURST_ENABLED", "BURST_MODE"}, false),
		burstAfterSeconds:   getEnvInt("BURST_AFTER_SECONDS", defaultBurstAfterSeconds),
		loadCycleEnabled:    getEnvBoolAny([]string{"LOAD_CYCLE_ENABLED", "QPS_CYCLE_ENABLED", "CYCLE_ENABLED"}, false),
		highLoadHoldSeconds: getEnvIntAny([]string{"HIGH_LOAD_HOLD_SECONDS", "HIGH_QPS_HOLD_SECONDS"}, defaultHighLoadHoldSeconds),
		lowLoadHoldSeconds:  getEnvIntAny([]string{"LOW_LOAD_HOLD_SECONDS", "LOW_QPS_HOLD_SECONDS"}, defaultLowLoadHoldSeconds),
		numRows:             getEnvInt64("NUM_ROWS", defaultNumRows),
		payloadSize:         getEnvInt("PAYLOAD_SIZE", defaultPayloadSize),
		fixedKey:            getEnvInt64("FIXED_KEY", -1),
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
	if len(cfg.loadSteps) > 0 {
		if cfg.loadSteps[0] > 0 {
			cfg.startLoad = cfg.loadSteps[0]
		}
		if cfg.loadSteps[len(cfg.loadSteps)-1] > 0 {
			cfg.endLoad = cfg.loadSteps[len(cfg.loadSteps)-1]
		}
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
	case cfg.startLoad <= 0:
		return cfg, fmt.Errorf("START_LOAD/START_QPS/QPS must be > 0, got %f", cfg.startLoad)
	case cfg.endLoad < 0:
		return cfg, fmt.Errorf("END_LOAD/END_QPS must be >= 0, got %f", cfg.endLoad)
	case cfg.stepLoadPercent < 0:
		return cfg, fmt.Errorf("STEP_LOAD_PERCENT/STEP_QPS_PERCENT/STEP_QPS must be >= 0, got %f", cfg.stepLoadPercent)
	case cfg.loadStepInterval <= 0:
		return cfg, fmt.Errorf("LOAD_STEP_INTERVAL_SECONDS/QPS_STEP_INTERVAL_SECONDS/INTERVAL_SECONDS must be > 0, got %d", cfg.loadStepInterval)
	case cfg.stepWarmupDiscard < 0:
		return cfg, fmt.Errorf("STEP_WARMUP_DISCARD_SECONDS must be >= 0, got %d", cfg.stepWarmupDiscard)
	case cfg.burstAfterSeconds < 0:
		return cfg, fmt.Errorf("BURST_AFTER_SECONDS must be >= 0, got %d", cfg.burstAfterSeconds)
	case cfg.highLoadHoldSeconds < 0:
		return cfg, fmt.Errorf("HIGH_LOAD_HOLD_SECONDS/HIGH_QPS_HOLD_SECONDS must be >= 0, got %d", cfg.highLoadHoldSeconds)
	case cfg.lowLoadHoldSeconds < 0:
		return cfg, fmt.Errorf("LOW_LOAD_HOLD_SECONDS/LOW_QPS_HOLD_SECONDS must be >= 0, got %d", cfg.lowLoadHoldSeconds)
	case len(cfg.loadSteps) == 0 && cfg.loadCycleEnabled && cfg.endLoad <= cfg.startLoad:
		return cfg, fmt.Errorf("END_LOAD must be greater than START_LOAD when LOAD_CYCLE_ENABLED=true, got start=%f end=%f", cfg.startLoad, cfg.endLoad)
	case len(cfg.loadSteps) == 0 && cfg.loadCycleEnabled && cfg.burstEnabled && cfg.highLoadHoldSeconds <= 0:
		return cfg, fmt.Errorf("HIGH_LOAD_HOLD_SECONDS must be > 0 for burst LOAD_CYCLE_ENABLED=true")
	case len(cfg.loadSteps) == 0 && cfg.loadCycleEnabled && !cfg.burstEnabled && cfg.lowLoadHoldSeconds <= 0:
		return cfg, fmt.Errorf("LOW_LOAD_HOLD_SECONDS must be > 0 for non-burst LOAD_CYCLE_ENABLED=true")
	case len(cfg.loadSteps) == 0 && cfg.loadCycleEnabled && !cfg.burstEnabled && cfg.stepLoadPercent <= 0:
		return cfg, fmt.Errorf("STEP_LOAD_PERCENT must be > 0 for non-burst LOAD_CYCLE_ENABLED=true")
	case cfg.numRows <= 0:
		return cfg, fmt.Errorf("NUM_ROWS must be > 0, got %d", cfg.numRows)
	case cfg.payloadSize <= 0:
		return cfg, fmt.Errorf("PAYLOAD_SIZE must be > 0, got %d", cfg.payloadSize)
	case cfg.fixedKey >= cfg.numRows:
		return cfg, fmt.Errorf("FIXED_KEY must be less than NUM_ROWS, got fixed_key=%d num_rows=%d", cfg.fixedKey, cfg.numRows)
	case cfg.maxInflight <= 0:
		return cfg, fmt.Errorf("MAX_INFLIGHT/PARALLELISM must be > 0, got %d", cfg.maxInflight)
	case cfg.loadMode == loadModeConcurrency && cfg.startLoad > float64(cfg.maxInflight):
		return cfg, fmt.Errorf("START_LOAD worker count must be <= MAX_INFLIGHT in LOAD_MODE=concurrency, got start=%f max_inflight=%d", cfg.startLoad, cfg.maxInflight)
	case cfg.loadMode == loadModeConcurrency && cfg.endLoad > float64(cfg.maxInflight):
		return cfg, fmt.Errorf("END_LOAD worker count must be <= MAX_INFLIGHT in LOAD_MODE=concurrency, got end=%f max_inflight=%d", cfg.endLoad, cfg.maxInflight)
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
	case len(cfg.loadSteps) > 0 && cfg.burstEnabled:
		return cfg, fmt.Errorf("BURST_ENABLED cannot be combined with LOAD_STEPS")
	case len(cfg.loadSteps) > 0 && cfg.stepLoadPercent > 0:
		return cfg, fmt.Errorf("STEP_LOAD_PERCENT cannot be combined with LOAD_STEPS")
	}
	for _, step := range cfg.loadSteps {
		switch {
		case math.IsNaN(step) || math.IsInf(step, 0) || step <= 0:
			return cfg, fmt.Errorf("LOAD_STEPS values must be finite and > 0, got %f", step)
		case cfg.loadMode == loadModeConcurrency && step > float64(cfg.maxInflight):
			return cfg, fmt.Errorf("LOAD_STEPS worker count must be <= MAX_INFLIGHT in LOAD_MODE=concurrency, got step=%f max_inflight=%d", step, cfg.maxInflight)
		}
	}
	switch cfg.queryMode {
	case "normal", "stats":
	default:
		return cfg, fmt.Errorf("QUERY_MODE must be one of [normal, stats], got %q", cfg.queryMode)
	}
	return cfg, nil
}

func parseLoadSteps(raw string) ([]float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	steps := make([]float64, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		parsed, err := strconv.ParseFloat(field, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid LOAD_STEPS value %q: %w", field, err)
		}
		steps = append(steps, parsed)
	}
	return steps, nil
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
