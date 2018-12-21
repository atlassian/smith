workspace(name = "com_github_atlassian_smith")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "7be7dc01f1e0afdba6c8eb2b43d2fa01c743be1b9273ab1eaf6c233df078d705",
    urls = ["https://github.com/bazelbuild/rules_go/releases/download/0.16.5/rules_go-0.16.5.tar.gz"],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "7949fc6cc17b5b191103e97481cf8889217263acf52e00b560683413af204fcb",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.16.0/bazel-gazelle-0.16.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "db6c798ff9de88bf449c5b977a54984458c73407a927d94322ccc8dc01b9a38c",
    strip_prefix = "rules_docker-f6664b6b5c7f4fac031883a7ec9fa6b8bab0ab98",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/f6664b6b5c7f4fac031883a7ec9fa6b8bab0ab98.tar.gz"],
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "68f0b91c5f96da8d61bc91f0bfa8fcefa5562871005fecf3bbf3dee479300edd",
    strip_prefix = "buildtools-78432f994f6611cbbfa3ebfb41e0c931c12c747d",
    urls = ["https://github.com/bazelbuild/buildtools/archive/78432f994f6611cbbfa3ebfb41e0c931c12c747d.tar.gz"],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "080773e11e832d3f2bf82576af1bf34d31e9fbdc99eb896b90732071afa5e4fd",
    strip_prefix = "bazel-tools-96c1e41762781a1f25de2f45e6f0557c9642ef94",
    urls = ["https://github.com/atlassian/bazel-tools/archive/96c1e41762781a1f25de2f45e6f0557c9642ef94.tar.gz"],
)

http_archive(
    name = "bazel_skylib",
    sha256 = "eb5c57e4c12e68c0c20bc774bfbc60a568e800d025557bc4ea022c6479acc867",
    strip_prefix = "bazel-skylib-0.6.0",
    urls = ["https://github.com/bazelbuild/bazel-skylib/archive/0.6.0.tar.gz"],
)

load("@bazel_skylib//lib:versions.bzl", "versions")

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
