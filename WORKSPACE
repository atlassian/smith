git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "a280fbac1a0a4c67b0eee660b4fd1b3db7c9f058",
)
git_repository(
    name = "io_bazel_rules_docker",
    remote = "https://github.com/bazelbuild/rules_docker.git",
    commit = "9dd92c73e7c8cf07ad5e0dca89a3c3c422a3ab7d",
#    tag = "v0.3.0",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains")
load("@io_bazel_rules_go//proto:def.bzl", "proto_register_toolchains")
load("@io_bazel_rules_docker//docker:docker.bzl", "docker_repositories", "docker_pull")

go_rules_dependencies()
go_register_toolchains()
proto_register_toolchains()

docker_repositories()

# https://github.com/GoogleCloudPlatform/distroless/blob/master/base/README.md
docker_pull(
    name = "distroless_base",
    digest = "sha256:872f258db0668e5cabfe997d4076b2fe5337e5b73cdd9ca47c7dbccd87e71341",
    registry = "gcr.io",
    repository = "distroless/base",
    tag = "latest",  # ignored, but kept here for documentation
)
