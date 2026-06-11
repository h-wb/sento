// Vendored Suwayomi module (MPL-2.0); build file adapted from upstream.
plugins {
    id(libs.plugins.kotlin.jvm.get().pluginId)
    id(libs.plugins.kotlin.serialization.get().pluginId)
}

dependencies {
    implementation(libs.bundles.shared)
}
