// Vendored Suwayomi module (MPL-2.0); build file adapted from upstream
// (ktlint dropped, project accessor spelled out).
plugins {
    id(libs.plugins.kotlin.jvm.get().pluginId)
    id(libs.plugins.kotlin.serialization.get().pluginId)
}

dependencies {
    implementation(libs.bundles.shared)

    // Android stub library
    implementation(libs.android.stubs)

    // XML
    compileOnly(libs.xmlpull)

    // Config API
    implementation(project(":AndroidCompat:Config"))

    // APK sig verifier
    compileOnly(libs.apksig)

    // AndroidX annotations
    compileOnly(libs.android.annotations)

    // substitute for duktape-android/quickjs
    implementation(libs.bundles.polyglot)

    // Kotlin wrapper around Java Preferences, makes certain things easier
    implementation(libs.bundles.settings)

    // Android version of SimpleDateFormat
    implementation(libs.icu4j)

    // OpenJDK lacks native JPEG encoder and native WEBP decoder
    implementation(libs.bundles.twelvemonkeys)
    implementation(libs.imageio.webp)
}
