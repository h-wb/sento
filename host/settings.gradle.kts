rootProject.name = "sento-host"

// AndroidCompat's build file uses typesafe accessors (projects.androidCompat.config).
enableFeaturePreview("TYPESAFE_PROJECT_ACCESSORS")

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
        // Dependency versions come from the pinned Suwayomi-Server
        // submodule, so vendored code and its modules always compile
        // against the versions upstream tested.
        create("libs") {
            from(files("Suwayomi-Server/gradle/libs.versions.toml"))
        }
    }
}

// The fake Android runtime builds straight from the submodule — it is a
// standalone module with no imports from the rest of Suwayomi-Server.
include(":AndroidCompat")
project(":AndroidCompat").projectDir = file("Suwayomi-Server/AndroidCompat")
include(":AndroidCompat:Config")
project(":AndroidCompat:Config").projectDir = file("Suwayomi-Server/AndroidCompat/Config")

include(":app")
