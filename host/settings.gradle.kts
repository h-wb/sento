rootProject.name = "sento-host"

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
}

include(":AndroidCompat")
include(":AndroidCompat:Config")
include(":app")
