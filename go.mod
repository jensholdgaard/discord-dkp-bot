module github.com/jensholdgaard/discord-dkp-bot

go 1.23

require (
	github.com/XSAM/otelsql v0.36.0
	github.com/bwmarrin/discordgo v0.28.1
	github.com/jmoiron/sqlx v1.4.0
	github.com/lib/pq v1.10.9
	go.opentelemetry.io/contrib/bridges/otelslog v0.9.0
	go.opentelemetry.io/contrib/instrumentation/database/sql v0.1.0
	go.opentelemetry.io/otel v1.34.0
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.10.0
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.34.0
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.34.0
	go.opentelemetry.io/otel/log v0.10.0
	go.opentelemetry.io/otel/metric v1.34.0
	go.opentelemetry.io/otel/sdk v1.34.0
	go.opentelemetry.io/otel/sdk/log v0.10.0
	go.opentelemetry.io/otel/sdk/metric v1.34.0
	go.opentelemetry.io/otel/trace v1.34.0
	gopkg.in/yaml.v3 v3.0.1
)
