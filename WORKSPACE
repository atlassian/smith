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
    sha256 = "8795052cc537db8e0350ef6b5ad9d7a60079b9724359f43bf9f7287ca7704dee",
    strip_prefix = "rules_docker-0d6d69a2a4bbc33fc61a8350897b0e8136491ad5",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/0d6d69a2a4bbc33fc61a8350897b0e8136491ad5.tar.gz"],
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "681130514b50ee640cd5eee9cbd192fd48072b4bc9abc6a17a1fba7a2817ec0e",
    strip_prefix = "buildtools-5a4c4ca9753ad0f8f9eb3463d84bc89388846420",
    urls = ["https://github.com/bazelbuild/buildtools/archive/5a4c4ca9753ad0f8f9eb3463d84bc89388846420.tar.gz"],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "bd3daaa2a928d0549ac0bb1c2571031b47ea09bed6802c9aca517cc662f0f51c",
    strip_prefix = "bazel-tools-e0e575b8a809c4565ef189be871bb6b11cd91043",
    urls = ["https://github.com/atlassian/bazel-tools/archive/e0e575b8a809c4565ef189be871bb6b11cd91043.tar.gz"],
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
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")
load("@com_github_bazelbuild_buildtools//buildifier:deps.bzl", "buildifier_dependencies")
load("@com_github_atlassian_bazel_tools//buildozer:deps.bzl", "buildozer_dependencies")
load("@com_github_atlassian_bazel_tools//goimports:deps.bzl", "goimports_dependencies")
load("@com_github_atlassian_bazel_tools//gometalinter:deps.bzl", "gometalinter_dependencies")

go_rules_dependencies()

go_register_toolchains()

go_image_repositories()

gazelle_dependencies()

buildifier_dependencies()

buildozer_dependencies()

goimports_dependencies()

gometalinter_dependencies()
