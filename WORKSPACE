git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "fabe06345cff38edfe49a18ec3705e781698e98c",
)
git_repository(
    name = "io_bazel_rules_docker",
    remote = "https://github.com/bazelbuild/rules_docker.git",
    commit = "04165ef6de8e12975740238445cad6a623e6de2f",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains")
load("@io_bazel_rules_docker//container:container.bzl", "container_pull", container_repositories = "repositories")

go_rules_dependencies()
go_register_toolchains()

container_repositories()

# https://github.com/GoogleCloudPlatform/distroless/blob/master/base/README.md
container_pull(
    name = "distroless_base",
    digest = "sha256:4a8979a768c3ef8d0a8ed8d0af43dc5920be45a51749a9c611d178240f136eb4",
    registry = "gcr.io",
    repository = "distroless/base",
    #tag = "latest",  # ignored, but kept here for documentation
)
