git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    tag = "0.6.0",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains", "go_repository")
go_rules_dependencies()
go_register_toolchains()

load("@io_bazel_rules_go//proto:def.bzl", "proto_register_toolchains")
proto_register_toolchains()

go_repository(
    name = "com_github_tedsuo_ifrit",
    commit = "08b0eeeeac72245729c8c62f5f1276eb940e9b3d",
    importpath = "github.com/tedsuo/ifrit",
)

go_repository(
    name = "com_github_oklog_ulid",
    commit = "66bb6560562feca7045b23db1ae85b01260f87c5",
    importpath = "github.com/oklog/ulid",
)

go_repository(
    name = "com_github_onsi_gomega",
    commit = "39a54bd3c3bbfe1c331a9b3207e92134c77ed812",
    importpath = "github.com/onsi/gomega",
)

go_repository(
    name = "com_github_onsi_ginkgo",
    commit = "a1f616c97771e46da1722d3aa9dcde0a43f55682",
    importpath = "github.com/onsi/ginkgo",
)

go_repository(
    name = "org_golang_x_net",
    commit = "3da985ce5951d99de868be4385f21ea6c2b22f24",
    importpath = "golang.org/x/net",
)

go_repository(
    name = "org_golang_x_text",
    commit = "4ee4af566555f5fbe026368b75596286a312663a",
    importpath = "golang.org/x/text",
)

go_repository(
    name = "org_cloudfoundry_code_lager",
    commit = "b4f07fd10491fae0d6d9a0d07145e9f90935ac50",
    importpath = "code.cloudfoundry.org/lager",
)

go_repository(
    name = "com_github_golang_protobuf",
    commit = "6e4cc92cc905d5f4a73041c1b8228ea08f4c6147",
    importpath = "github.com/golang/protobuf",
)

go_repository(
    name = "in_gopkg_yaml_v2",
    commit = "cd8b52f8269e0feb286dfeef29f8fe4d5b397e0b",
    importpath = "gopkg.in/yaml.v2",
)

go_repository(
    name = "org_golang_google_genproto",
    commit = "aa2eb687b4d3e17154372564ad8d6bf11c3cf21f",
    importpath = "google.golang.org/genproto",
)

go_repository(
    name = "com_github_square_certstrap",
    commit = "fa1359e6e510efcf9cd67bc0edbe43d7300a7833",
    importpath = "github.com/square/certstrap",
    build_file_generation = "on",
)
