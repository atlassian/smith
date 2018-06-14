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
    sha256 = "0e73051b2ee35fccf7100bc58169fd95653d57b33d3ae2c27f87d5391af40a01",
    strip_prefix = "bazel-tools-5ecaf1b6a74c0375dcc62c763d5350b3abe64fea",
    urls = ["https://github.com/atlassian/bazel-tools/archive/5ecaf1b6a74c0375dcc62c763d5350b3abe64fea.zip"],
)

load("@io_bazel_rules_go//go:def.bzl", "go_register_toolchains", "go_rules_dependencies")
load(
    "@io_bazel_rules_docker//go:image.bzl",
    go_image_repositories = "repositories",
)
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")
load("@com_github_bazelbuild_buildtools//buildifier:deps.bzl", "buildifier_dependencies")
load("@com_github_atlassian_bazel_tools//buildozer:deps.bzl", "buildozer_dependencies")

go_rules_dependencies()

go_register_toolchains()

go_image_repositories()

gazelle_dependencies()

buildifier_dependencies()

buildozer_dependencies()
