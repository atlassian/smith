workspace(name = "com_github_atlassian_smith")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "2ef1d7970012550e5cf636b66359c21b37b3ffdf8346c6f1743a3686180ffe05",
    strip_prefix = "rules_go-3553e886579e390f045893050e4d79e760e70ebb",
    urls = ["https://github.com/bazelbuild/rules_go/archive/3553e886579e390f045893050e4d79e760e70ebb.tar.gz"],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "6e875ab4b6bf64a38c352887760f21203ab054676d9c1b274963907e0768740d",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.15.0/bazel-gazelle-0.15.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "35ea63d865e9e484ef4150629a302614e92d0bb95757770ffc273faf4d9a1f17",
    strip_prefix = "rules_docker-39186e056fd7dc0c29c676e387e1ad73fc381aa2",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/39186e056fd7dc0c29c676e387e1ad73fc381aa2.tar.gz"],
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "2a593582bfaf77717afb33371891f87f5d631af6de10069926ff8e51eab1a232",
    strip_prefix = "buildtools-53432872c9e41db2d613d653f3cd0707d53ebc56",
    urls = ["https://github.com/bazelbuild/buildtools/archive/53432872c9e41db2d613d653f3cd0707d53ebc56.tar.gz"],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "91208f44bbc6ac9773f34b624e25c90216bdf35e533ec9caa6fd60e7d33b0de2",
    strip_prefix = "bazel-tools-333dc4fc3538c407a8af095ad35bfb83e26ab853",
    urls = ["https://github.com/atlassian/bazel-tools/archive/333dc4fc3538c407a8af095ad35bfb83e26ab853.tar.gz"],
)

http_archive(
    name = "bazel_skylib",
    sha256 = "b5f6abe419da897b7901f90cbab08af958b97a8f3575b0d3dd062ac7ce78541f",
    strip_prefix = "bazel-skylib-0.5.0",
    urls = ["https://github.com/bazelbuild/bazel-skylib/archive/0.5.0.tar.gz"],
)

load("@bazel_skylib//:lib.bzl", "versions")

versions.check(minimum_bazel_version = "0.18.0")

load("@io_bazel_rules_go//go:def.bzl", "go_register_toolchains", "go_rules_dependencies")
load(
    "@io_bazel_rules_docker//go:image.bzl",
    go_image_repositories = "repositories",
)
load("@com_github_bazelbuild_buildtools//buildifier:deps.bzl", "buildifier_dependencies")
load("@com_github_atlassian_bazel_tools//buildozer:deps.bzl", "buildozer_dependencies")
load("@com_github_atlassian_bazel_tools//goimports:deps.bzl", "goimports_dependencies")
load("@com_github_atlassian_bazel_tools//gometalinter:deps.bzl", "gometalinter_dependencies")
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies")

go_rules_dependencies()

go_register_toolchains()

gazelle_dependencies()

goimports_dependencies()

go_image_repositories()

buildifier_dependencies()

buildozer_dependencies()

gometalinter_dependencies()
