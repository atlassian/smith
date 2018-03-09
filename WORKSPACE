git_repository(
    name = "bazel_gazelle",
    commit = "db967cc738fb9cc1f081461b531c525dea57b2a0",
    remote = "https://github.com/bazelbuild/bazel-gazelle.git",
)

git_repository(
    name = "io_bazel_rules_go",
    commit = "4ce98b727e37d18ed6482ed27d2e7ce0b7711a19",
    remote = "https://github.com/bazelbuild/rules_go.git",
)

git_repository(
    name = "io_bazel_rules_docker",
    commit = "27c94dec66c3c9fdb478c33994471c5bfc15b6eb",
    remote = "https://github.com/bazelbuild/rules_docker.git",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains")
load(
    "@io_bazel_rules_docker//go:image.bzl",
    go_image_repositories = "repositories",
)
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

go_rules_dependencies()

go_register_toolchains()

go_image_repositories()

gazelle_dependencies()
