git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "95b702c5331b5d01445fe485c25fc80f3f9e0dcf",
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
    digest = "sha256:bef8d030c7f36dfb73a8c76137616faeea73ac5a8495d535f27c911d0db77af3",
    registry = "gcr.io",
    repository = "distroless/base",
    #tag = "latest",  # ignored, but kept here for documentation
)
