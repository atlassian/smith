workspace(name = "com_github_atlassian_smith")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "8b68d0630d63d95dacc0016c3bb4b76154fe34fca93efd65d1c366de3fcb4294",
    urls = ["https://github.com/bazelbuild/rules_go/releases/download/0.12.1/rules_go-0.12.1.tar.gz"],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "ddedc7aaeb61f2654d7d7d4fd7940052ea992ccdb031b8f9797ed143ac7e8d43",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.12.0/bazel-gazelle-0.12.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "8333642f283e73818fe14c744ca07ca168b78fe9fe5c7c0e9a37c295a8029b75",
    strip_prefix = "rules_docker-452878d665648ada0aaf816931611fdd9c683a97",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/452878d665648ada0aaf816931611fdd9c683a97.zip"],
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "d2755e7e16c087e6f841691d772cb89a1e9255ea16b6c52b33a119a07a9dd249",
    strip_prefix = "buildtools-47728e38feb98d5f354ea1eb99e0e44f0e4d7a14",
    urls = ["https://github.com/bazelbuild/buildtools/archive/47728e38feb98d5f354ea1eb99e0e44f0e4d7a14.zip"],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "c6abcc19e65707a0232e4523d68c0d320f780db9f3eb0e36adea4fcd1055463b",
    strip_prefix = "bazel-tools-06d357a7c08ab0854821e106f0891aa80d130b35",
    urls = ["https://github.com/atlassian/bazel-tools/archive/06d357a7c08ab0854821e106f0891aa80d130b35.tar.gz"],
)

load("@io_bazel_rules_go//go:def.bzl", "go_register_toolchains", "go_rules_dependencies")
load(
    "@io_bazel_rules_docker//go:image.bzl",
    go_image_repositories = "repositories",
)
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")
load("@com_github_bazelbuild_buildtools//buildifier:deps.bzl", "buildifier_dependencies")
load("@com_github_atlassian_bazel_tools//buildozer:deps.bzl", "buildozer_dependencies")
load("@com_github_atlassian_bazel_tools//goimports:deps.bzl", "goimports_dependencies")

go_rules_dependencies()

go_register_toolchains()

go_image_repositories()

gazelle_dependencies()

buildifier_dependencies()

buildozer_dependencies()

goimports_dependencies()
