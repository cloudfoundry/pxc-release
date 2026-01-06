module github.com/cloudfoundry-incubator/galera-healthcheck

go 1.25.0

require (
	code.cloudfoundry.org/lager/v3 v3.57.0
	code.cloudfoundry.org/tlsconfig v0.42.0
	github.com/DATA-DOG/go-sqlmock v1.5.2
	github.com/erikstmartin/go-testdb v0.0.0-20160219214506-8d10e4a1bae5
	github.com/go-sql-driver/mysql v1.9.3
	github.com/maxbrunsfeld/counterfeiter/v6 v6.12.1
	github.com/onsi/ginkgo/v2 v2.27.3
	github.com/onsi/gomega v1.38.3
	github.com/pkg/errors v0.9.1
	github.com/tedsuo/rata v1.0.0
	golang.org/x/net v0.48.0
	gopkg.in/validator.v2 v2.0.1
	gopkg.in/yaml.v3 v3.0.1
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/fsnotify/fsnotify v1.5.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20260106004452-d7df1bf2cac7 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/square/certstrap v1.3.0 // indirect
	go.step.sm/crypto v0.75.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.46.0 // indirect
	golang.org/x/mod v0.31.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/tools v0.40.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

exclude golang.org/x/tools v0.38.0
