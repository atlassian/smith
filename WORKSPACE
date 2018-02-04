git_repository(
    name = "bazel_gazelle",
    commit = "2e9aecb45fd33c4fc105b44258d40e2fbb760ff7",
    remote = "https://github.com/bazelbuild/bazel-gazelle.git",
)

git_repository(
    name = "io_bazel_rules_go",
    commit = "213d5fcd0853a51f970a2cdd4c984b411d5bbf79",
    remote = "https://github.com/bazelbuild/rules_go.git",
)

git_repository(
    name = "io_bazel_rules_docker",
    commit = "bf925ec58ad96f2ead21cd8379caedbe3c26efc9",
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
