workspace(name = "com_github_atlassian_smith")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "f87fa87475ea107b3c69196f39c82b7bbf58fe27c62a338684c20ca17d1d8613",
    urls = ["https://github.com/bazelbuild/rules_go/releases/download/0.16.2/rules_go-0.16.2.tar.gz"],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "6e875ab4b6bf64a38c352887760f21203ab054676d9c1b274963907e0768740d",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.15.0/bazel-gazelle-0.15.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "1996d03503a8c593874915361938f3472364208027ef2435a5e7cd79410ee798",
    strip_prefix = "rules_docker-9527234ef0b5a57bce93be524cb56d7ab1a85ea3",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/9527234ef0b5a57bce93be524cb56d7ab1a85ea3.tar.gz"],
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "953ed402234e54874a2a4020b011fd8d5e6e575883fe2c11fb112819c19edb28",
    strip_prefix = "buildtools-d39e4d5c25111527369142f16cdb49aa67707313",
    urls = ["https://github.com/bazelbuild/buildtools/archive/d39e4d5c25111527369142f16cdb49aa67707313.tar.gz"],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "8f3f0fdafc350845f96e024b1c61cfdda9b1102a93ac5bdb8e08f545334c81a8",
    strip_prefix = "bazel-tools-e9957c4df9ee31e63b4f396512cb8df74afe97b2",
    urls = ["https://github.com/atlassian/bazel-tools/archive/e9957c4df9ee31e63b4f396512cb8df74afe97b2.tar.gz"],
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
