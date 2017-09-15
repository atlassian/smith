git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "bd13f2d59c804acae7ca8c18fdeb4bf0ecfa1e93",
)
git_repository(
    name = "io_bazel_rules_docker",
    remote = "https://github.com/bazelbuild/rules_docker.git",
    tag = "v0.1.0",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains")
load("@io_bazel_rules_docker//docker:docker.bzl", "docker_repositories", "docker_pull")

go_rules_dependencies()
go_register_toolchains("1.9")

docker_repositories()

# https://github.com/GoogleCloudPlatform/distroless/blob/master/base/README.md
docker_pull(
    name = "distroless_base",
    digest = "sha256:872f258db0668e5cabfe997d4076b2fe5337e5b73cdd9ca47c7dbccd87e71341",
    registry = "gcr.io",
    repository = "distroless/base",
    tag = "latest",  # ignored, but kept here for documentation
)
