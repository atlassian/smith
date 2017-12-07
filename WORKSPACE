git_repository(
    name = "io_bazel_rules_go",
    remote = "https://github.com/bazelbuild/rules_go.git",
    commit = "26a0398ea9b380e981fad24065913942c84829e9",
)
git_repository(
    name = "io_bazel_rules_docker",
    remote = "https://github.com/bazelbuild/rules_docker.git",
    commit = "cce1de49c54b145a9d9542685660f72d8fa593a7",
)
git_repository(
    name = "distroless",
    remote = "https://github.com/GoogleCloudPlatform/distroless.git",
    commit = "07963f53de460af7edf2a1504712f70d9f3b0bdd",
)

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains")
load("@io_bazel_rules_docker//container:container.bzl", "container_pull", container_repositories = "repositories")
load("@distroless//package_manager:package_manager.bzl", "dpkg_src", "dpkg_list", "package_manager_repositories")
load("@io_bazel_rules_docker//go:image.bzl", go_image_repositories = "repositories")

go_image_repositories()
go_rules_dependencies()
go_register_toolchains()
container_repositories()
package_manager_repositories()

# https://github.com/GoogleCloudPlatform/distroless/blob/master/base/README.md
container_pull(
    name = "distroless_base",
    digest = "sha256:bef8d030c7f36dfb73a8c76137616faeea73ac5a8495d535f27c911d0db77af3",
    registry = "gcr.io",
    repository = "distroless/base",
    #tag = "latest",  # ignored, but kept here for documentation
)

dpkg_src(
    name = "debian_stretch",
    arch = "amd64",
    distro = "stretch",
    sha256 = "9aea0e4c9ce210991c6edcb5370cb9b11e9e554a0f563e7754a4028a8fd0cb73",
    snapshot = "20171204T214933Z",
    url = "http://snapshot.debian.org/archive",
)

dpkg_list(
    name = "cpp_bundle",
    packages = [
        "libstdc++6",
        "multiarch-support",
        "libc6",
        "libgcc1",
    ],
    sources = [
        "@debian_stretch//file:Packages.json",
    ],
)
