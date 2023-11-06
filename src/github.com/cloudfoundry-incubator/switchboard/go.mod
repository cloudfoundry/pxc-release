module github.com/cloudfoundry-incubator/switchboard

go 1.20

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	code.cloudfoundry.org/tlsconfig v0.0.0-20231017135636-f0e44068c22f
	github.com/cloudfoundry-incubator/galera-healthcheck v0.0.0-20220901215914-d591811a0fba
	github.com/maxbrunsfeld/counterfeiter/v6 v6.7.0
	github.com/onsi/ginkgo v1.16.5
	github.com/onsi/gomega v1.29.0
	github.com/pivotal-cf-experimental/service-config v0.0.0-20160129003516-b1dc94de6ada
	github.com/tedsuo/ifrit v0.0.0-20230516164442-7862c310ad26
	gopkg.in/validator.v2 v2.0.1
)

require (
	filippo.io/edwards25519 v1.0.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/imdario/mergo v0.3.16 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/square/certstrap v1.3.0 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	go.step.sm/crypto v0.36.1 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/mod v0.14.0 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.14.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.14.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/cloudfoundry-incubator/galera-healthcheck => ../galera-healthcheck
