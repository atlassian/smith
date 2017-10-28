git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "bcc7a4ff5a21ff2640f86cb302a7d976e33d89f1",
)
git_repository(
    name = "io_bazel_rules_docker",
    remote = "https://github.com/bazelbuild/rules_docker.git",
    commit = "58d022892232e5d59daba7760289976d5f6e7433",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains")
load("@io_bazel_rules_go//proto:def.bzl", "proto_register_toolchains")
load("@io_bazel_rules_docker//container:container.bzl", "container_pull", container_repositories = "repositories")

go_rules_dependencies()
go_register_toolchains()
proto_register_toolchains()

container_repositories()

# https://github.com/GoogleCloudPlatform/distroless/blob/master/base/README.md
container_pull(
    name = "distroless_base",
    digest = "sha256:872f258db0668e5cabfe997d4076b2fe5337e5b73cdd9ca47c7dbccd87e71341",
    registry = "gcr.io",
    repository = "distroless/base",
    #tag = "latest",  # ignored, but kept here for documentation
)
