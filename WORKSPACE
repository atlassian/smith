workspace(name = "com_github_atlassian_smith")

load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

http_archive(
    name = "io_bazel_rules_go",
    sha256 = "ade51a315fa17347e5c31201fdc55aa5ffb913377aa315dceb56ee9725e620ee",
    urls = ["https://github.com/bazelbuild/rules_go/releases/download/0.16.6/rules_go-0.16.6.tar.gz"],
)

http_archive(
    name = "bazel_gazelle",
    sha256 = "7949fc6cc17b5b191103e97481cf8889217263acf52e00b560683413af204fcb",
    urls = ["https://github.com/bazelbuild/bazel-gazelle/releases/download/0.16.0/bazel-gazelle-0.16.0.tar.gz"],
)

http_archive(
    name = "io_bazel_rules_docker",
    sha256 = "03f5924ec802d686551a31c478b4aab59578ada7c77be88a1ee62769ed6668a4",
    strip_prefix = "rules_docker-170335d284991ecc9fa5a6682c46bd32f167daa9",
    urls = ["https://github.com/bazelbuild/rules_docker/archive/170335d284991ecc9fa5a6682c46bd32f167daa9.tar.gz"],
)

http_archive(
    name = "com_github_bazelbuild_buildtools",
    sha256 = "61408f4ad3f9bd8d1f32da1afed748e592764947343cf99481927bf91137d77e",
    strip_prefix = "buildtools-9f8fdb20dd423621ef00ced33dcb40204703c2c8",
    urls = ["https://github.com/bazelbuild/buildtools/archive/9f8fdb20dd423621ef00ced33dcb40204703c2c8.tar.gz"],
)

http_archive(
    name = "com_github_atlassian_bazel_tools",
    sha256 = "8adb7ed5338f6c47501144725a41a21a1e572469671dacd6f6eb717adc45887c",
    strip_prefix = "bazel-tools-48732510b3e423741ebb3f80f408134c75d56c4e",
    urls = ["https://github.com/atlassian/bazel-tools/archive/48732510b3e423741ebb3f80f408134c75d56c4e.tar.gz"],
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
