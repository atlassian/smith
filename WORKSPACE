workspace(name = "com_github_atlassian_smith")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "97cf62bdef33519412167fd1e4b0810a318a7c234f5f8dc4f53e2da86241c492",
    urls = ["https://github.com/bazelbuild/rules_go/releases/download/0.15.3/rules_go-0.15.3.tar.gz"],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "c0a5739d12c6d05b6c1ad56f2200cb0b57c5a70e03ebd2f7b87ce88cabf09c7b",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.14.0/bazel-gazelle-0.14.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "8795052cc537db8e0350ef6b5ad9d7a60079b9724359f43bf9f7287ca7704dee",
    strip_prefix = "rules_docker-0d6d69a2a4bbc33fc61a8350897b0e8136491ad5",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/0d6d69a2a4bbc33fc61a8350897b0e8136491ad5.tar.gz"],
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "a25411abad46673b35c2e3d59c53712d6e779800d1dffeed38e3fe3d05348a0b",
    strip_prefix = "buildtools-ae772d29d07002dfd89ed1d9ff673a1721f1b8dd",
    urls = ["https://github.com/bazelbuild/buildtools/archive/ae772d29d07002dfd89ed1d9ff673a1721f1b8dd.tar.gz"],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "15b3c0b43f29f802d4ec06ee2b8e175fac08c0e93f91950830e5ed3b4171dd3a",
    strip_prefix = "bazel-tools-03b64959dd47b6d904de72cfdca5eaa3a3945bf1",
    urls = ["https://github.com/atlassian/bazel-tools/archive/03b64959dd47b6d904de72cfdca5eaa3a3945bf1.tar.gz"],
)

http_archive(
    name = "bazel_skylib",
    sha256 = "b5f6abe419da897b7901f90cbab08af958b97a8f3575b0d3dd062ac7ce78541f",
    strip_prefix = "bazel-skylib-0.5.0",
    urls = ["https://github.com/bazelbuild/bazel-skylib/archive/0.5.0.tar.gz"],
)

load("@bazel_skylib//:lib.bzl", "versions")

versions.check(minimum_bazel_version = "0.14.0")

load("@io_bazel_rules_go//go:def.bzl", "go_register_toolchains", "go_rules_dependencies")
load(
    "@io_bazel_rules_docker//go:image.bzl",
    go_image_repositories = "repositories",
)
load("@com_github_bazelbuild_buildtools//buildifier:deps.bzl", "buildifier_dependencies")
load("@com_github_atlassian_bazel_tools//buildozer:deps.bzl", "buildozer_dependencies")
load("@com_github_atlassian_bazel_tools//goimports:deps.bzl", "goimports_dependencies")
load("@com_github_atlassian_bazel_tools//gometalinter:deps.bzl", "gometalinter_dependencies")

go_rules_dependencies()

go_register_toolchains()

load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

gazelle_dependencies()

go_image_repositories()

buildifier_dependencies()

buildozer_dependencies()

goimports_dependencies()

gometalinter_dependencies()
