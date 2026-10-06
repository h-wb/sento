rootProject.name = "sento-extensions"

// AndroidCompat's build file uses typesafe accessors (projects.androidCompat.config).
enableFeaturePreview("TYPESAFE_PROJECT_ACCESSORS")

// Vendored Suwayomi pieces are materialized by vendir (pinned in
// vendir.lock.yml), not committed.
val vendor = file("vendor/suwayomi")
require(vendor.isDirectory) {
    "vendor/suwayomi is missing — run: mise run extensions:vendor (vendir sync)"
}

dependencyResolutionManagement {
    repositories {
        mavenCentral()
        google()
        // android.jar stub published by the Suwayomi project
        maven("https://github.com/Suwayomi/Suwayomi-Server/raw/android-jar/")
        maven("https://jitpack.io")
        maven("https://jogamp.org/deployment/maven")
        maven("https://packages.jetbrains.team/maven/p/ij/intellij-dependencies")
    }
    versionCatalogs {
        // Dependency versions come from the pinned Suwayomi sync, so
        // vendored code always compiles against the versions upstream
        // tested.
        create("libs") {
            from(files("vendor/suwayomi/gradle/libs.versions.toml"))
        }
    }
}

// The fake Android runtime builds straight from the sync — it is a
// standalone module with no imports from the rest of Suwayomi-Server.
include(":AndroidCompat")
project(":AndroidCompat").projectDir = vendor.resolve("AndroidCompat")
include(":AndroidCompat:Config")
project(":AndroidCompat:Config").projectDir = vendor.resolve("AndroidCompat/Config")

include(":app")
