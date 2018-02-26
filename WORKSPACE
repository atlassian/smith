git_repository(
    name = "bazel_gazelle",
    commit = "006d93517d5c38aec366e6abe004ef5ab69f056d",
    remote = "https://github.com/bazelbuild/bazel-gazelle.git",
)

git_repository(
    name = "io_bazel_rules_go",
    commit = "6b39964af66c98580be4c5ac6cf1d243332f78e4",
    remote = "https://github.com/bazelbuild/rules_go.git",
)

git_repository(
    name = "io_bazel_rules_docker",
    commit = "198367210c55fba5dded22274adde1a289801dc4",
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
