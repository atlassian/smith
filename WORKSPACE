git_repository(
    name = "bazel_gazelle",
    commit = "93aa43ebe91c59ea00873232620198008d58e595",
    remote = "https://github.com/bazelbuild/bazel-gazelle.git",
)

git_repository(
    name = "io_bazel_rules_go",
    commit = "1c41d106559cbfa6fffe75481eeb492ae77471c0",
    remote = "https://github.com/bazelbuild/rules_go.git",
)

git_repository(
    name = "io_bazel_rules_docker",
    commit = "898b3b964cebb1b709c8529a3c449bf45700cc33",
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
