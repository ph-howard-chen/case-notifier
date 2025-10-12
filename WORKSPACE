load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive")

# Go rules
http_archive(
    name = "io_bazel_rules_go",
    sha256 = "099a9fb96a376ccbbb7d291ed4ecbdfd42f6bc822ab77ae6f1b5cb9e914e94fa",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/rules_go/releases/download/v0.35.0/rules_go-v0.35.0.zip",
        "https://github.com/bazelbuild/rules_go/releases/download/v0.35.0/rules_go-v0.35.0.zip",
    ],
)

# Gazelle
http_archive(
    name = "bazel_gazelle",
    sha256 = "5982e5463f171da99e3bdaeff8c0f48283a7a5f396ec5282910b9e8a49c0dd7e",
    urls = [
        "https://mirror.bazel.build/github.com/bazelbuild/bazel-gazelle/releases/download/v0.25.0/bazel-gazelle-v0.25.0.tar.gz",
        "https://github.com/bazelbuild/bazel-gazelle/releases/download/v0.25.0/bazel-gazelle-v0.25.0.tar.gz",
    ],
)

load("@io_bazel_rules_go//go:deps.bzl", "go_register_toolchains", "go_rules_dependencies")
load("@bazel_gazelle//:deps.bzl", "gazelle_dependencies", "go_repository")

go_rules_dependencies()

go_register_toolchains(version = "1.23.0")

gazelle_dependencies()

# External Go dependencies
go_repository(
    name = "com_github_resend_resend_go_v2",
    importpath = "github.com/resend/resend-go/v2",
    sum = "h1:Ctj2EekOZ2ggH9L5K7ZuO+1SIrO7Iy+Dy4pvNAafb1k=",
    version = "v2.26.0",
)

go_repository(
    name = "com_github_chromedp_chromedp",
    importpath = "github.com/chromedp/chromedp",
    sum = "h1:r3b/WtwM50RsBZHMUm9fsNhhzRStTHrKdr2zmwbZSzM=",
    version = "v0.14.2",
)

go_repository(
    name = "com_github_chromedp_cdproto",
    importpath = "github.com/chromedp/cdproto",
    sum = "h1:UQ4AU+BGti3Sy/aLU8KVseYKNALcX9UXY6DfpwQ6J8E=",
    version = "v0.0.0-20250724212937-08a3db8b4327",
)

go_repository(
    name = "com_github_chromedp_sysutil",
    importpath = "github.com/chromedp/sysutil",
    sum = "h1:PUFNv5EcprjqXZD9nJb9b/c9ibAbxiYo4exNWZyipwM=",
    version = "v1.1.0",
)

go_repository(
    name = "com_github_gobwas_httphead",
    importpath = "github.com/gobwas/httphead",
    sum = "h1:exrUm0f4YX0L7EBwZHuCF4GDp8aJfVeBrlLQrs6NqWU=",
    version = "v0.1.0",
)

go_repository(
    name = "com_github_gobwas_pool",
    importpath = "github.com/gobwas/pool",
    sum = "h1:xfeeEhW7pwmX8nuLVlqbzVc7udMDrwetjEv+TZIz1og=",
    version = "v0.2.1",
)

go_repository(
    name = "com_github_gobwas_ws",
    importpath = "github.com/gobwas/ws",
    sum = "h1:CTaoG1tojrh4ucGPcoJFiAQUAsEWekEWvLy7GsVNqGs=",
    version = "v1.4.0",
)

go_repository(
    name = "com_github_go_json_experiment_json",
    importpath = "github.com/go-json-experiment/json",
    sum = "h1:iizUGZ9pEquQS5jTGkh4AqeeHCMbfbjeb0zMt0aEFzs=",
    version = "v0.0.0-20250725192818-e39067aee2d2",
)
