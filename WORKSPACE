workspace(name = "com_github_atlassian_smith")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "12b89992cd76a864cd1d862077e475149898e69fad18c843cb3c90328c8879f7",
    strip_prefix = "rules_go-d023cdf5d7ba59e8ba61a214ff8277556ab5066f",
    urls = ["https://github.com/bazelbuild/rules_go/archive/d023cdf5d7ba59e8ba61a214ff8277556ab5066f.tar.gz"],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "7949fc6cc17b5b191103e97481cf8889217263acf52e00b560683413af204fcb",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.16.0/bazel-gazelle-0.16.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "00449701ccbdc2581bb8d1be33af75246596ae75fdca19c1196be1df962435ce",
    strip_prefix = "rules_docker-67d567bcfe5920acfaec270c41aa3e5f3262ca42",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/67d567bcfe5920acfaec270c41aa3e5f3262ca42.tar.gz"],
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "896f18860254d9a165ad65550666806cbf58dbb0fb71b1821df132c20db42b44",
    strip_prefix = "buildtools-aa1408f15df9f4c9e713dd5949fedfb04865199a",
    urls = ["https://github.com/bazelbuild/buildtools/archive/aa1408f15df9f4c9e713dd5949fedfb04865199a.tar.gz"],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "1f6afacd6e17d515d8aae083d21090d61a0f4b67ed7733a87e222d9cfbcf65c2",
    strip_prefix = "bazel-tools-53556f7bd66e71e96be027306fa5b6df68060165",
    urls = ["https://github.com/atlassian/bazel-tools/archive/53556f7bd66e71e96be027306fa5b6df68060165.tar.gz"],
)

http_archive(
    name = "bazel_skylib",
    sha256 = "2c62d8cd4ab1e65c08647eb4afe38f51591f43f7f0885e7769832fa137633dcb",
    strip_prefix = "bazel-skylib-0.7.0",
    urls = ["https://github.com/bazelbuild/bazel-skylib/archive/0.7.0.tar.gz"],
)

load("@bazel_skylib//lib:versions.bzl", "versions")

versions.check(minimum_bazel_version = "0.18.0")

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")

go_rules_dependencies()

go_register_toolchains(nogo = "@//:nogo")

load(
    "@io_bazel_rules_docker//go:image.bzl",
    go_image_repositories = "repositories",
)
load("@com_github_bazelbuild_buildtools//buildifier:deps.bzl", "buildifier_dependencies")
load("@com_github_atlassian_bazel_tools//buildozer:deps.bzl", "buildozer_dependencies")
load("@com_github_atlassian_bazel_tools//goimports:deps.bzl", "goimports_dependencies")
load("@com_github_atlassian_bazel_tools//gometalinter:deps.bzl", "gometalinter_dependencies")
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

gazelle_dependencies()

goimports_dependencies()

go_image_repositories()

buildifier_dependencies()

buildozer_dependencies()

gometalinter_dependencies()
