git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "2e319588571f20fdaaf83058b690abd32f596e89",
)
git_repository(
    name = "io_bazel_rules_docker",
    remote = "https://github.com/bazelbuild/rules_docker.git",
    tag = "v0.1.0",
)

load("@io_bazel_rules_go//go:def.bzl", "go_repositories")
load("@io_bazel_rules_docker//docker:docker.bzl", "docker_repositories", "docker_pull")

go_repositories(
    go_version = "1.9",
)

docker_repositories()

docker_pull(
    name = "official_busybox",
    digest = "sha256:b82b5740006c1ab823596d2c07f081084ecdb32fd258072707b99f52a3cb8692",
    registry = "index.docker.io",
    repository = "library/busybox",
    tag = "latest",  # ignored, but kept here for documentation
)
