module github.com/cloudfoundry/galera-init

go 1.13

require (
	code.cloudfoundry.org/lager v2.0.0+incompatible
	github.com/DATA-DOG/go-sqlmock v1.1.4-0.20160722192640-05f39e9110c0
	github.com/containerd/continuity v0.0.0-20181001140422-bd77b46c8352 // indirect
	github.com/docker/docker v1.4.2-0.20181208172742-edf5134ba77d // indirect
	github.com/docker/libnetwork v0.8.0-dev.2.0.20181115152545-1f28166bb386 // indirect
	github.com/fsouza/go-dockerclient v1.3.1
	github.com/go-sql-driver/mysql v1.2.1-0.20160802113842-0b58b37b664c
	github.com/google/uuid v1.1.0
	github.com/imdario/mergo v0.0.0-20160517064435-50d4dbd4eb0e // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo v1.7.0
	github.com/onsi/gomega v1.4.3
	github.com/opencontainers/runc v1.0.0-rc5.0.20181113215238-10d38b660a77 // indirect
	github.com/pivotal-cf-experimental/service-config v0.0.0-20160129003516-b1dc94de6ada
	github.com/pkg/errors v0.8.0
	github.com/sirupsen/logrus v1.2.0 // indirect
	golang.org/x/crypto v0.0.0-20181203042331-505ab145d0a9 // indirect
	golang.org/x/net v0.0.0-20181207154023-610586996380 // indirect
	golang.org/x/sys v0.0.0-20181208175041-ad97f365e150 // indirect
	gopkg.in/validator.v2 v2.0.0-20160201165114-3e4f037f12a1
	gopkg.in/yaml.v1 v1.0.0-20140924161607-9f9df34309c0 // indirect
)

replace gopkg.in/fsnotify.v1 v1.4.7 => gopkg.in/fsnotify/fsnotify.v1 v1.4.7
