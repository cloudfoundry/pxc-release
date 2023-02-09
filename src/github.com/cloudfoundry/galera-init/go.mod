module github.com/cloudfoundry/galera-init

go 1.14

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/DATA-DOG/go-sqlmock v1.1.4-0.20160722192640-05f39e9110c0
	github.com/Microsoft/hcsshim v0.8.20 // indirect
	github.com/fsnotify/fsnotify v1.5.0 // indirect
	github.com/fsouza/go-dockerclient v1.7.4
	github.com/go-sql-driver/mysql v1.6.0
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/uuid v1.2.0
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/maxbrunsfeld/counterfeiter/v6 v6.2.3
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.16.0
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/pivotal-cf-experimental/service-config v0.0.0-20160129003516-b1dc94de6ada
	github.com/pkg/errors v0.9.1
	go.opencensus.io v0.23.0 // indirect
	gopkg.in/validator.v2 v2.0.0-20210331031555-b37d688a7fb0
)

replace gopkg.in/fsnotify.v1 v1.4.7 => gopkg.in/fsnotify/fsnotify.v1 v1.4.7
